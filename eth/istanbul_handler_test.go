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
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	lru "github.com/hashicorp/golang-lru"
)

func TestIstanbulMessage(t *testing.T) {
	var (
		evmux  = new(event.TypeMux)
		engine = &MockIstanbulEngine{}
		db, _  = ethdb.NewMemDatabase()
		config = &params.ChainConfig{DAOForkBlock: big.NewInt(1)}
		gspec  = &core.Genesis{Config: config}
	)
	gspec.MustCommit(db)
	blockchain, _ := core.NewBlockChain(db, config, engine, evmux, vm.Config{})
	p, err := NewProtocolManager(config, downloader.FullSync, DefaultConfig.NetworkId, 1000, evmux, new(testTxPool), engine, blockchain, db)
	if err != nil {
		t.Fatalf("failed to start test protocol manager: %v", err)
	}
	pm, ok := p.(*istanbulProtocolManager)
	if !ok {
		panic("cast istanbulProtocolManager failed")
	}

	// generate one msg
	data := []byte("data1")
	hash := istanbul.RLPHash(data)
	msg := makeMsg(IstanbulMsg, data)
	peer := newPeer(IstanbulVersion, p2p.NewPeer(randomID(), "name", []p2p.Cap{}), nil)
	pubKey, _ := peer.ID().Pubkey()
	addr := crypto.PubkeyToAddress(*pubKey)

	// 1. this message should not be in cache
	// for peers
	if _, ok := pm.recentMessages.Get(addr); ok {
		t.Fatalf("the cache of messages for this peer should be nil")
	}

	// for self
	if _, ok := pm.knownMessages.Get(hash); ok {
		t.Fatalf("the cache of messages should be nil")
	}

	// 2. this message should be in cache after we handle it
	err = pm.handleMsg(peer, msg)
	if err != nil {
		t.Fatalf("handle message failed: %v", err)
	}
	// for peers
	if ms, ok := pm.recentMessages.Get(addr); ms == nil || !ok {
		t.Fatalf("the cache of messages for this peer cannot be nil")
	} else if m, ok := ms.(*lru.ARCCache); !ok {
		t.Fatalf("the cache of messages for this peer cannot be casted")
	} else if _, ok := m.Get(hash); !ok {
		t.Fatalf("the cache of messages for this peer cannot be found")
	}

	// for self
	if _, ok := pm.knownMessages.Get(hash); !ok {
		t.Fatalf("the cache of messages cannot be found")
	}
}

func randomID() (id discover.NodeID) {
	key, _ := crypto.GenerateKey()
	return discover.PubkeyID(&key.PublicKey)
}

func makeMsg(msgcode uint64, data interface{}) p2p.Msg {
	size, r, _ := rlp.EncodeToReader(data)
	return p2p.Msg{Code: msgcode, Size: uint32(size), Payload: r}
}

type MockIstanbulEngine struct{}

func (m *MockIstanbulEngine) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

func (m *MockIstanbulEngine) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return nil
}

func (m *MockIstanbulEngine) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for _ = range headers {
			results <- nil
		}
	}()
	return abort, results
}

func (m *MockIstanbulEngine) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	return nil
}

func (m *MockIstanbulEngine) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	return nil
}

func (m *MockIstanbulEngine) Prepare(chain consensus.ChainReader, header *types.Header) error {
	return nil
}

func (m *MockIstanbulEngine) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	return nil, nil
}

func (m *MockIstanbulEngine) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	return nil, nil
}

func (m *MockIstanbulEngine) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{}
}

func (m *MockIstanbulEngine) HandleMsg(addr common.Address, data []byte) error {
	return nil
}

func (m *MockIstanbulEngine) NewChainHead(block *types.Block) error {
	return nil
}

func (m *MockIstanbulEngine) Start(chain consensus.ChainReader, inserter func(types.Blocks) (int, error)) error {
	return nil
}

func (m *MockIstanbulEngine) Stop() error {
	return nil
}
