package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/kwilteam/kwil-db/config"
	"github.com/kwilteam/kwil-db/core/crypto"
	"github.com/kwilteam/kwil-db/core/crypto/auth"
	ethdeposits "github.com/kwilteam/kwil-db/extensions/listeners/eth_deposits"
	"github.com/kwilteam/kwil-db/test/setup"
	"github.com/kwilteam/kwil-db/test/specifications"
	"github.com/stretchr/testify/require"
)

// TestKwildDatabaseIntegration is to ensure that nodes are able to
// produce blocks and accept db related transactions and agree on the
// state of the database
func TestKwildDatabaseIntegration(t *testing.T) {
	p := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.DefaultNodeConfig(),
				setup.DefaultNodeConfig(),
				setup.DefaultNodeConfig(),
			},
			DBOwner: "0xabc",
		},
	})

	ctx := context.Background()

	clt := p.Nodes[0].JSONRPCClient(t, ctx, false, "")

	ping, err := clt.Ping(ctx)
	require.NoError(t, err)

	require.Equal(t, "pong", ping)

	// specifications.CreateNamespaceSpecification(ctx, t, clt)
}

// TestKwildValidatorUpdates is to test the functionality of
// validators joining and leaving the network.
func TestKwildValidatorUpdates(t *testing.T) {
	p := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.DefaultNodeConfig(),
				setup.DefaultNodeConfig(),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Validator = false
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Validator = false
				}),
			},
			// ConfigureGenesis: func(genDoc *config.GenesisConfig) {
			// 	genDoc.JoinExpiry = 5 // 5 sec at 1block/sec
			// },
			DBOwner: "0xabc",
		},
	})

	ctx := context.Background()

	// wait for all the nodes to discover each other
	time.Sleep(2 * time.Second)

	n0Admin := p.Nodes[0].AdminClient(t, ctx)
	n1Admin := p.Nodes[1].AdminClient(t, ctx)
	n2Admin := p.Nodes[2].AdminClient(t, ctx)
	n3Admin := p.Nodes[3].AdminClient(t, ctx)

	// Ensure that the network has 2 validators
	specifications.CurrentValidatorsSpecification(ctx, t, n0Admin, 2)

	// Reject join requests from an existing validator
	specifications.JoinExistingValidatorSpecification(ctx, t, n0Admin, p.Nodes[1].PrivateKey())

	// Reject leave requests from a non-validator
	specifications.NonValidatorLeaveSpecification(ctx, t, n3Admin)

	// Reject leave requests from the leader
	specifications.InvalidLeaveSpecification(ctx, t, n0Admin)

	time.Sleep(2 * time.Second)

	// Node0 and 1 are Validators and Node2 will issue a join request and requires approval from both validators
	specifications.ValidatorNodeJoinSpecification(ctx, t, n2Admin, p.Nodes[2].PrivateKey(), 2)

	// Nodes can't self approve join requests
	specifications.NodeApprovalFailSpecification(ctx, t, n2Admin, p.Nodes[2].PrivateKey())

	// Non validators can't approve join requests
	specifications.NodeApprovalFailSpecification(ctx, t, n3Admin, p.Nodes[2].PrivateKey())

	// node0 approves the join request
	specifications.ValidatorNodeApproveSpecification(ctx, t, n0Admin, p.Nodes[2].PrivateKey(), 2, 2, false)

	// node1 approves the join request and the node2 becomes a validator
	specifications.ValidatorNodeApproveSpecification(ctx, t, n1Admin, p.Nodes[2].PrivateKey(), 2, 3, true)

	// Ensure that the network has 3 validators
	specifications.CurrentValidatorsSpecification(ctx, t, n0Admin, 3)
	specifications.CurrentValidatorsSpecification(ctx, t, n3Admin, 3)

	/*
		Leave Process:
		- node2 issues a leave request -> removes it from the validator list
		- Validator set count should be reduced by 1
	*/
	specifications.ValidatorNodeLeaveSpecification(ctx, t, n2Admin)

	// Node should be able to rejoin the network
	specifications.ValidatorNodeJoinSpecification(ctx, t, n2Admin, p.Nodes[2].PrivateKey(), 2)
	time.Sleep(2 * time.Second)
	specifications.ValidatorNodeApproveSpecification(ctx, t, n0Admin, p.Nodes[2].PrivateKey(), 2, 2, false)
	specifications.ValidatorNodeApproveSpecification(ctx, t, n1Admin, p.Nodes[2].PrivateKey(), 2, 3, true)
	time.Sleep(2 * time.Second)
	specifications.CurrentValidatorsSpecification(ctx, t, n3Admin, 3)
}

