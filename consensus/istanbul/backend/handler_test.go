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

package backend

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	lru "github.com/hashicorp/golang-lru"
)

func TestIstanbulMessage(t *testing.T) {
	_, backend := newBlockChain(1)

	// generate one msg
	data := []byte("data1")
	hash := istanbul.RLPHash(data)
	msg := makeMsg(istanbulMsg, data)
	addr := common.StringToAddress("address")

	// 1. this message should not be in cache
	// for peers
	if _, ok := backend.recentMessages.Get(addr); ok {
		t.Fatalf("the cache of messages for this peer should be nil")
	}

	// for self
	if _, ok := backend.knownMessages.Get(hash); ok {
		t.Fatalf("the cache of messages should be nil")
	}

	// 2. this message should be in cache after we handle it
	_, err := backend.HandleMsg(addr, msg)
	if err != nil {
		t.Fatalf("handle message failed: %v", err)
	}
	// for peers
	if ms, ok := backend.recentMessages.Get(addr); ms == nil || !ok {
		t.Fatalf("the cache of messages for this peer cannot be nil")
	} else if m, ok := ms.(*lru.ARCCache); !ok {
		t.Fatalf("the cache of messages for this peer cannot be casted")
	} else if _, ok := m.Get(hash); !ok {
		t.Fatalf("the cache of messages for this peer cannot be found")
	}

	// for self
	if _, ok := backend.knownMessages.Get(hash); !ok {
		t.Fatalf("the cache of messages cannot be found")
	}
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
