// Copyright 2014 The go-ethereum Authors
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

// Package eth implements the Ethereum protocol.
package eth

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts"
	casperContract "github.com/ethereum/go-ethereum/contracts/casper"
)

type casper struct {
	contract *casperContract.Casper
}

// NewCasperGen returns a func which can generate capser instance at specific state and given address
func NewCasperGen(address common.Address) func(bind.ContractBackend) (contracts.Casper, error) {
	return func(backend bind.ContractBackend) (contracts.Casper, error) {
		return NewCasper(address, backend)
	}
}

// NewCasper returns casper instance at specific state and address
func NewCasper(address common.Address, backend bind.ContractBackend) (contracts.Casper, error) {
	contract, err := casperContract.NewCasper(address, backend)
	return &casper{
		contract: contract,
	}, err
}

// GetLastJustifiedEpoch returns the last justified epoch in specific backend
func (c *casper) GetLastJustifiedEpoch() (*big.Int, error) {
	return c.contract.GetLastJustifiedEpoch(&bind.CallOpts{})
}

// GetLastFinalizedEpoch returns the last finalized epoch in specific backend
func (c *casper) GetLastFinalizedEpoch() (*big.Int, error) {
	return c.contract.GetLastFinalizedEpoch(&bind.CallOpts{})
}

// GetCheckpointHashes returns the checkpoint hashes in specific backend
func (c *casper) GetCheckpointHashes(number *big.Int) ([32]byte, error) {
	return c.contract.GetCheckpointHashes(&bind.CallOpts{}, number)
}