func TestValidatorJoinExpirySpecification(t *testing.T) {
	p := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.DefaultNodeConfig(),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Validator = false
				}),
			},
			ConfigureGenesis: func(genDoc *config.GenesisConfig) {
				genDoc.JoinExpiry = 5 // 5 sec at 1block/sec
			},
			DBOwner: "0xabc",
		},
	})

	ctx := context.Background()

	// wait for all the nodes to discover each other
	time.Sleep(2 * time.Second)

	n0Admin := p.Nodes[0].AdminClient(t, ctx)
	n1Admin := p.Nodes[1].AdminClient(t, ctx)

	// Ensure that the network has 2 validators
	specifications.CurrentValidatorsSpecification(ctx, t, n0Admin, 1)

	// Reject join requests from an existing validator
	specifications.ValidatorJoinExpirySpecification(ctx, t, n1Admin, p.Nodes[1].PrivateKey(), 8*time.Second)
}

func TestKwildValidatorRemoveSpecification(t *testing.T) {
	p := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.DefaultNodeConfig(),
				setup.DefaultNodeConfig(),
				setup.DefaultNodeConfig(),
				setup.DefaultNodeConfig(),
			},
			DBOwner: "0xabc",
		},
	})

	ctx := context.Background()

	// wait for all the nodes to discover each other
	time.Sleep(2 * time.Second)

	n0Admin := p.Nodes[0].AdminClient(t, ctx)
	n1Admin := p.Nodes[1].AdminClient(t, ctx)
	n2Admin := p.Nodes[2].AdminClient(t, ctx)

	// 4 validators, remove one validator, requires approval from 2 validators
	specifications.ValidatorNodeRemoveSpecificationV4R1(ctx, t, n0Admin, n1Admin, n2Admin, p.Nodes[3].PrivateKey())

	// Node3 is no longer a validator
	specifications.CurrentValidatorsSpecification(ctx, t, n0Admin, 3)

	// leader can't be removed from the validator set
	specifications.InvalidRemovalSpecification(ctx, t, n1Admin, p.Nodes[0].PrivateKey())
}

func TestKwildBlockSync(t *testing.T) {
	p := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.DefaultNodeConfig(),
				setup.DefaultNodeConfig(),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Validator = false
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Validator = false
				}),
			},
			DBOwner: "0xabc",
		},
		InitialServices: []string{"node0", "node1", "node2", "pg0", "pg1", "pg2"}, // should bringup only node 0,1,2
	})
	ctx := context.Background()
	// wait for all the nodes to discover each other
	time.Sleep(2 * time.Second)

	// bring up node3, pg3 and ensure that it does blocksync correctly
	p.RunServices(t, ctx, []*setup.ServiceDefinition{
		setup.KwildServiceDefinition("node3"),
		setup.PostgresServiceDefinition("pg3"),
	})

	// time for node to blocksync and catch up
	time.Sleep(4 * time.Second)

	// ensure that all nodes are in sync
	info, err := p.Nodes[3].JSONRPCClient(t, ctx, false, "").ChainInfo(ctx)
	require.NoError(t, err)
	require.Greater(t, info.BlockHeight, uint64(0))

	// TODO: Add some kind of data verification here
}

