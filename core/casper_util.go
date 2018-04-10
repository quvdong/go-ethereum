// Copyright 2014 The go-ethereum Authors
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
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

var ErrNilCasperGen = errors.New("nil casper generator")

// SetCasperGen sets casperGen
func (bc *BlockChain) SetCasperGen(casperGen func(bind.ContractBackend) (contracts.Casper, error)) {
	bc.casperGen = casperGen
}

func (bc *BlockChain) AcceptNewCasperBlock(newBlock *types.Block, newTd *big.Int) (bool, error) {
	currentBlock := bc.CurrentBlock()
	currentTd := bc.GetTd(currentBlock.Hash(), currentBlock.NumberU64())
	currentScore, err := bc.getScore(currentBlock, currentTd)
	if err != nil {
		return false, err
	}
	newScore, err := bc.getScore(newBlock, newTd)
	if err != nil {
		return false, err
	}
	if newScore.Cmp(currentScore) > 0 {
		// Check if we will revert any finalized block in the Casper case
		safe, err := bc.safeForLastFinalizedBlock(newBlock)
		if err != nil {
			return false, err
		}
		return safe, nil
	}
	return false, nil
}

// getScore returns the score of a block from Casper's perspective
func (bc *BlockChain) getScore(block *types.Block, td *big.Int) (*big.Int, error) {
	score := new(big.Int).Set(td)
	casperScore, err := bc.getLastJustifiedEpoch(block)
	if err != nil {
		return nil, err
	}
	casperScale := new(big.Int).Exp(big.NewInt(10), big.NewInt(40), nil)
	casperScore.Mul(casperScore, casperScale)
	score.Add(score, casperScore)
	return score, nil
}

// getLastJustifiedEpoch returns the last justified epoch for a given block
func (bc *BlockChain) getLastJustifiedEpoch(block *types.Block) (*big.Int, error) {
	if bc.casperGen == nil {
		log.Warn("Casper generator is not initialized")
		return nil, ErrNilCasperGen
	}
	state, err := bc.StateAt(block.Root())
	if err != nil {
		return nil, err
	}
	stateBackend := NewStateBackend(block, state, bc)
	contract, err := bc.casperGen(stateBackend)
	if err != nil {
		log.Warn("Failed to get Casper contract", "err", err)
		return nil, err
	}
	justified, err := contract.GetLastJustifiedEpoch()
	if err != nil {
		log.Warn("Failed to get current chain status from Casper", "err", err)
		return nil, err
	}
	return justified, nil
}

// safeForLastFinalizedBlock returns true if the new head will NOT revert the last finalized block
func (bc *BlockChain) safeForLastFinalizedBlock(newBlock *types.Block) (bool, error) {
	if bc.casperGen == nil {
		log.Warn("Casper generator is not initialized")
		return false, ErrNilCasperGen
	}
	currentState, err := bc.State()
	if err != nil {
		return false, err
	}
	stateBackend := NewStateBackend(bc.CurrentBlock(), currentState, bc)
	contract, err := bc.casperGen(stateBackend)
	if err != nil {
		log.Warn("Failed to get Casper contract", "err", err)
		return false, err
	}
	blockNumber, err := contract.GetLastFinalizedEpoch()
	if err != nil {
		log.Warn("Failed to get current chain status from Casper", "err", err)
		return false, err
	}
	hashBytes, err := contract.GetCheckpointHashes(blockNumber)
	if err != nil {
		log.Warn("Failed to get current chain status from Casper", "err", err)
		return false, err
	}
	blockHash := common.BytesToHash(hashBytes[:])
	parentBlock := bc.GetBlock(newBlock.ParentHash(), newBlock.NumberU64()-1)
	// Find the correct block
	for ; parentBlock != nil && parentBlock.Number().Cmp(blockNumber) > 0; parentBlock = bc.GetBlock(parentBlock.ParentHash(), parentBlock.NumberU64()-1) {
	}
	// Compare its hash
	if parentBlock == nil {
		return false, nil
	}
	return parentBlock.Hash() == blockHash, nil
}
