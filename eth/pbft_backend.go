// Copyright 2017 AMIS Technologies
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package eth implements the Ethereum protocol.
package eth

import (
	"fmt"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/clique"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/gasprice"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

type PBFT struct {
	*Ethereum

	pbftProtocolManager *PBFTProtocolManager
}

func (s *PBFT) AddLesServer(ls LesServer) {
	s.lesServer = ls
	s.pbftProtocolManager.lesServer = ls
}

// New creates a new Ethereum object (including the
// initialisation of the common Ethereum object)
func NewPBFT(ctx *node.ServiceContext, config *Config) (*PBFT, error) {
	chainDb, err := CreateDB(ctx, config, "pbftchaindata")
	if err != nil {
		return nil, err
	}
	stopDbUpgrade := upgradeSequentialKeys(chainDb)
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised PBFT chain configuration", "config", chainConfig)

	eth := &Ethereum{
		chainDb:        chainDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         CreateConsensusEngine(ctx, config, chainConfig, chainDb),
		shutdownChan:   make(chan bool),
		stopDbUpgrade:  stopDbUpgrade,
		netVersionId:   config.NetworkId,
		etherbase:      config.Etherbase,
		MinerThreads:   config.MinerThreads,
		solcPath:       config.SolcPath,
	}
	pbft := &PBFT{Ethereum: eth}

	if err := addMipmapBloomBins(chainDb); err != nil {
		return nil, err
	}
	log.Info("Initialising PBFT protocol", "versions", PBFTProtocolVersions, "network", config.NetworkId)

	if !config.SkipBcVersionCheck {
		bcVersion := core.GetBlockChainVersion(chainDb)
		if bcVersion != core.BlockChainVersion && bcVersion != 0 {
			return nil, fmt.Errorf("Blockchain DB version mismatch (%d / %d). Run geth upgradedb.\n", bcVersion, core.BlockChainVersion)
		}
		core.WriteBlockChainVersion(chainDb, core.BlockChainVersion)
	}

	vmConfig := vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
	pbft.blockchain, err = core.NewBlockChain(chainDb, pbft.chainConfig, pbft.engine, pbft.eventMux, vmConfig)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		pbft.blockchain.SetHead(compat.RewindTo)
		core.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}

	newPool := core.NewTxPool(pbft.chainConfig, pbft.EventMux(), pbft.blockchain.State, pbft.blockchain.GasLimit)
	pbft.txPool = newPool

	maxPeers := config.MaxPeers
	if config.LightServ > 0 {
		// if we are running a light server, limit the number of ETH peers so that we reserve some space for incoming LES connections
		// temporary solution until the new peer connectivity API is finished
		halfPeers := maxPeers / 2
		maxPeers -= config.LightPeers
		if maxPeers < halfPeers {
			maxPeers = halfPeers
		}
	}

	if pbft.pbftProtocolManager, err = NewPBFTProtocolManager(pbft.chainConfig, config.FastSync, config.NetworkId, maxPeers, pbft.eventMux, pbft.txPool, pbft.engine, pbft.blockchain, chainDb); err != nil {
		return nil, err
	}

	pbft.miner = miner.New(pbft, pbft.chainConfig, pbft.EventMux(), pbft.engine)
	pbft.miner.SetGasPrice(config.GasPrice)
	pbft.miner.SetExtra(config.ExtraData)

	pbft.ApiBackend = &EthApiBackend{pbft.Ethereum, nil}
	gpoParams := gasprice.Config{
		Blocks:     config.GpoBlocks,
		Percentile: config.GpoPercentile,
		Default:    config.GasPrice,
	}
	pbft.ApiBackend.gpo = gasprice.NewOracle(pbft.ApiBackend, gpoParams)

	return pbft, nil
}

func (s *PBFT) SendVote() {
	s.pbftProtocolManager.BroadcastVote()
}

