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

package types

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestHeaderHash(t *testing.T) {
	// 0xac1777d3630c38a34f60ab6bd4ff14160ab3781afa3a0bf565619b036265a02e
	expectedExtra := common.FromHex("0x00000000000000000000000000000000000000000000000000000000000000000444add0ec310f115a0e603b2d7db9f067778eaf8a294fc7e8f22b3bcdcf955dd7ff3ba2ed833f82126beaaed781d2d2ab6350f5c4566a2c6eaac407a68be76812f765c24641ec63dc2852b378aba2b4400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
	expectedHash := common.HexToHash("0xac1777d3630c38a34f60ab6bd4ff14160ab3781afa3a0bf565619b036265a02e")

	// for istanbul consensus
	header := &Header{MixDigest: IstanbulDigest, Extra: expectedExtra}
	if !reflect.DeepEqual(header.Hash(), expectedHash) {
		t.Errorf("expected: %v, but got: %v", expectedHash, header.Hash())
	}

	// useless information
	unexpectedExtra := append(expectedExtra, []byte{1, 2, 3}...)
	header.Extra = unexpectedExtra
	if !reflect.DeepEqual(header.Hash(), expectedHash) {
		t.Errorf("expected: %v, but got: %v", expectedHash, header.Hash())
	}
}

func TestIstanbulExtraFilter(t *testing.T) {
	testCases := []struct {
		testExtra []byte

		expectedExtra []byte
	}{
		// valid extra
		{hexutil.MustDecode("0x00000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"), hexutil.MustDecode("0x00000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")},

		// extra is nil
		{[]byte{}, bytes.Repeat([]byte{0x00}, IstanbulExtraVanity + IstanbulExtraValidatorSize + IstanbulExtraSeal)},

		// information is not enough
		{[]byte{1, 2, 3}, append([]byte{1, 2, 3}, bytes.Repeat([]byte{0x00}, IstanbulExtraVanity + IstanbulExtraValidatorSize + IstanbulExtraSeal -3 /*testExtra size is 3*/)...)},

		// validator size not mapping to validator length
		// validator size is 1, but validator length is 0
		{hexutil.MustDecode("0x0000000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"), hexutil.MustDecode("0x00000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")},

		// wrong signature length, 60 bytes
		{hexutil.MustDecode("0x000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"), hexutil.MustDecode("0x00000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")},
	}

	for _, test := range testCases {
		newHeader := IstanbulExtraFilter(&Header{Extra: test.testExtra})

		if !reflect.DeepEqual(newHeader.Extra, test.expectedExtra) {
			t.Errorf("expected: %v, but got: %v", test.expectedExtra, newHeader.Extra)
		}
	}
}
