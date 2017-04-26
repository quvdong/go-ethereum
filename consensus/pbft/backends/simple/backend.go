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

package simple

import (
	"bytes"
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/pbft"
	pbftCore "github.com/ethereum/go-ethereum/consensus/pbft/core"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	extraVanity = 32 // Fixed number of extra-data prefix bytes reserved for signer vanity
	extraSeal   = 65 // Fixed number of extra-data suffix bytes reserved for signer seal
)

func New(timeout int, eventMux *event.TypeMux, privateKey *ecdsa.PrivateKey, db ethdb.Database) consensus.PBFT {
	backend := &simpleBackend{
		peerSet:      newPeerSet(),
		eventMux:     eventMux,
		pbftEventMux: new(event.TypeMux),
		privateKey:   privateKey,
		logger:       log.New("backend", "simple"),
		db:           db,
		timeout:      uint64(timeout),
	}
	return backend
}

// ----------------------------------------------------------------------------
type simpleBackend struct {
	id             uint64
	peerSet        *peerSet
	valSet         *pbft.ValidatorSet
	eventMux       *event.TypeMux
	pbftEventMux   *event.TypeMux
	privateKey     *ecdsa.PrivateKey
	consensusState *pbft.State
	core           pbftCore.Engine
	logger         log.Logger
	quitSync       chan struct{}
	db             ethdb.Database
	timeout        uint64

	// the channels for pbft engine notifications
	viewChange chan bool
	commit     chan common.Hash
}

// ID implements pbft.Backend.ID
func (sb *simpleBackend) ID() uint64 {
	return sb.id
}

// Validators implements pbft.Backend.Validators
func (sb *simpleBackend) Validators() *pbft.ValidatorSet {
	return sb.valSet
}

// Send implements pbft.Backend.Send
func (sb *simpleBackend) Send(data []byte) error {
	pbftMsg := pbft.MessageEvent{
		ID:      sb.ID(),
		Payload: data,
	}
	pbftByte, err := Encode(&pbftMsg)
	if err != nil {
		return err
	}

	// send to self
	go sb.pbftEventMux.Post(pbftMsg)

	// send to other peers
	for _, peer := range sb.peerSet.List() {
		go sb.eventMux.Post(pbft.ConsensusDataEvent{
			PeerID: peer.ID(),
			Data:   pbftByte,
		})
	}
	return nil
}

// Commit implements pbft.Backend.Commit
func (sb *simpleBackend) Commit(proposal *pbft.Proposal) error {
	log.Info("Committed", "id", sb.ID(), "proposal", proposal)
	// step1: update validator set from extra data of block
	// step2: insert chain
	block := &types.Block{}
	err := rlp.DecodeBytes(proposal.Payload, block)
	if err != nil {
		log.Warn("decode block error", "err", err)
		return err
	}
	// it's a proposer
	if sb.commit != nil {
		go func() {
			sb.commit <- block.Hash()
		}()
	} else {
		go sb.eventMux.Post(pbft.ConsensusCommitBlockEvent{Block: block})
	}
	return nil
}

// ViewChanged implements pbft.Backend.ViewChanged
func (sb *simpleBackend) ViewChanged(needNewProposal bool) error {
	// step1: update proposer
	// step2: notify proposer and validator
	if sb.viewChange != nil {
		go func() {
			sb.viewChange <- needNewProposal
		}()
	}
	if sb.IsProposer() {
		go sb.eventMux.Post(core.ChainHeadEvent{})
	}
	return nil
}

// Hash implements pbft.Backend.Hash
func (sb *simpleBackend) Hash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

// Encode implements pbft.Backend.Encode
func (sb *simpleBackend) Encode(v interface{}) ([]byte, error) {
	return rlp.EncodeToBytes(v)
}

// Decode implements pbft.Backend.Decode
func (sb *simpleBackend) Decode(b []byte, v interface{}) error {
	return rlp.DecodeBytes(b, v)
}

// EventMux implements pbft.Backend.EventMux
func (sb *simpleBackend) EventMux() *event.TypeMux {
	// not implemented
	return sb.pbftEventMux
}

// Verify implements pbft.Backend.Verify
func (sb *simpleBackend) Verify(proposal *pbft.Proposal) (bool, error) {
	// not implemented
	return true, nil
}

// Sign implements pbft.Backend.Sign
func (sb *simpleBackend) Sign(data []byte) ([]byte, error) {
	hashData := crypto.Keccak256([]byte(data))
	return crypto.Sign(hashData, sb.privateKey)
}

// CheckSignature implements pbft.Backend.CheckSignature
func (sb *simpleBackend) CheckSignature(data []byte, address common.Address, sig []byte) error {
	//1. Keccak data
	hashData := crypto.Keccak256([]byte(data))
	//2. Recover public key
	pubkey, err := crypto.SigToPub(hashData, sig)
	if err != nil {
		log.Error("CheckSignature", "error", err)
		return err
	}
	//3. Compare derived addresses
	signer := crypto.PubkeyToAddress(*pubkey)
	if bytes.Compare(signer.Bytes(), address.Bytes()) != 0 {
		return pbft.ErrInvalidSignature
	}
	return nil
}

// UpdateState implements pbft.Backend.UpdateState
func (sb *simpleBackend) UpdateState(state *pbft.State) error {
	sb.consensusState = state
	return nil
}

func (sb *simpleBackend) IsProposer() bool {
	if sb.valSet == nil {
		return false
	}
	return sb.valSet.IsProposer(sb.id)
}
