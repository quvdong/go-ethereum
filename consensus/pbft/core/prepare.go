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
	"reflect"

	"github.com/ethereum/go-ethereum/consensus/pbft"
)

func (c *core) sendPrepare() {
	logger := c.logger.New("state", c.state)
	logger.Trace("sendPrepare")

	subject, err := Encode(c.subject)
	if err != nil {
		logger.Error("Failed to encode", "subject", c.subject)
		return
	}
	c.broadcast(&message{
		Code: msgPrepare,
		Msg:  subject,
	})
}

func (c *core) handlePrepare(msg *message, src pbft.Validator) error {
	logger := c.logger.New("from", src.Address().Hex(), "state", c.state)
	logger.Trace("handlePrepare")

	if c.waitingForRoundChange {
		logger.Warn("Waiting for a RoundChange, ignore", "msg", msg)
		return pbft.ErrIgnored
	}

	// Decode prepare message
	var prepare *pbft.Subject
	err := msg.Decode(&prepare)
	if err != nil {
		return errFailedDecodePrepare
	}

	if err := c.checkMessage(msgPrepare, prepare.View); err != nil {
		return err
	}

	if err := c.verifyPrepare(prepare, src); err != nil {
		return err
	}

	c.acceptPrepare(msg, src)

	// Change to StatePrepared if we've received enough prepare messages
	// and we are in earlier state before StatePrepared
	if int64(c.current.Prepares.Size()) > 2*c.F && c.state.Cmp(StatePrepared) < 0 {
		c.setState(StatePrepared)
		c.sendCommit()
	}

	return nil
}

// verifyPrepare verifies if the received prepare message is equivalent to our subject
func (c *core) verifyPrepare(prepare *pbft.Subject, src pbft.Validator) error {
	logger := c.logger.New("from", src.Address().Hex(), "state", c.state)

	if !reflect.DeepEqual(prepare, c.subject) {
		logger.Warn("Inconsistent subjects between prepare and proposal", "expected", c.subject, "got", prepare)
		return pbft.ErrSubjectNotMatched
	}

	return nil
}

func (c *core) acceptPrepare(msg *message, src pbft.Validator) error {
	logger := c.logger.New("from", src.Address().Hex(), "state", c.state)

	// Add the prepare message to current snapshot
	if err := c.current.Prepares.Add(msg); err != nil {
		logger.Error("Failed to add prepare message to snapshot", "msg", msg, "error", err)
		return err
	}

	return nil
}
