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
	"testing"

	"github.com/ethereum/go-ethereum/consensus/pbft"
	"github.com/ethereum/go-ethereum/crypto"
)

// view number is not checked for now
func TestVerifyPrepare(t *testing.T) {
	// for log purpose
	privateKey, _ := crypto.GenerateKey()
	peer := pbft.NewValidator(uint64(0), getPublicKeyAddress(privateKey))

	sys := newTestSystemWithBackend(uint64(1))

	testCases := []struct {
		expected error

		prepare *pbft.Subject
		self    *pbft.Subject
	}{
		{
			// normal case
			expected: nil,
			prepare: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(0), Sequence: big.NewInt(0)},
				Digest: []byte{1},
			},
			self: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(0), Sequence: big.NewInt(0)},
				Digest: []byte{1},
			},
		},
		{
			// old message
			expected: pbft.ErrOldMessage,
			prepare: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(0), Sequence: big.NewInt(0)},
				Digest: []byte{1},
			},
			self: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(1), Sequence: big.NewInt(1)},
				Digest: []byte{1},
			},
		},
		{
			// malicious package(lack of sequence)
			expected: pbft.ErrSubjectNotMatched,
			prepare: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(0), Sequence: nil},
				Digest: []byte{1},
			},
			self: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(1), Sequence: big.NewInt(1)},
				Digest: []byte{1},
			},
		},
		{
			// wrong prepare message with same sequence but different view number
			expected: pbft.ErrSubjectNotMatched,
			prepare: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(1), Sequence: big.NewInt(0)},
				Digest: []byte{1},
			},
			self: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(0), Sequence: big.NewInt(0)},
				Digest: []byte{1},
			},
		},
		{
			// wrong prepare message with same view number but different sequence
			expected: pbft.ErrSubjectNotMatched,
			prepare: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(0), Sequence: big.NewInt(1)},
				Digest: []byte{1},
			},
			self: &pbft.Subject{
				View:   &pbft.View{ViewNumber: big.NewInt(0), Sequence: big.NewInt(0)},
				Digest: []byte{1},
			},
		},
	}
	for i, test := range testCases {
		c := sys.backends[0].engine.(*core)
		c.subject = test.self

		if err := c.verifyPrepare(test.prepare, peer); err != nil {
			if err != test.expected {
				t.Errorf("expected result is not the same (%d), err:%v", i, err)
			}
		}
	}
}
