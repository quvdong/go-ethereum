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
	"math/big"

	"github.com/ethereum/go-ethereum/consensus/pbft"
)

func (c *core) sendCheckpoint(cp *pbft.Checkpoint) {
	logger := c.logger.New("state", c.state)
	logger.Debug("sendCheckpoint")
	c.broadcast(pbft.MsgCheckpoint, cp)
}

func (c *core) handleCheckpoint(cp *pbft.Checkpoint, src pbft.Validator) error {
	if cp == nil {
		return pbft.ErrInvalidMessage
	}

	logger := c.logger.New("from", src.Address().Hex(), "state", c.state)
	var log *pbft.Log

	logger.Debug("handleCheckpoint")

	c.consensusLogsMu.Lock()
	defer c.consensusLogsMu.Unlock()

	if cp.Sequence.Cmp(c.current.Sequence) == 0 { // current
		log = c.current
	} else if cp.Sequence.Cmp(c.current.Sequence) < 0 { // old checkpoint
		logIndex := c.searchLog(cp.Sequence, 0, len(c.consensusLogs)-1)
		if logIndex >= 0 {
			log = c.consensusLogs[logIndex]
		} else {
			logger.Error("Failed to find log entry", "seq", cp.Sequence, "current", c.current.Sequence)
			return pbft.ErrInvalidMessage
		}
	} else { // future checkpoint
		return pbft.ErrInvalidMessage
	}

	if _, err := log.Checkpoints.Add(cp, src); err != nil {
		logger.Error("Failed to add checkpoint", "error", err)
		return err
	}

	return nil
}

func (c *core) searchLog(seq *big.Int, low, high int) (mid int) {
	if low < 0 || high >= len(c.consensusLogs) {
		return -1
	}

	for low <= high {
		mid = (low + high) / 2
		if c.consensusLogs[mid].Sequence.Cmp(seq) > 0 {
			high = mid - 1
		} else if c.consensusLogs[mid].Sequence.Cmp(seq) < 0 {
			low = mid + 1
		} else {
			return mid
		}
	}

	return -1
}

func (c *core) buildStableCheckpoint() {
	var stableCheckpoint *pbft.Log
	stableCheckpointIndex := -1
	logger := c.logger.New("seq", c.sequence)

	c.consensusLogsMu.Lock()
	for i := len(c.consensusLogs) - 1; i >= 0; i-- {
		log := c.consensusLogs[i]
		if log.Checkpoints.Size() > int(c.F*2) {
			stableCheckpoint = log
			stableCheckpointIndex = i
			break
		}
	}

	// We found a stable checkpoint
	if stableCheckpointIndex != -1 {
		// Remove old logs
		c.consensusLogs = c.consensusLogs[stableCheckpointIndex+1:]
	}

	// Release the lock as soon as possible
	c.consensusLogsMu.Unlock()

	// TODO: store stable checkpoint to disk
	logger.Debug("Stable checkpoint", "checkpoint", stableCheckpoint)
}
