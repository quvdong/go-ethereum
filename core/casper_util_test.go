// Copyright 2018 The go-ethereum Authors
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

package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

var (
	// Global values used to control how mockCasper returns GetLastJustifiedEpoch in succession, assuming
	// the code queries last justified epoch of the current block first and the new block second
	globalJustified      = big.NewInt(1234)
	globalJustifiedIncBy = big.NewInt(0)
)

type mockCasper struct {
	justified *big.Int     // last justified epoch
	finalized *types.Block // last finalized block
}

// mockCasperGen returns a func which can generate a mocked Capser instance for testing
func mockCasperGen(finalized *types.Block) func(bind.ContractBackend) (contracts.Casper, error) {
	return func(backend bind.ContractBackend) (contracts.Casper, error) {
		globalJustified.Add(globalJustified, globalJustifiedIncBy)
		return &mockCasper{
			justified: new(big.Int).Set(globalJustified),
			finalized: finalized,
		}, nil
	}
}

// GetLastJustifiedEpoch implements Casper interface
func (c *mockCasper) GetLastJustifiedEpoch() (*big.Int, error) {
	return c.justified, nil
}

// GetLastFinalizedEpoch implements Casper interface
func (c *mockCasper) GetLastFinalizedEpoch() (*big.Int, error) {
	return c.finalized.Number(), nil
}

// GetCheckpointHashes implements Casper interface
func (c *mockCasper) GetCheckpointHashes(number *big.Int) ([32]byte, error) {
	return c.finalized.Hash(), nil
}

// Constructs a chain from block 1 to 10, with head at block 10, and a side chain of 3 blocks (6', 7', and 8') rooted
// at block 5, with the last Casper finalized block at block number 'lastFinalized'.
// If 'directChild' is true, test if Test if Casper will accept block 10 as head.
// Otherwise test if Casper will accept block 8' as head.
func testNewBlockWithSideChain(t *testing.T, tdDiff int64, justifiedIncBy int64, lastFinalized uint64, directChild bool, shouldAccept bool) {
	// Configure and generate a sample block chain
	var (
		gendb, _ = ethdb.NewMemDatabase()
		key, _   = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address  = crypto.PubkeyToAddress(key.PublicKey)
		funds    = big.NewInt(1000000000)
		gspec    = &Genesis{
			Config: params.TestChainConfig,
			Alloc:  GenesisAlloc{address: {Balance: funds}},
		}
		genesis = gspec.MustCommit(gendb)
	)
	blockchain, _ := NewBlockChain(gendb, nil, gspec.Config, ethash.NewFaker(), vm.Config{})
	defer blockchain.Stop()

	// main chain with the last finalized block
	blocks := makeBlockChain(genesis, 10, ethash.NewFaker(), gendb, forkSeed)
	blocksToInsert := blocks[:]
	if directChild {
		blocksToInsert = blocks[:9]
	}
	if _, err := blockchain.InsertChain(blocksToInsert); err != nil {
		t.Fatalf("failed to insert chain: %v", err)
	}

	// side chain
	sideBlocks := makeBlockChain(blockchain.GetBlockByNumber(5), 3, ethash.NewFaker(), gendb, forkSeed)
	blocksToInsert = sideBlocks[:2]
	if directChild {
		blocksToInsert = sideBlocks[:]
	}
	if _, err := blockchain.InsertChain(blocksToInsert); err != nil {
		t.Fatalf("failed to insert side chain: %v", err)
	}

	// determine new block
	blockchain.SetCasperGen(mockCasperGen(blockchain.GetBlockByNumber(lastFinalized)))
	newBlock := sideBlocks[2]
	if directChild {
		newBlock = blocks[9]
	}

	// determine TD for the new block
	currentBlock := blockchain.CurrentBlock()
	currentTd := blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64())
	newTd := new(big.Int).Add(currentTd, new(big.Int).SetInt64(tdDiff))

	globalJustifiedIncBy = big.NewInt(justifiedIncBy)
	result, err := blockchain.acceptNewCasperBlock(newBlock, newTd)
	if err != nil {
		t.Errorf("casper failed to decide on new block: %v", err)
	}
	if result != shouldAccept {
		t.Errorf("casper shouldAccept new block %v, expected %v", result, shouldAccept)
	}
}

func TestAcceptNewCasperBlock(t *testing.T) {
	// Simple case where the new block is child of the current head => ACCEPT
	testNewBlockWithSideChain(t, 1, 0, 8, true, true)

	// higher TD, same LastJustifiedEpoch, common last finalized block => ACCEPT
	testNewBlockWithSideChain(t, 1, 0, 4, false, true)
	// same TD, higher LastJustifiedEpoch, common last finalized block => ACCEPT
	testNewBlockWithSideChain(t, 0, 1, 4, false, true)

	// higher TD, same LastJustifiedEpoch, but revert the last finalized block => REJECT
	testNewBlockWithSideChain(t, 1, 0, 8, false, false)
	// lower TD, same LastJustifiedEpoch, common last finalized block => REJECT
	testNewBlockWithSideChain(t, -1, 0, 4, false, false)
	// same TD, lower LastJustifiedEpoch, common last finalized block => REJECT
	testNewBlockWithSideChain(t, 0, -1, 4, false, false)
}
