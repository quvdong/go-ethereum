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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/pbft"
	pbftCore "github.com/ethereum/go-ethereum/consensus/pbft/core"
	"github.com/ethereum/go-ethereum/consensus/pbft/validator"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	errNotProposer         = errors.New("not a proposer")
	errViewChanged         = errors.New("view changed")
	errOtherBlockCommitted = errors.New("other block is committed")

	errInvalidExtraDataFormat = errors.New("invalid extra data format")

	defaultDifficulty = big.NewInt(1)
)

// Author retrieves the Ethereum address of the account that minted the given
// block, which may be different from the header's coinbase if a consensus
// engine is based on signatures.
func (sb *simpleBackend) Author(header *types.Header) (common.Address, error) {
	return ecrecover(header)
}

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
	if !sb.IsProposer() {
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

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
//
// Note, the block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *simpleBackend) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	if !sb.IsProposer() {
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
	if !sb.IsProposer() {
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

// AddPeer implements consensus.PBFT.AddPeer
func (sb *simpleBackend) AddPeer(peerID string, publicKey *ecdsa.PublicKey) error {
	peer := newPeer(peerID, publicKey)
	// check is validator
	if val := sb.valSet.GetByAddress(peer.Address()); val != nil {
		// add to peer set
		sb.peerSet.Add(peer)
		// post connection event to pbft core
		go sb.pbftEventMux.Post(pbft.ConnectionEvent{
			Address: val.Address(),
		})
	}
	return nil
}

// RemovePeer implements consensus.PBFT.RemovePeer
func (sb *simpleBackend) RemovePeer(peerID string) error {
	sb.peerSet.Remove(peerID)
	return nil
}

// HandleMsg implements consensus.PBFT.HandleMsg
func (sb *simpleBackend) HandleMsg(peerID string, data []byte) error {
	peer := sb.peerSet.Get(peerID)
	if peer == nil {
		return nil
	}

	msgEvent, err := Decode(data)
	if err != nil {
		return err
	}

	if val := sb.valSet.GetByAddress(peer.Address()); val != nil {
		go sb.pbftEventMux.Post(*msgEvent)
	}

	return nil
}

// Start implements consensus.PBFT.Start
func (sb *simpleBackend) Start(chain consensus.ChainReader) error {
	if !sb.initValidatorSet(chain) {
		return errInvalidExtraDataFormat
	}
	sb.core = pbftCore.New(sb)
	return sb.core.Start()
}

// Stop implements consensus.PBFT.Stop
func (sb *simpleBackend) Stop() error {
	return sb.core.Stop()
}

func (sb *simpleBackend) initValidatorSet(chain consensus.ChainReader) bool {
	currentHeader := chain.CurrentHeader()
	// get the validator byte array and feed into validator set
	length := len(currentHeader.Extra)
	valSet, r := validator.NewSet(currentHeader.Extra[extraVanity : length-extraSeal])
	if !r || valSet == nil {
		return false
	}
	sb.valSet = valSet
	return true
}

// FIXME: Need to update this for PBFT
// sigHash returns the hash which is used as input for the PBFT
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func sigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewKeccak256()

	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.UncleHash,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra[:len(header.Extra)-65], // Yes, this will panic if extra is too short
		header.MixDigest,
		header.Nonce,
	})
	hasher.Sum(hash[:0])
	return hash
}

// FIXME: Need to update this for PBFT
// ecrecover extracts the Ethereum account address from a signed header.
func ecrecover(header *types.Header) (common.Address, error) {
	// Retrieve the signature from the header extra-data
	if len(header.Extra) < extraSeal {
		return common.Address{}, consensus.ErrMissingSignature
	}
	signature := header.Extra[len(header.Extra)-extraSeal:]

	// Recover the public key and the Ethereum address
	pubkey, err := crypto.Ecrecover(sigHash(header).Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])

	return signer, nil
}
