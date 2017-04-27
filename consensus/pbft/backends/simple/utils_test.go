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
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/pbft"
)

func TestEventMessageEncodeAndDecode(t *testing.T) {
	pbftMsg := pbft.MessageEvent{
		Address: common.HexToAddress("0x70524d664ffe731100208a0154e556f9bb679ae6"),
		Payload: []byte("hello world"),
	}

	// test encode
	b, err := Encode(&pbftMsg)
	if err != nil {
		t.Errorf("shouldn't gor error, got:%v, expected: nil", err)
	}

	// test decode
	gotMsg, err := Decode(b)
	if err != nil {
		t.Errorf("shouldn't gor error, got:%v, expected: nil", err)
	}

	if gotMsg.Address != pbftMsg.Address {
		t.Errorf("got wrong address from message event, got:%v, expected:%v", gotMsg.Address.Hex(), pbftMsg.Address.Hex())
	}

	if string(gotMsg.Payload) != string(pbftMsg.Payload) {
		t.Errorf("got wrong payload from message event, got:%v, expected:%v", gotMsg.Payload, pbftMsg.Payload)
	}

}
