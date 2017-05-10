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

package core

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/pbft"
)

// FIXME: only support fixed validator set
func newSyncState(valSet pbft.ValidatorSet) *syncState {
	return &syncState{
		valSet:      valSet,
		bitmap:      make(map[common.Address]bool),
		checkpoints: make(map[string][]*pbft.Preprepare),
	}
}

type syncState struct {
	mutex *sync.RWMutex

	valSet      pbft.ValidatorSet
	bitmap      map[common.Address]bool
	checkpoints map[string][]*pbft.Preprepare // proposal's hash -> votes
}

func (s *syncState) VerifyCheckpoint(preprepare *pbft.Preprepare) error {
	return nil
}

func (s *syncState) AddCheckpoint(preprepare *pbft.Preprepare, src common.Address) error {
	if err := s.VerifyCheckpoint(preprepare); err != nil {
		return err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// duplicate checkpoint
	if has := s.bitmap[src]; has {
		return nil
	}
	phash := preprepare.Proposal.Header.DataHash.Hex()
	votes := s.checkpoints[phash]
	if votes == nil {
		votes = make([]*pbft.Preprepare, 0)
		s.checkpoints[phash] = votes
	}
	votes = append(votes, preprepare)
	return nil
}

func (s *syncState) OneThirdCheckPoints() *pbft.Preprepare {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, cps := range s.checkpoints {
		if len(cps) > (1 / 3 * s.valSet.Size()) {
			return cps[0]
		}
	}
	return nil
}
