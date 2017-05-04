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

package pbft

import (
	"bytes"
	"encoding/gob"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

const (
	MsgPreprepare uint64 = iota
	MsgPrepare
	MsgCommit
	MsgCheckpoint
	MsgViewChange
	MsgNewView
	MsgInvalid
)

// TODO: under cooking
type State struct {
	View     *View
	Proposal *Proposal

	PrepareMsgs map[uint64]*Subject
	CommitMsgs  map[uint64]*Subject
}

type Message struct {
	Code      uint64
	Msg       interface{}
	Signature []byte
}

func (m *Message) ToPayload() ([]byte, error) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(m)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Decode(b []byte, validateFn func([]byte, []byte) (common.Address, error)) (*Message, error) {
	var msg Message
	// Decode message
	err := gob.NewDecoder(bytes.NewBuffer(b)).Decode(&msg)
	if err != nil {
		return nil, err
	}

	// Validate message (on a Message without Signature)
	if validateFn != nil {
		m := &Message{Code: msg.Code, Msg: msg.Msg, Signature: nil}
		var payload []byte
		payload, err = m.ToPayload()
		if err != nil {
			return nil, err
		}
		_, err = validateFn(payload, msg.Signature)
	}
	// Still return the message even the err is not nil
	return &msg, err
}

func Encode(code uint64, val interface{}, signFn func([]byte) ([]byte, error)) (*Message, error) {
	var sig []byte

	if signFn != nil {
		// Create message without signature (for data signing)
		m := &Message{Code: code, Msg: val, Signature: nil}

		// Sign message
		payload, err := m.ToPayload()
		if err != nil {
			return nil, err
		}

		sig, err = signFn(payload)
		if err != nil {
			return nil, err
		}
	}

	// Return Message with signature
	return &Message{
		Code:      code,
		Msg:       val,
		Signature: sig,
	}, nil
}

type Request struct {
	Payload []byte
}

type View struct {
	ViewNumber *big.Int
	Sequence   *big.Int
}

type ProposalHeader struct {
	Sequence   *big.Int
	ParentHash common.Hash
	DataHash   common.Hash
}

type Proposal struct {
	Header     *ProposalHeader
	Payload    []byte
	Signatures [][]byte
}

type Preprepare struct {
	View     *View
	Proposal *Proposal
}

type Subject struct {
	View   *View
	Digest []byte
}

type ViewChange struct {
	ViewNumber *big.Int
	PSet       []*Subject
	QSet       []*Subject
	Proposal   *Proposal
}

type SignedViewChange struct {
	Data      []byte
	Signature []byte
}

type NewView struct {
	ViewNumber *big.Int
	VSet       *SignedViewChange
	XSet       *Subject
	Proposal   *Proposal
}

type Checkpoint struct {
	Sequence  *big.Int
	Digest    []byte
	Signature []byte
}

func init() {
	gob.Register(&Preprepare{})
	gob.Register(&Subject{})
	gob.Register(&Checkpoint{})
}