func TestStatesync(t *testing.T) {
	/*
		Node 1, 2, 3 has snapshots enabled
		Node 4 tries to sync with the network, with statesync enabled.
		Node4 should be able to sync with the network and catch up with the latest state (maybe check for the existence of some kind of data)
	*/
	p := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Configure = func(conf *config.Config) {
						conf.Snapshots.Enable = true
						conf.Snapshots.RecurringHeight = 50
					}
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Configure = func(conf *config.Config) {
						conf.Snapshots.Enable = true
						conf.Snapshots.RecurringHeight = 50
					}
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Validator = false
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Validator = false
					nc.Configure = func(conf *config.Config) {
						conf.StateSync.Enable = true
						// conf.StateSync.TrustedProviders = conf.P2P.BootNodes
					}
				}),
			},
			DBOwner: "0xabc",
		},
		ContainerStartTimeout: 2 * time.Minute,                                          // increase the timeout for statesync, it generally doesn't take this long, docker can be slow
		InitialServices:       []string{"node0", "node1", "node2", "pg0", "pg1", "pg2"}, // should bringup only node 0,1,2
	})
	ctx := context.Background()

	// wait for all the nodes to discover each other and to produce snapshots
	time.Sleep(50 * time.Second)

	// bring up node3, pg3 and ensure that it does blocksync correctly
	p.RunServices(t, ctx, []*setup.ServiceDefinition{
		setup.KwildServiceDefinition("node3"),
		setup.PostgresServiceDefinition("pg3"),
	})

	// time for node to blocksync and catch up
	time.Sleep(4 * time.Second)

	// ensure that all nodes are in sync
	info, err := p.Nodes[3].JSONRPCClient(t, ctx, false, "").ChainInfo(ctx)
	require.NoError(t, err)
	require.Greater(t, info.BlockHeight, uint64(50))

	// TODO: Add some kind of data verification here
}

func TestLongRunningNetworkMigrations(t *testing.T) {
	net1 := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.DefaultNodeConfig(),
				setup.DefaultNodeConfig(),
				setup.DefaultNodeConfig(),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Validator = false
				}),
			},
			DBOwner: "0xabc",
		},
	})

	ctx := context.Background()
	time.Sleep(2 * time.Second)

	// rpc listen addresses of the nodes?

	// Deploy some tables and insert some data

	// Trigger a network migration request

	var listenAddresses []string
	for _, node := range net1.Nodes {
		_, addr, err := node.JSONRPCEndpoint(t, ctx)
		require.NoError(t, err)
		listenAddresses = append(listenAddresses, addr)
	}

	n0Admin := net1.Nodes[0].AdminClient(t, ctx)
	n1Admin := net1.Nodes[1].AdminClient(t, ctx)
	// n2Admin := net1.Nodes[2].AdminClient(t, ctx)
	n3Admin := net1.Nodes[3].AdminClient(t, ctx)

	specifications.SubmitMigrationProposal(ctx, t, n0Admin)

	// node1 approves the migration again and verifies that the migration is still pending
	specifications.ApproveMigration(ctx, t, n0Admin, true)

	// non validator can't approve the migration
	specifications.NonValidatorApproveMigration(ctx, t, n3Admin)

	// node1 approves the migration and verifies that the migration is no longer pending
	specifications.ApproveMigration(ctx, t, n1Admin, false)

	// Setup a new network with the same keys and enter the activation phase
	net2 := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.PrivateKey = net1.Nodes[0].PrivateKey()
					nc.Configure = func(conf *config.Config) {
						conf.Migrations.Enable = true
						conf.Migrations.MigrateFrom = listenAddresses[0]

						conf.Snapshots.Enable = true
						conf.Snapshots.RecurringHeight = 25
					}
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.PrivateKey = net1.Nodes[1].PrivateKey()
					nc.Configure = func(conf *config.Config) {
						conf.Migrations.Enable = true
						conf.Migrations.MigrateFrom = listenAddresses[1]

						conf.Snapshots.Enable = true
						conf.Snapshots.RecurringHeight = 25
					}
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.PrivateKey = net1.Nodes[2].PrivateKey()
					nc.Configure = func(conf *config.Config) {
						conf.Migrations.Enable = true
						conf.Migrations.MigrateFrom = listenAddresses[2]
					}
					nc.Validator = false
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.PrivateKey = net1.Nodes[3].PrivateKey()
					nc.Configure = func(conf *config.Config) {
						conf.Migrations.Enable = true
						conf.Migrations.MigrateFrom = listenAddresses[3]

						conf.StateSync.Enable = true
					}
					nc.Validator = false
				}),
			},
			DBOwner: "0xabc",
		},
		InitialServices:       []string{"new-node0", "new-node1", "new-node2", "new-pg0", "new-pg1", "new-pg2"}, // should bringup only node 0,1,2
		ServicesPrefix:        "new-",
		PortOffset:            100,
		DockerNetwork:         net1.NetworkName(),
		ContainerStartTimeout: 2 * time.Minute, // increase the timeout for downloading the genesis state and starting migration
	})

	// Verify the existence of some data

	// time for node to do blocksync and catchup
	// net2.RunServices(t, ctx, []*setup.ServiceDefinition{
	// 	setup.KwildServiceDefinition("new-node2"),
	// 	setup.KwildServiceDefinition("new-pg2"),
	// })

	// time for node to do statesync and catchup
	net2.RunServices(t, ctx, []*setup.ServiceDefinition{
		setup.KwildServiceDefinition("new-node3"),
		setup.KwildServiceDefinition("new-pg3"),
	})

	// time for node to blocksync and catch up
	time.Sleep(4 * time.Second)

	// ensure that all nodes are in sync
	info, err := net2.Nodes[3].JSONRPCClient(t, ctx, false, "").ChainInfo(ctx)
	require.NoError(t, err)
	require.Greater(t, info.BlockHeight, uint64(50)) // TODO: height > 50 + migration height
}

