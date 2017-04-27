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

package simulation

import (
	"bytes"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

const (
	extraVanity = 32 // Fixed number of extra-data prefix bytes reserved for signer vanity
	extraSeal   = 65 // Fixed number of extra-data suffix bytes reserved for signer seal
)

type mockChain struct {
	rwMutex *sync.RWMutex

	blocksByHash map[common.Hash]*types.Block
	blocks       []*types.Block
}

func newMockChain(genesis *types.Block) *mockChain {
	chain := &mockChain{
		rwMutex:      new(sync.RWMutex),
		blocksByHash: make(map[common.Hash]*types.Block),
		blocks:       make([]*types.Block, 0),
	}
	chain.InsertBlock(genesis)
	return chain
}

func (c *mockChain) Config() *params.ChainConfig {
	// not implemented
	return nil
}

func (c *mockChain) CurrentHeader() *types.Header {
	c.rwMutex.RLock()
	defer c.rwMutex.RUnlock()

	return c.blocks[len(c.blocks)-1].Header()
}

func (c *mockChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	return c.GetHeaderByHash(hash)
}

func (c *mockChain) GetHeaderByNumber(number uint64) *types.Header {
	c.rwMutex.RLock()
	defer c.rwMutex.RUnlock()

	return c.blocks[number].Header()
}

func (c *mockChain) GetHeaderByHash(hash common.Hash) *types.Header {
	c.rwMutex.RLock()
	defer c.rwMutex.RUnlock()

	return c.blocksByHash[hash].Header()
}

func (c *mockChain) GetBlock(hash common.Hash, number uint64) *types.Block {
	c.rwMutex.RLock()
	defer c.rwMutex.RUnlock()

	return c.blocksByHash[hash]
}

func (c *mockChain) InsertBlock(block *types.Block) error {
	c.rwMutex.Lock()
	defer c.rwMutex.Unlock()

	if block.Number().Uint64() == uint64(len(c.blocks)) {
		c.blocks = append(c.blocks, block)
		c.blocksByHash[block.Hash()] = block

	}
	return nil
}

func AppendValidators(header *types.Header, addrs []common.Address) *types.Block {
	if len(header.Extra) < extraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, extraVanity)...)
	}
	header.Extra = header.Extra[:extraVanity]

	for _, addr := range addrs {
		header.Extra = append(header.Extra, addr[:]...)
	}
	header.Extra = append(header.Extra, make([]byte, extraSeal)...)
	return types.NewBlockWithHeader(header)
}