// APIs returns the collection of RPC services the ethereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *PBFT) APIs() []rpc.API {
	apis := ethapi.GetAPIs(s.ApiBackend, s.solcPath)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "pbft",
			Version:   "1.0",
			Service:   s,
			Public:    true,
		},
	}...)
}

func (s *PBFT) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *PBFT) Etherbase() (eb common.Address, err error) {
	if s.etherbase != (common.Address{}) {
		return s.etherbase, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			return accounts[0].Address, nil
		}
	}
	return common.Address{}, fmt.Errorf("etherbase address must be explicitly specified")
}

// set in js console via admin interface or wrapper from cli flags
func (self *PBFT) SetEtherbase(etherbase common.Address) {
	self.etherbase = etherbase
	self.miner.SetEtherbase(etherbase)
}

func (s *PBFT) StartMining(local bool) error {
	eb, err := s.Etherbase()
	if err != nil {
		log.Error("Cannot start mining without etherbase", "err", err)
		return fmt.Errorf("etherbase missing: %v", err)
	}
	if clique, ok := s.engine.(*clique.Clique); ok {
		wallet, err := s.accountManager.Find(accounts.Account{Address: eb})
		if wallet == nil || err != nil {
			log.Error("Etherbase account unavailable locally", "err", err)
			return fmt.Errorf("singer missing: %v", err)
		}
		clique.Authorize(eb, wallet.SignHash)
	}
	if local {
		// If local (CPU) mining is started, we can disable the transaction rejection
		// mechanism introduced to speed sync times. CPU mining on mainnet is ludicrous
		// so noone will ever hit this path, whereas marking sync done on CPU mining
		// will ensure that private networks work in single miner mode too.
		atomic.StoreUint32(&s.pbftProtocolManager.acceptTxs, 1)
	}
	go s.miner.Start(eb)
	return nil
}

func (s *PBFT) StopMining()         { s.miner.Stop() }
func (s *PBFT) IsMining() bool      { return s.miner.Mining() }
func (s *PBFT) Miner() *miner.Miner { return s.miner }

func (s *PBFT) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *PBFT) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *PBFT) TxPool() *core.TxPool               { return s.txPool }
func (s *PBFT) EventMux() *event.TypeMux           { return s.eventMux }
func (s *PBFT) Engine() consensus.Engine           { return s.engine }
func (s *PBFT) ChainDb() ethdb.Database            { return s.chainDb }
func (s *PBFT) IsListening() bool                  { return true } // Always listening
func (s *PBFT) EthVersion() int                    { return int(s.pbftProtocolManager.SubProtocols[0].Version) }
func (s *PBFT) NetVersion() int                    { return s.netVersionId }
func (s *PBFT) Downloader() *downloader.Downloader { return s.pbftProtocolManager.downloader }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *PBFT) Protocols() []p2p.Protocol {
	if s.lesServer == nil {
		return s.pbftProtocolManager.SubProtocols
	} else {
		return append(s.pbftProtocolManager.SubProtocols, s.lesServer.Protocols()...)
	}
}

// Start implements node.Service, starting all internal goroutines needed by the
// Ethereum protocol implementation.
func (s *PBFT) Start(srvr *p2p.Server) error {
	s.netRPCService = ethapi.NewPublicNetAPI(srvr, s.NetVersion())

	s.pbftProtocolManager.Start()
	if s.lesServer != nil {
		s.lesServer.Start(srvr)
	}
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Ethereum protocol.
func (s *PBFT) Stop() error {
	if s.stopDbUpgrade != nil {
		s.stopDbUpgrade()
	}
	s.blockchain.Stop()
	s.pbftProtocolManager.Stop()
	if s.lesServer != nil {
		s.lesServer.Stop()
	}
	s.txPool.Stop()
	s.miner.Stop()
	s.eventMux.Stop()

	s.chainDb.Close()
	close(s.shutdownChan)

	return nil
}

// This function will wait for a shutdown and resumes main thread execution
func (s *PBFT) WaitForShutdown() {
	<-s.shutdownChan
}
