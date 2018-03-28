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
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts/casper"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

// getScore returns the score of a block
func (bc *BlockChain) getScore(block *types.Block) *big.Int {
	score := bc.GetTd(block.Hash(), block.NumberU64())

	if bc.chainConfig.IsCasper(block.Number()) {
		if scored, casperScore := bc.getCasperScore(block); scored {
			casperScale := new(big.Int).Exp(big.NewInt(int64(10)), big.NewInt(int64(40)), nil)
			casperScore.Mul(casperScore, casperScale)
			score.Add(score, casperScore)
		}
	}
	return score
}

// getCasperScore returns the score of a block from Casper's perspective
func (bc *BlockChain) getCasperScore(block *types.Block) (bool, *big.Int) {
	state, err := bc.StateAt(block.Root())
	if err != nil {
		return false, nil
	}
	stateBackend := NewStateBackend(block, state, bc)
	contract, err := casper.New(stateBackend)
	if err != nil {
		log.Warn("Failed to get Casper contract", "err", err)
		return false, nil
	}
	justified, err := contract.GetLastJustifiedEpoch(&bind.CallOpts{})
	if err != nil {
		log.Warn("Failed to get current chain status from Casper", "err", err)
		return false, nil
	}
	return true, justified
}

// safeForLastFinalizedBlock returns true if the new head will NOT revert the last finalized block
func (bc *BlockChain) safeForLastFinalizedBlock(newBlock *types.Block, newState *state.StateDB) bool {
	stateBackend := NewStateBackend(newBlock, newState, bc)
	contract, err := casper.New(stateBackend)
	if err != nil {
		log.Warn("Failed to get Casper contract", "err", err)
		return false
	}
	blockNumber, err := contract.GetLastFinalizedEpoch(&bind.CallOpts{})
	if err != nil {
		log.Warn("Failed to get current chain status from Casper", "err", err)
		return false
	}
	hashBytes, err := contract.GetCheckpointHashes(&bind.CallOpts{}, blockNumber)
	if err != nil {
		log.Warn("Failed to get current chain status from Casper", "err", err)
		return false
	}
	blockHash := common.BytesToHash(hashBytes[:])
	parentBlock := bc.GetBlock(newBlock.ParentHash(), newBlock.NumberU64()-1)
	for {
		if parentBlock == nil || blockNumber.Cmp(parentBlock.Number()) > 0 {
			return false
		} else if parentBlock.Hash() == blockHash {
			// The last finalized block IS currentBlock's ancestor
			return true
		} else {
			parentBlock = bc.GetBlock(parentBlock.ParentHash(), parentBlock.NumberU64()-1)
		}
	}
	return false
}