func TestSingleNodeKwildEthDepositsOracleIntegration(t *testing.T) {
	ctx := context.Background()

	dockerNetwork, err := setup.CreateDockerNetwork(ctx, t)
	require.NoError(t, err)

	// deploy the hardhat service
	// I couldn't easily integrate it into Setup tests, as we need to first run the
	// eth node and deploy contracts and use these contracts to configure and run the
	// kwild nodes. So, I am keeping the deployment separate for now.
	ethNode := setup.DeployETHNode(t, ctx, dockerNetwork.Name)
	require.NotNil(t, ethNode)

	// ensure that both the contracts are deployed
	require.Equal(t, 2, len(ethNode.Deployers))
	deployer := ethNode.Deployers[0]

	// start mining
	deployerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = deployer.KeepMining(deployerCtx)
	require.NoError(t, err)

	privk, err := secp256k1.GeneratePrivateKeyFromRand(rand.Reader)
	require.NoError(t, err)
	privKey := (*crypto.Secp256k1PrivateKey)(privk)
	signer := &auth.Secp256k1Signer{
		Secp256k1PrivateKey: *privKey,
	}
	addr := signer.CompactID()

	// Configure Kwil nodes to use the deployed contracts
	p := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Configure = func(conf *config.Config) {
						conf.Extensions = make(map[string]map[string]string)
						cfg := ethdeposits.EthDepositConfig{
							RPCProvider:          ethNode.UnexposedChainRPC,
							ContractAddress:      deployer.EscrowAddress(),
							StartingHeight:       0,
							ReconnectionInterval: 30,
							MaxRetries:           20,
							BlockSyncChunkSize:   1000,
						}
						conf.Extensions["eth_deposits"] = cfg.Map()
					}
					nc.PrivateKey = privKey
				}),
			},
			DBOwner: setup.OwnerAddress,
			ConfigureGenesis: func(genDoc *config.GenesisConfig) {
				genDoc.DisabledGasCosts = false
				alloc := config.GenesisAlloc{
					ID:      hex.EncodeToString(addr),
					KeyType: "secp256k1",
					Amount:  big.NewInt(100000000000000),
				}
				genDoc.Allocs = append(genDoc.Allocs, alloc)
			},
		},
		DockerNetwork: dockerNetwork.Name,
	})

	fmt.Println(ethNode.Deployers[0].EscrowAddress(), ethNode.Deployers[1].EscrowAddress())

	// Deposit the amount to the escrow
	specifications.ApproveSpecification(ctx, t, deployer)
	amount := big.NewInt(10)

	rpcClient := p.Nodes[0].JSONRPCClient(t, ctx, false, setup.UserPubKey1)

	// execute sql statement without enough balance
	specifications.DeployDbInsufficientFundsSpecification(ctx, t, deployer, rpcClient)

	specifications.DepositSuccessSpecification(ctx, t, deployer, rpcClient, amount)

}

