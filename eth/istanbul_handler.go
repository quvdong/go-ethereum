// Copyright 2015 The go-ethereum Authors
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

package eth

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// support eth/63 protocol
	istanbulName           = "istanbul"
	istanbulVersion        = 64
	istanbulProtocolLength = 18

	IstanbulMsg = 0x11
)

type istanbulProtocolManager struct {
	*protocolManager

	engine   consensus.Istanbul
	eventSub *event.TypeMuxSubscription
}

func newIstanbulProtocolManager(config *params.ChainConfig, mode downloader.SyncMode, networkId uint64, maxPeers int, mux *event.TypeMux, txpool txPool, engine consensus.Istanbul, blockchain *core.BlockChain, chaindb ethdb.Database) (*istanbulProtocolManager, error) {
	// Create default protocol manager
	defaultManager, err := newProtocolManager(config, mode, networkId, maxPeers, mux, txpool, engine, blockchain, chaindb)
	if err != nil {
		return nil, err
	}

	// Create the istanbul protocol manager
	manager := &istanbulProtocolManager{
		protocolManager: defaultManager,
		engine:          engine,
	}

	// Overwrite SubProtocols to support only istanbul protocol
	manager.SubProtocols = []p2p.Protocol{
		p2p.Protocol{
			Name:    istanbulName,
			Version: istanbulVersion,
			Length:  istanbulProtocolLength,
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				peer := manager.newPeer(int(istanbulVersion), p, rw)
				select {
				case manager.newPeerCh <- peer:
					manager.wg.Add(1)
					defer manager.wg.Done()
					return manager.handle(peer, manager.handleMsg)
				case <-manager.quitSync:
					return p2p.DiscQuitting
				}
			},
			NodeInfo: func() interface{} {
				return manager.NodeInfo()
			},
			PeerInfo: func(id discover.NodeID) interface{} {
				if p := manager.peers.Peer(fmt.Sprintf("%x", id[:8])); p != nil {
					return p.Info()
				}
				return nil
			},
		},
	}

	return manager, nil
}

func (pm *istanbulProtocolManager) Start() {
	// receive the Istanbul event
	pm.eventSub = pm.eventMux.Subscribe(istanbul.ConsensusDataEvent{}, core.ChainHeadEvent{})
	go pm.eventLoop()
	pm.protocolManager.Start()
	pm.engine.Start(pm.protocolManager.blockchain, pm.commitBlock)
}

func (pm *istanbulProtocolManager) Stop() {
	log.Info("Stopping Ethereum protocol")
	pm.engine.Stop()
	pm.protocolManager.Stop()
	pm.eventSub.Unsubscribe() // quits eventLoop
}

// handleMsg overrides protocolManager.handleMsg()
func (pm *istanbulProtocolManager) handleMsg(p *peer, msg p2p.Msg) error {
	// Handle the message depending on its contents
	switch {
	case msg.Code == IstanbulMsg:
		pubKey, err := p.ID().Pubkey()
		if err != nil {
			return err
		}
		var data []byte
		if err := msg.Decode(&data); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		pm.engine.HandleMsg(pubKey, data)
	default:
		// Handle by default handler if the message isn't istanbul message
		return pm.protocolManager.handleMsg(p, msg)
	}
	return nil
}

// event loop for Istanbul
func (pm *istanbulProtocolManager) eventLoop() {
	// automatically stops if unsubscribe
	for obj := range pm.eventSub.Chan() {
		switch ev := obj.Data.(type) {
		case istanbul.ConsensusDataEvent:
			pm.sendEvent(ev)
		case core.ChainHeadEvent:
			pm.newHead(ev)
		}
	}
}

// event loop for Istanbul events
func (pm *istanbulProtocolManager) sendEvent(event istanbul.ConsensusDataEvent) {
	// FIXME: it's inefficient because retrieving all peers every time
	p := pm.findPeer(event.Target)
	if p == nil {
		log.Warn("Failed to find peer by address", "addr", event.Target)
		return
	}
	p2p.Send(p.rw, IstanbulMsg, event.Data)
}

func (pm *istanbulProtocolManager) commitBlock(block *types.Block) error {
	// TODO: find a better way to handle validator insert block
	if _, err := pm.blockchain.InsertChain(types.Blocks{block}); err != nil {
		log.Debug("Block insert failed", "number", block.Number(), "hash", block.Hash(), "err", err)
		return err
	}
	// Only announce the block, not broadcast it
	go pm.BroadcastBlock(block, false)
	return nil
}

func (pm *istanbulProtocolManager) newHead(event core.ChainHeadEvent) {
	block := event.Block
	if block != nil {
		pm.engine.NewChainHead(block)
	}
}

// findPeer retrieves all peers by given address
func (pm *istanbulProtocolManager) findPeer(addr common.Address) *peer {
	for _, p := range pm.peers.Peers() {
		pubKey, err := p.ID().Pubkey()
		if err != nil {
			continue
		}
		if crypto.PubkeyToAddress(*pubKey) == addr {
			return p
		}
	}
	return nil
}
