// Copyright 2016 The go-ethereum Authors
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

package casper

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestDecimalType(t *testing.T) {
	// Pre-defined account
	key, _ := crypto.GenerateKey()
	auth := bind.NewKeyedTransactor(key)

	alloc := make(core.GenesisAlloc)
	alloc[auth.From] = core.GenesisAccount{Balance: big.NewInt(1000000000000000000)}
	sim := backends.NewSimulatedBackend(alloc)

	// Deploy casper contract
	epochLength := big.NewInt(50)
	withdrawalDelay := big.NewInt(5)
	owner := common.HexToAddress("0x9482a978b3f5962a5b0957d9ee9eef472ee55b42f1")
	sighasher := common.HexToAddress("0x9482a978b3f5962a5b0957d9ee9eef472ee55b42f2")
	purityChecker := common.HexToAddress("0x9482a978b3f5962a5b0957d9ee9eef472ee55b42f3")
	baseInterestFactor := float64(0.1)
	basePenaltyFactor := float64(0.0001)
	minDepositSize := big.NewInt(1500)

	_, _, contract, err := DeployCasper(auth, sim, epochLength, withdrawalDelay, owner, sighasher, purityChecker, baseInterestFactor, basePenaltyFactor, minDepositSize)
	sim.Commit()
	if err != nil {
		t.Errorf("Failed to deploy casper contract, err: %v", err)
	}

	// Ensure all contract parameters are set correctly
	f, err := contract.GetBasePenaltyFactor(&bind.CallOpts{})
	if err != nil {
		t.Errorf("Failed to get base penalty factor, err: %v", err)
	}
	if f != basePenaltyFactor {
		t.Errorf("Mismatch base penalty factor, got: %v, want: %v", f, basePenaltyFactor)
	}

	f, err = contract.GetBaseInterestFactor(&bind.CallOpts{})
	if err != nil {
		t.Errorf("Failed to get base interest factor, err: %v", err)
	}
	if f != baseInterestFactor {
		t.Errorf("Mismatch base interest factor, got: %v, want: %v", f, baseInterestFactor)
	}

	e, err := contract.GetEpochLength(&bind.CallOpts{})
	if err != nil {
		t.Errorf("Failed to get epoch length, err: %v", err)
	}
	if e.Cmp(epochLength) != 0 {
		t.Errorf("Mismatch epoch length, got: %v, want: %v", e, epochLength)
	}

	e, err = contract.GetWithdrawalDelay(&bind.CallOpts{})
	if err != nil {
		t.Errorf("Failed to get withdrawal delay, err: %v", err)
	}
	if e.Cmp(withdrawalDelay) != 0 {
		t.Errorf("Mismatch withdrawal delay, got: %v, want: %v", e, withdrawalDelay)
	}
}
