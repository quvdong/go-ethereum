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
	"crypto/ecdsa"
	"errors"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/pbft"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	errNotProposer         = errors.New("not a proposer")
	errViewChanged         = errors.New("view changed")
	errOtherBlockCommitted = errors.New("other block is committed")

	defaultDifficulty = big.NewInt(1)
)

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
func (sb *simpleBackend) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return nil
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (sb *simpleBackend) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			err := sb.VerifyHeader(chain, header, seals[i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of a given engine.
func (sb *simpleBackend) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (sb *simpleBackend) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (sb *simpleBackend) Prepare(chain consensus.ChainReader, header *types.Header) error {
	if !sb.isProposer() {
		return errNotProposer
	}

	// copy the parent extra data as the header extra data
	height := uint64(header.Number.Int64())
	parent := chain.GetHeader(header.ParentHash, height-1)
	header.Extra = parent.Extra
	// use the same difficulty for all blocks
	header.Difficulty = defaultDifficulty
	return nil
}

func (sb *simpleBackend) isProposer() bool {
	if sb.valSet == nil {
		return false
	}
	return reflect.DeepEqual(sb.valSet.GetProposer(), sb.valSet.GetByIndex(sb.id))
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
//
// Note, the block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *simpleBackend) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	if !sb.isProposer() {
		return nil, errNotProposer
	}

	// No block rewards in PBFT, so the state remains as is and uncles are dropped
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = types.CalcUncleHash(nil)

	// Assemble and return the final block for sealing
	return types.NewBlock(header, txs, nil, receipts), nil
}

func (sb *simpleBackend) closeChannels() {
	if sb.viewChange != nil {
		close(sb.viewChange)
		sb.viewChange = nil
	}

	if sb.commit != nil {
		close(sb.commit)
		sb.commit = nil
	}
}

func (sb *simpleBackend) newChannels() {
	sb.viewChange = make(chan bool, 1)
	sb.commit = make(chan common.Hash, 1)
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (sb *simpleBackend) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	if !sb.isProposer() {
		return nil, errNotProposer
	}

	sb.newChannels()
	defer sb.closeChannels()

	// step 1. feed block into PBFT engine
	b, e := rlp.EncodeToBytes(block)
	if e != nil {
		return nil, e
	}
	go sb.EventMux().Post(pbft.RequestEvent{
		Payload: b,
	})

	for {
		select {
		case needNewProposal := <-sb.viewChange:
			if needNewProposal {
				return nil, errViewChanged
			}
			// if we don't need to change block, we keep waiting events.
		case hash := <-sb.commit:
			if block.Hash() == hash {
				return block, nil
			}
			return nil, errOtherBlockCommitted
		}
	}
}

// APIs returns the RPC APIs this consensus engine provides.
func (sb *simpleBackend) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "pbft",
		Version:   "1.0",
		Service:   &API{chain: chain, backend: sb},
		Public:    true,
	}}
}

func (sb *simpleBackend) AddPeer(peerID string, publicKey *ecdsa.PublicKey) {
	peer := newPeer(peerID, publicKey)
	// check is validator
	if val := sb.valSet.GetByAddress(peer.Address()); val != nil {
		// add to peer set
		sb.peerSet.Add(peer)
		// post connection event to pbft core
		go sb.pbftEventMux.Post(pbft.ConnectionEvent{
			ID: val.ID(),
		})
	}
}

func (sb *simpleBackend) RemovePeer(peerID string) {
	sb.peerSet.Remove(peerID)
}

func (sb *simpleBackend) HandleMsg(peerID string, data []byte) {
	peer := sb.peerSet.Get(peerID)
	if peer == nil {
		return
	}

	msgEvent, err := Decode(data)
	if err != nil {
		return
	}

	if val := sb.valSet.GetByAddress(peer.Address()); val != nil {
		go sb.pbftEventMux.Post(*msgEvent)
	}
}

func (sb *simpleBackend) Start(chain consensus.ChainReader) {
	sb.initValidatorSet(chain)
	sb.core.Start()
}

func (sb *simpleBackend) Stop() {
	sb.core.Stop()
}

func (sb *simpleBackend) initValidatorSet(chain consensus.ChainReader) {
	currentHeader := chain.CurrentHeader()
	addrs := getValidatorSet(currentHeader)
	vals := make([]*pbft.Validator, len(addrs))
	for i, addr := range addrs {
		vals[i] = pbft.NewValidator(uint64(i), addr)
	}
	sb.valSet = pbft.NewValidatorSet(vals)

	// FIXME: self should be the one of valifators
	// update self id
	// get peer by address
	addr := crypto.PubkeyToAddress(sb.privateKey.PublicKey)
	privVal := sb.valSet.GetByAddress(addr)
	// update validator id
	if privVal != nil {
		sb.id = privVal.ID()
	}
}

func getValidatorSet(block *types.Header) []common.Address {
	// get validator address from block header
	addrs := make([]common.Address, (len(block.Extra)-extraVanity-extraSeal)/common.AddressLength)
	for i := 0; i < len(addrs); i++ {
		copy(addrs[i][:], block.Extra[extraVanity+i*common.AddressLength:])
	}
	return addrs
}