// TestKwildEthDepositFundTransfer tests out the ways in which the validator accounts can be funded
// One way is during network bootstrapping using allocs in the genesis file
// Other, is through transfer from one kwil account to another
func TestKwildEthDepositFundTransfer(t *testing.T) {
	ctx := context.Background()

	dockerNetwork, err := setup.CreateDockerNetwork(ctx, t)
	require.NoError(t, err)

	// deploy the hardhat service
	// I couldn't easily integrate it into Setup tests, as we need to first run the
	// eth node and deploy contracts and use these contracts to configure and run the
	// kwild nodes. So, I am keeping the deployment separate for now.
	ethNode := setup.DeployETHNode(t, ctx, dockerNetwork.Name)
	require.NotNil(t, ethNode)

	// ensure that both the contracts are deployed
	require.Equal(t, 2, len(ethNode.Deployers))
	deployer := ethNode.Deployers[0]

	// start mining
	deployerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = deployer.KeepMining(deployerCtx)
	require.NoError(t, err)

	privk, err := secp256k1.GeneratePrivateKeyFromRand(rand.Reader)
	require.NoError(t, err)
	privKey := (*crypto.Secp256k1PrivateKey)(privk)
	signer := &auth.Secp256k1Signer{
		Secp256k1PrivateKey: *privKey,
	}
	addr := signer.CompactID()

	// Configure Kwil nodes to use the deployed contracts
	p := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Configure = func(conf *config.Config) {
						conf.Extensions = make(map[string]map[string]string)
						cfg := ethdeposits.EthDepositConfig{
							RPCProvider:          ethNode.UnexposedChainRPC,
							ContractAddress:      deployer.EscrowAddress(),
							StartingHeight:       0,
							ReconnectionInterval: 30,
							MaxRetries:           20,
							BlockSyncChunkSize:   1000,
						}
						conf.Extensions["eth_deposits"] = cfg.Map()
					}
					nc.PrivateKey = privKey
				}),
			},
			DBOwner: setup.OwnerAddress,
			ConfigureGenesis: func(genDoc *config.GenesisConfig) {
				genDoc.DisabledGasCosts = false
				alloc := config.GenesisAlloc{
					ID:      hex.EncodeToString(addr),
					KeyType: "secp256k1",
					Amount:  big.NewInt(100000000000000),
				}
				genDoc.Allocs = append(genDoc.Allocs, alloc)
			},
		},
		DockerNetwork: dockerNetwork.Name,
	})

	fmt.Println(ethNode.Deployers[0].EscrowAddress(), ethNode.Deployers[1].EscrowAddress())

	clt := p.Nodes[0].JSONRPCClient(t, ctx, false, setup.UserPubKey1)

	specifications.FundValidatorSpecification(ctx, t, deployer, clt, privKey)
}

