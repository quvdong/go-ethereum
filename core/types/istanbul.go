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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

var (
	// The IstanbulDigest represents a constant of "Istanbul practical byzantine fault tolerance".
	IstanbulDigest = common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	ExtraVanity        = int(params.MaximumExtraDataSize) // Fixed number of extra-data prefix bytes reserved for signer vanity
	ExtraValidatorSize = 1                                // Fixed number of extra-data infix bytes reserved for validator size
	ExtraSeal          = 65                               // Fixed number of extra-data suffix bytes reserved for signer seal
)

// IstanbulExtraFilter is used to filter out the useless information.
//
// We just keep the following data in header.Extra.
// ┌─────────────────┬──────────────────────┬────────────────────────────────┬───────────────┐
// │ vanity(32 bytes)│ validator#N (1 bytes)│ validator address(N * 20 bytes)│ seal(65 bytes)│
// └─────────────────┴──────────────────────┴────────────────────────────────┴───────────────┘
func IstanbulExtraFilter(h *Header) *Header {
	newHeader := CopyHeader(h)
	defaultLength := ExtraVanity + ExtraValidatorSize + ExtraSeal

	// The number of validator addresses is not known since the extra may be insufficient,
	// but we can checks whether extra is enough(vanity + validator#N + seal).
	// If it is not enough, compensate the lack of bytes to avoid panic in getting slice.
	if len(newHeader.Extra) < defaultLength {
		newHeader.Extra = append(newHeader.Extra, bytes.Repeat([]byte{0x00}, defaultLength-len(newHeader.Extra))...)
	}

	// Calculate the validator length.
	valLength := int(newHeader.Extra[ExtraVanity : ExtraVanity+ExtraValidatorSize][0]) * common.AddressLength

	// Same as before, but validator length is also considered.
	if len(newHeader.Extra) < defaultLength+valLength {
		newHeader.Extra = append(newHeader.Extra, bytes.Repeat([]byte{0x00}, defaultLength+valLength-len(newHeader.Extra))...)
	}

	// We just keep vanity, validator#N, validators, and seal in Extra.
	newHeader.Extra = newHeader.Extra[0 : ExtraVanity+ExtraValidatorSize+valLength+ExtraSeal]

	return newHeader
}
