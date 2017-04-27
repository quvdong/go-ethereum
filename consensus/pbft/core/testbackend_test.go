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
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/pbft"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	elog "github.com/ethereum/go-ethereum/log"
)

var testLogger = elog.New()

type testSystemBackend struct {
	id  uint64
	sys *testSystem

	engine Engine
	peers  *pbft.ValidatorSet
	events *event.TypeMux

	commitMsgs []*pbft.Proposal

	address common.Address
}

// ==============================================
//
// define the functions that needs to be provided for PBFT.

func (self *testSystemBackend) Address() common.Address {
	return self.address
}

// Peers returns all connected peers
func (self *testSystemBackend) Validators() *pbft.ValidatorSet {
	return self.peers
}

func (self *testSystemBackend) EventMux() *event.TypeMux {
	return self.events
}

func (self *testSystemBackend) Send(message []byte) error {
	testLogger.Info("enqueuing a message...", "address", self.Address())
	self.sys.queuedMessage <- pbft.MessageEvent{
		Address: self.Address(),
		Payload: message,
	}
	return nil
}

func (self *testSystemBackend) UpdateState(state *pbft.State) error {
	testLogger.Warn("nothing to happen")
	return nil
}

func (self *testSystemBackend) ViewChanged(needNewProposal bool) error {
	testLogger.Warn("nothing to happen")
	return nil
}

func (self *testSystemBackend) Commit(proposal *pbft.Proposal) error {
	testLogger.Info("commit message", "address", self.Address())
	self.commitMsgs = append(self.commitMsgs, proposal)
	return nil
}

func (self *testSystemBackend) Verify(proposal *pbft.Proposal) (bool, error) {
	return true, nil
}

func (self *testSystemBackend) Sign(data []byte) ([]byte, error) {
	testLogger.Warn("not sign any data")
	return data, nil
}

func (self *testSystemBackend) CheckSignature([]byte, common.Address, []byte) error {
	return nil
}

func (self *testSystemBackend) IsProposer() bool {
	testLogger.Info("use replica 0 as proposer")
	if len(self.sys.backends) == 0 {
		return false
	}
	return self.Address() == self.sys.backends[0].Address()
}

func (self *testSystemBackend) Hash(b interface{}) common.Hash {
	return common.StringToHash("Test")
}
func (self *testSystemBackend) Encode(b interface{}) ([]byte, error) {
	return []byte(""), nil

}
func (self *testSystemBackend) Decode([]byte, interface{}) error {
	return nil
}

func (self *testSystemBackend) NewRequest(request []byte) {
	go self.events.Post(pbft.RequestEvent{
		Payload: request,
	})
}

// ==============================================
//
// define the functions that need to be provided for PBFT protocol manager.

func (self *testSystemBackend) AddPeer(peerID string, publicKey *ecdsa.PublicKey) error {
	testLogger.Warn("nothing to happen")
	return nil
}

// Remove a peer
func (self *testSystemBackend) RemovePeer(peerPublicKey string) error {
	testLogger.Warn("nothing to happen")
	return nil
}

// Handle a message from peer
func (self *testSystemBackend) HandleMsg(peerPublicKey string, data []byte) error {
	testLogger.Warn("nothing to happen")
	return nil
}

// Start is initialized peers
func (self *testSystemBackend) Start(chain consensus.ChainReader) error {
	return nil
}

// Stop the engine
func (self *testSystemBackend) Stop() error {
	testLogger.Warn("nothing to happen")
	return nil
}

// ==============================================
//
// define the struct that need to be provided for DB manager.

// Save an object into db
func (self *testSystemBackend) Save(key string, val interface{}) error {
	testLogger.Warn("nothing to happen")
	return nil
}

// Restore an object to val from db
func (self *testSystemBackend) Restore(key string, val interface{}) error {
	testLogger.Warn("nothing to happen")
	return nil
}

// ==============================================
//
// define the struct that need to be provided for integration tests.

type testSystem struct {
	backends []*testSystemBackend

	queuedMessage chan pbft.MessageEvent
	quit          chan struct{}
}

func newTestSystem(n uint64) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)
	return &testSystem{
		backends: make([]*testSystemBackend, n),

		queuedMessage: make(chan pbft.MessageEvent),
		quit:          make(chan struct{}),
	}
}

func NewTestSystemWithBackend(n uint64) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)

	// generate validators
	peers := make([]*pbft.Validator, int(n))
	for i := uint64(0); i < n; i++ {
		// TODO: the private key should be stored if we want to add new feature for sign data
		privateKey, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}

		peers[i] = pbft.NewValidator(getPublicKeyAddress(privateKey))
	}
	vset := pbft.NewValidatorSet(peers)

	sys := newTestSystem(n)

	for i := uint64(0); i < n; i++ {
		backend := sys.NewBackend(i)
		backend.peers = vset
		backend.address = vset.GetByIndex(i).Address()

		core := New(backend).(*core)
		core.current = pbft.NewLog(&pbft.Preprepare{
			View:     &pbft.View{},
			Proposal: &pbft.Proposal{},
		})
		core.logger = testLogger

		backend.engine = core
	}

	return sys
}

// run is triggered backend, core, and queue system.
func (t *testSystem) run() {
	// start a queue system
	go func() {
		for {
			select {
			case <-t.quit:
				return
			case queuedMessage := <-t.queuedMessage:
				testLogger.Info("consuming a queue message...", "msg from", queuedMessage.Address)
				for _, backend := range t.backends {
					go backend.EventMux().Post(queuedMessage)
				}
			}
		}
	}()
}

func (t *testSystem) stop() {
	close(t.quit)

	for _, backend := range t.backends {
		backend.engine.Stop()
		backend.Stop()
	}
}

func (t *testSystem) NewBackend(id uint64) *testSystemBackend {
	backend := &testSystemBackend{
		id:     id,
		sys:    t,
		events: new(event.TypeMux),
	}

	t.backends[id] = backend
	return backend
}

// ==============================================
//
// helper functions.

func getPublicKeyAddress(privateKey *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(privateKey.PublicKey)
}