func TestKwildEthDepositOracleValidatorUpdates(t *testing.T) {
	ctx := context.Background()

	dockerNetwork, err := setup.CreateDockerNetwork(ctx, t)
	require.NoError(t, err)

	// deploy the hardhat service
	// I couldn't easily integrate it into Setup tests, as we need to first run the
	// eth node and deploy contracts and use these contracts to configure and run the
	// kwild nodes. So, I am keeping the deployment separate for now.
	ethNode := setup.DeployETHNode(t, ctx, dockerNetwork.Name)
	require.NotNil(t, ethNode)

	// ensure that both the contracts are deployed
	require.Equal(t, 2, len(ethNode.Deployers))
	deployer := ethNode.Deployers[0]

	// start mining
	deployerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = deployer.KeepMining(deployerCtx)
	require.NoError(t, err)

	var validators []*crypto.Secp256k1PrivateKey
	for i := 0; i < 3; i++ {
		privk, err := secp256k1.GeneratePrivateKeyFromRand(rand.Reader)
		require.NoError(t, err)
		privKey := (*crypto.Secp256k1PrivateKey)(privk)
		validators = append(validators, privKey)
	}

	ethConfig := ethdeposits.EthDepositConfig{
		RPCProvider:          ethNode.UnexposedChainRPC,
		ContractAddress:      deployer.EscrowAddress(),
		StartingHeight:       0,
		ReconnectionInterval: 30,
		MaxRetries:           20,
		BlockSyncChunkSize:   1000,
	}

	extFn := func(conf *config.Config) {
		conf.Extensions = make(map[string]map[string]string)
		cfg := ethConfig
		conf.Extensions["eth_deposits"] = cfg.Map()
	}

	byzFn := func(conf *config.Config) {
		conf.Extensions = make(map[string]map[string]string)
		cfg := ethConfig
		cfg.ContractAddress = ethNode.Deployers[1].EscrowAddress()
		conf.Extensions["eth_deposits"] = cfg.Map()
	}

	// Configure Kwil nodes to use the deployed contracts
	p := setup.SetupTests(t, &setup.TestConfig{
		ClientDriver: setup.CLI,
		Network: &setup.NetworkConfig{
			Nodes: []*setup.NodeConfig{
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Configure = extFn
					nc.PrivateKey = validators[0]
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Configure = byzFn
					nc.PrivateKey = validators[1]
				}),
				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
					nc.Configure = extFn
					nc.PrivateKey = validators[2]
				}),
			},
			DBOwner: setup.OwnerAddress,
			ConfigureGenesis: func(genDoc *config.GenesisConfig) {
				genDoc.DisabledGasCosts = true
			},
		},
		DockerNetwork: dockerNetwork.Name,
	})

	rpcClients := make([]setup.JSONRPCClient, 3)
	adminClients := make([]*setup.AdminClient, 3)
	for i := 0; i < 3; i++ {
		rpcClients[i] = p.Nodes[i].JSONRPCClient(t, ctx, false, setup.UserPubKey1)
		adminClients[i] = p.Nodes[i].AdminClient(t, ctx)
	}

	// Deposit the amount to the escrow and verify the balance
	amount := big.NewInt(10)
	specifications.EthDepositSpecification(ctx, t, deployer, rpcClients[0], amount, false)

	// Node2 leaves it's validator role
	specifications.ValidatorNodeLeaveSpecification(ctx, t, adminClients[2])

	// verify that the validator set has been updated
	specifications.CurrentValidatorsSpecification(ctx, t, adminClients[0], 2)

	// make a deposit to the escrow, it should not be successful
	specifications.EthDepositSpecification(ctx, t, deployer, rpcClients[0], amount, true)

	// Node2 rejoins the network as a validator
	// And catches up with all the events it missed and votes for the observed events
	// The last deposit should now get approved and credited to the account
	specifications.ValidatorNodeJoinSpecification(ctx, t, adminClients[2], validators[2], 2)

	// 2 validators should approve the join request
	specifications.ValidatorNodeApproveSpecification(ctx, t, adminClients[0], validators[2], 2, 2, false)

	// get the balance of the account
	sender, err := specifications.SenderAccountID(t)
	require.NoError(t, err)

	acct1, err := rpcClients[0].GetAccount(ctx, sender, 0)
	require.NoError(t, err)

	// second approval
	specifications.ValidatorNodeApproveSpecification(ctx, t, adminClients[1], validators[2], 2, 3, true)
	specifications.CurrentValidatorsSpecification(ctx, t, adminClients[0], 3)

	// ensure that the balance has been updated
	require.Eventually(t, func() bool {
		acct2, err := rpcClients[0].GetAccount(ctx, sender, 0)
		require.NoError(t, err)
		return acct2.Balance.Cmp(acct1.Balance) == 1
	}, 60*time.Second, 3*time.Second)
}

// TODO: There is no straightforward way to test the oracle expiry and refund
// as we can't update the resolution expiry on fly. Can run these two tests with a
// custom build with a very short Expiration period.
// func TestKwildEthDepositOracleExpiryIntegration(t *testing.T) {
// 	ctx := context.Background()

