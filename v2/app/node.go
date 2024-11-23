package app

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"kwil/config"
	"kwil/crypto"
	"kwil/log"
	"kwil/node"
	"kwil/node/consensus"
	"kwil/version"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type server struct {
	cfg *config.Config // KwildConfig
	log log.Logger

	cancelCtxFunc context.CancelFunc
	closers       *closeFuncs
	dbCtx         interface {
		Done() <-chan struct{}
		Err() error
	}

	// Modules
	node *node.Node
	ce   *consensus.ConsensusEngine
}

func runNode(ctx context.Context, rootDir string, cfg *config.Config) error {
	// Writing to stdout and a log file.  TODO: config outputs
	rot, err := log.NewRotatorWriter(filepath.Join(rootDir, "kwild.log"), 10_000, 0)
	if err != nil {
		return fmt.Errorf("failed to create log rotator: %w", err)
	}
	defer func() {
		if err := rot.Close(); err != nil {
			fmt.Printf("failed to close log rotator: %v", err)
		}
	}()

	logWriter := io.MultiWriter(os.Stdout, rot) // tee to stdout and log file

	logger := log.New(log.WithLevel(cfg.LogLevel), log.WithFormat(cfg.LogFormat),
		log.WithName("KWILD"), log.WithWriter(logWriter))
	// NOTE: level and name can be set independently for different systems

	logger.Infof("Starting kwild version %v", version.KwilVersion)

	genFile := filepath.Join(rootDir, GenesisFileName)

	logger.Infof("Loading the genesis configuration from %s", genFile)

	genConfig, err := config.LoadGenesisConfig(genFile)
	if err != nil {
		return fmt.Errorf("failed to load genesis config: %w", err)
	}

	privKey, err := crypto.UnmarshalSecp256k1PrivateKey(cfg.PrivateKey)
	if err != nil {
		return err
	}
	pubKey := privKey.Public().Bytes()

	logger.Info("Parsing the pubkey", "key", hex.EncodeToString(pubKey))

	host, port, user, pass := cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Pass

	d := &coreDependencies{
		ctx:        ctx,
		rootDir:    rootDir,
		cfg:        cfg,
		genesisCfg: genConfig,
		privKey:    privKey,
		logger:     logger,
		dbOpener:   newDBOpener(host, port, user, pass),
		poolOpener: newPoolBOpener(host, port, user, pass),
	}

	server := buildServer(ctx, d)

	// start the server
	// Start is blocking, for now.
	return server.Start(ctx)
}

func (s *server) Start(ctx context.Context) error {
	defer func() {
		if err := recover(); err != nil {
			s.log.Error("Panic in server", "error", err)
		}

		s.log.Info("Closing server resources...")
		err := s.closers.closeAll()
		if err != nil {
			s.log.Error("failed to close resource:", "error", err)
		}
		s.log.Info("Server is now shut down.")
	}()

	s.log.Info("Starting the server")

	cancelCtx, done := context.WithCancel(ctx)
	s.cancelCtxFunc = done

	group, groupCtx := errgroup.WithContext(cancelCtx)

	group.Go(func() error {
		// If the DB dies unexpectedly, stop the entire error group.
		select {
		case <-s.dbCtx.Done(): // DB died
			return s.dbCtx.Err() // shutdown the server
		case <-groupCtx.Done(): // something else died or was shut down
			return nil
		}
	})

	// start rpc services

	// start node (p2p)
	group.Go(func() error {
		if err := s.node.Start(groupCtx, s.cfg.P2P.BootNodes...); err != nil {
			s.log.Error("failed to start node", "error", err)
			return err
		}
		return nil
	})

	// TODO: node is starting the consensus engine for ease of testing
	// Start the consensus engine

	err := group.Wait()

	if err != nil {
		if errors.Is(err, context.Canceled) {
			s.log.Info("server context is canceled")
			return nil
		}

		s.log.Error("server error", zap.Error(err))
		s.cancelCtxFunc()
		return err
	}

	return nil
}
