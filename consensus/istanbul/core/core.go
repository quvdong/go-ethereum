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
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
)

const (
	keyStableCheckpoint = "StableCheckpoint"
)

func New(backend istanbul.Backend, config *istanbul.Config) Engine {
	return &core{
		config:            config,
		address:           backend.Address(),
		state:             StateAcceptRequest,
		logger:            log.New("address", backend.Address()),
		backend:           backend,
		backlogs:          make(map[istanbul.Validator]*prque.Prque),
		backlogsMu:        new(sync.Mutex),
		pendingRequests:   prque.New(),
		pendingRequestsMu: new(sync.Mutex),
	}
}

// ----------------------------------------------------------------------------

type core struct {
	config  *istanbul.Config
	address common.Address
	N       int64
	F       int64
	state   State
	logger  log.Logger

	backend istanbul.Backend
	events  *event.TypeMuxSubscription

	lastProposer          common.Address
	lastProposal          istanbul.Proposal
	valSet                istanbul.ValidatorSet
	waitingForRoundChange bool

	backlogs   map[istanbul.Validator]*prque.Prque
	backlogsMu *sync.Mutex

	current *roundState

	roundChangeSet   *roundChangeSet
	roundChangeTimer *time.Timer

	pendingRequests   *prque.Prque
	pendingRequestsMu *sync.Mutex
}

func (c *core) finalizeMessage(msg *message) ([]byte, error) {
	// Add sender address
	msg.Address = c.Address()

	// Sign message
	data, err := msg.PayloadNoSig()
	if err != nil {
		return nil, err
	}
	msg.Signature, err = c.backend.Sign(data)
	if err != nil {
		return nil, err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func (c *core) send(msg *message, target common.Address) {
	logger := c.logger.New("state", c.state)

	payload, err := c.finalizeMessage(msg)
	if err != nil {
		logger.Error("Failed to finalize message", "msg", msg, "err", err)
		return
	}

	// send payload
	if err = c.backend.Send(payload, target); err != nil {
		logger.Error("Failed to send message", "msg", msg, "err", err)
		return
	}
}

func (c *core) broadcast(msg *message) {
	logger := c.logger.New("state", c.state)

	payload, err := c.finalizeMessage(msg)
	if err != nil {
		logger.Error("Failed to finalize message", "msg", msg, "err", err)
		return
	}

	// Broadcast payload
	if err = c.backend.Broadcast(c.valSet, payload); err != nil {
		logger.Error("Failed to broadcast message", "msg", msg, "err", err)
		return
	}
}

func (c *core) currentView() *istanbul.View {
	return &istanbul.View{
		Sequence: new(big.Int).Set(c.current.Sequence()),
		Round:    new(big.Int).Set(c.current.Round()),
	}
}

func (c *core) nextRound() *istanbul.View {
	return &istanbul.View{
		Sequence: new(big.Int).Set(c.current.Sequence()),
		Round:    new(big.Int).Add(c.current.Round(), common.Big1),
	}
}

func (c *core) isPrimary() bool {
	v := c.valSet
	if v == nil {
		return false
	}
	return v.IsProposer(c.backend.Address())
}

func (c *core) commit() {
	c.setState(StateCommitted)

	proposal := c.current.Proposal()
	if proposal != nil {
		if err := c.backend.Commit(proposal); err != nil {
			c.sendRoundChange()
			return
		}
	}
}

func (c *core) startNewRound(newView *istanbul.View, roundChange bool) {
	var logger log.Logger
	if c.current == nil {
		logger = c.logger.New("old_round", -1, "old_seq", 0, "old_proposer", c.valSet.GetProposer())
	} else {
		logger = c.logger.New("old_round", c.current.Round(), "old_seq", c.current.Sequence(), "old_proposer", c.valSet.GetProposer())
	}

	c.valSet = c.backend.Validators(c.lastProposal)
	c.N = int64(c.valSet.Size())
	c.F = int64(math.Ceil(float64(c.N)/3) - 1)
	// Clear invalid RoundChange messages
	c.roundChangeSet = newRoundChangeSet(c.valSet)
	// New snapshot for new round
	c.current = newRoundState(newView, c.valSet)
	// Calculate new proposer
	c.valSet.CalcProposer(c.lastProposer, newView.Round.Uint64())
	c.waitingForRoundChange = false
	c.setState(StateAcceptRequest)
	if roundChange && c.isPrimary() {
		c.backend.NextRound()
	}
	c.newRoundChangeTimer()

	logger.Debug("New round", "new_round", newView.Round, "new_seq", newView.Sequence, "new_proposer", c.valSet.GetProposer(), "valSet", c.valSet.List(), "size", c.valSet.Size())
}

func (c *core) catchUpRound(view *istanbul.View) {
	logger := c.logger.New("old_round", c.current.Round(), "old_seq", c.current.Sequence(), "old_proposer", c.valSet.GetProposer())
	c.waitingForRoundChange = true
	c.current = newRoundState(view, c.valSet)
	c.newRoundChangeTimer()

	logger.Trace("Catch up round", "new_round", view.Round, "new_seq", view.Sequence, "new_proposer", c.valSet)
}

func (c *core) setState(state State) {
	if c.state != state {
		c.state = state
	}
	if state == StateAcceptRequest {
		c.processPendingRequests()
	}
	c.processBacklog()
}

func (c *core) Address() common.Address {
	return c.address
}

func (c *core) RoundState() (State, *roundState) {
	return c.state, c.current
}

func (c *core) Backlog() map[istanbul.Validator]*prque.Prque {
	return c.backlogs
}

func (c *core) newRoundChangeTimer() {
	if c.roundChangeTimer != nil {
		c.roundChangeTimer.Stop()
	}

	timeout := time.Duration(c.config.RequestTimeout) * time.Millisecond
	c.roundChangeTimer = time.AfterFunc(timeout, func() {
		c.sendRoundChange()
	})
}

func (c *core) checkValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	// 1. Get signature address
	signer, err := istanbul.GetSignatureAddress(data, sig)
	if err != nil {
		log.Error("Failed to get signer address", "err", err)
		return common.Address{}, err
	}

	// 2. Check validator
	if _, val := c.valSet.GetByAddress(signer); val != nil {
		return val.Address(), nil
	}

	return common.Address{}, istanbul.ErrUnauthorizedAddress
}