// 	dockerNetwork, err := setup.CreateDockerNetwork(ctx, t)
// 	require.NoError(t, err)
// 	ethNode := setup.DeployETHNode(t, ctx, dockerNetwork.Name)
// 	require.NotNil(t, ethNode)

// 	// ensure that both the contracts are deployed
// 	require.Equal(t, 2, len(ethNode.Deployers))
// 	fmt.Println(ethNode.Deployers[0].EscrowAddress(), ethNode.Deployers[1].EscrowAddress())

// 	deployer := ethNode.Deployers[1]

// 	// start mining on the second contract and only one node will be
// 	// listening on the contract -> node1
// 	deployerCtx, cancel := context.WithCancel(ctx)
// 	defer cancel()
// 	err = deployer.KeepMining(deployerCtx)
// 	require.NoError(t, err)

// 	var validators []*crypto.Secp256k1PrivateKey
// 	for i := 0; i < 4; i++ {
// 		privk, err := secp256k1.GeneratePrivateKeyFromRand(rand.Reader)
// 		require.NoError(t, err)
// 		privKey := (*crypto.Secp256k1PrivateKey)(privk)
// 		validators = append(validators, privKey)
// 	}

// 	extFn := func(conf *config.Config) {
// 		conf.Extensions = make(map[string]map[string]string)
// 		cfg := ethdeposits.EthDepositConfig{
// 			RPCProvider:          ethNode.UnexposedChainRPC,
// 			ContractAddress:      ethNode.Deployers[0].EscrowAddress(),
// 			StartingHeight:       0,
// 			ReconnectionInterval: 30,
// 			MaxRetries:           20,
// 			BlockSyncChunkSize:   1000,
// 		}
// 		conf.Extensions["eth_deposits"] = cfg.Map()
// 	}

// 	byzFn := func(conf *config.Config) {
// 		conf.Extensions = make(map[string]map[string]string)
// 		cfg := ethdeposits.EthDepositConfig{
// 			RPCProvider:          ethNode.UnexposedChainRPC,
// 			ContractAddress:      ethNode.Deployers[1].EscrowAddress(),
// 			StartingHeight:       0,
// 			ReconnectionInterval: 30,
// 			MaxRetries:           20,
// 			BlockSyncChunkSize:   1000,
// 		}
// 		conf.Extensions["eth_deposits"] = cfg.Map()
// 	}

// 	// Configure Kwil nodes to use the deployed contracts
// 	p := setup.SetupTests(t, &setup.TestConfig{
// 		ClientDriver: setup.CLI,
// 		Network: &setup.NetworkConfig{
// 			Nodes: []*setup.NodeConfig{
// 				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
// 					nc.Configure = byzFn // only leader can create resolutions, so it has to be listening on the byzantine escrow contract which none of the validators are listening on
// 					nc.PrivateKey = validators[0]
// 				}),
// 				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
// 					nc.Configure = extFn
// 					nc.PrivateKey = validators[1]
// 				}),
// 				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
// 					nc.Configure = extFn
// 					nc.PrivateKey = validators[2]
// 				}),
// 				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
// 					nc.Configure = extFn
// 					nc.PrivateKey = validators[3]
// 				}),
// 			},
// 			DBOwner: setup.OwnerAddress,
// 			ConfigureGenesis: func(genDoc *config.GenesisConfig) {
// 				genDoc.DisabledGasCosts = false
// 				genDoc.VoteExpiry = 10
// 				for _, val := range validators {
// 					alloc := config.GenesisAlloc{
// 						ID:      hex.EncodeToString(val.Public().Bytes()),
// 						Amount:  big.NewInt(100000000000000),
// 						KeyType: "secp256k1",
// 					}
// 					genDoc.Allocs = append(genDoc.Allocs, alloc)
// 				}
// 			},
// 		},
// 		DockerNetwork: dockerNetwork.Name,
// 	})
// 	fmt.Println(ethNode.Deployers[0].EscrowAddress(), ethNode.Deployers[1].EscrowAddress())

// 	clt := p.Nodes[0].JSONRPCClient(t, ctx, false, setup.UserPubKey1)
// 	specifications.DepositResolutionExpirySpecification(ctx, t, deployer, clt, validators)
// }

// func TestKwildEthDepositOracleExpiryRefundIntegration(t *testing.T) {
// 	ctx := context.Background()

// 	dockerNetwork, err := setup.CreateDockerNetwork(ctx, t)
// 	require.NoError(t, err)
// 	ethNode := setup.DeployETHNode(t, ctx, dockerNetwork.Name)
// 	require.NotNil(t, ethNode)

// 	// ensure that both the contracts are deployed
// 	require.Equal(t, 2, len(ethNode.Deployers))
// 	deployer := ethNode.Deployers[0]

// 	// start mining
// 	deployerCtx, cancel := context.WithCancel(ctx)
// 	defer cancel()
// 	err = deployer.KeepMining(deployerCtx)
// 	require.NoError(t, err)

// 	var validators []*crypto.Secp256k1PrivateKey
// 	for i := 0; i < 4; i++ {
// 		privk, err := secp256k1.GeneratePrivateKeyFromRand(rand.Reader)
// 		require.NoError(t, err)
// 		privKey := (*crypto.Secp256k1PrivateKey)(privk)
// 		validators = append(validators, privKey)
// 	}

// 	ethCfg := ethdeposits.EthDepositConfig{
// 		RPCProvider:          ethNode.UnexposedChainRPC,
// 		ContractAddress:      deployer.EscrowAddress(),
// 		StartingHeight:       0,
// 		ReconnectionInterval: 30,
// 		MaxRetries:           20,
// 		BlockSyncChunkSize:   1000,
// 	}

// 	extFn := func(conf *config.Config) {
// 		conf.Extensions = make(map[string]map[string]string)
// 		cfg := ethCfg
// 		conf.Extensions["eth_deposits"] = cfg.Map()
// 	}

// 	byzFn := func(conf *config.Config) {
// 		conf.Extensions = make(map[string]map[string]string)
// 		cfg := ethCfg
// 		cfg.ContractAddress = ethNode.Deployers[1].EscrowAddress()
// 		conf.Extensions["eth_deposits"] = cfg.Map()
// 	}

// 	// Configure Kwil nodes to use the deployed contracts
// 	p := setup.SetupTests(t, &setup.TestConfig{
// 		ClientDriver: setup.CLI,
// 		Network: &setup.NetworkConfig{
// 			Nodes: []*setup.NodeConfig{
// 				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
// 					nc.Configure = extFn
// 					nc.PrivateKey = validators[0]
// 				}),
// 				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
// 					nc.Configure = extFn
// 					nc.PrivateKey = validators[1]
// 				}),
// 				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
// 					nc.Configure = byzFn
// 					nc.PrivateKey = validators[2]
// 				}),
// 				setup.CustomNodeConfig(func(nc *setup.NodeConfig) {
// 					nc.Configure = byzFn
// 					nc.PrivateKey = validators[3]
// 				}),
// 			},
// 			DBOwner: setup.OwnerAddress,
// 			ConfigureGenesis: func(genDoc *config.GenesisConfig) {
// 				genDoc.DisabledGasCosts = false
// 				genDoc.VoteExpiry = 10
// 				for _, val := range validators {
// 					alloc := config.GenesisAlloc{
// 						ID:      hex.EncodeToString(val.Public().Bytes()),
// 						Amount:  big.NewInt(100000000000000),
// 						KeyType: "secp256k1",
// 					}
// 					genDoc.Allocs = append(genDoc.Allocs, alloc)
// 				}
// 			},
// 		},
// 		DockerNetwork: dockerNetwork.Name,
// 	})

// 	clt := p.Nodes[0].JSONRPCClient(t, ctx, false, setup.UserPubKey1)

// 	// 4 nodes -> 2 listening on escrow contract 1 and 2 listening on byzantine contract
// 	// 2 validators approve the deposit, but the deposit is not resolved
// 	// 2 validators get the refund after expiry
// 	specifications.DepositResolutionExpiryRefundSpecification(ctx, t, deployer, clt, validators)
// }
