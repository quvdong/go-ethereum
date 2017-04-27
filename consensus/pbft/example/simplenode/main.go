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

package main

import (
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/pbft/backends/simulation"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

const (
	F = 1
	N = 3*F + 1
)

var (
	genesisBlock *types.Block
	blocks       map[common.Hash]*types.Block
)

func main() {
	// 0. Setup Logger
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	glogger.Verbosity(log.LvlDebug)
	log.Root().SetHandler(glogger)

	var nodeKeys = make([]*simulation.NodeKey, N)
	var addrs = make([]common.Address, N)
	for i := 0; i < N; i++ {
		nodeKeys[i] = simulation.NewNodeKey()
		addrs[i] = nodeKeys[i].Address()
	}

	// generate genesis block
	defaultGenesis, _ := core.DefaultGenesisBlock().ToBlock()
	genesisBlock := simulation.AppendValidators(defaultGenesis.Header(), addrs)

	var backends = make([]*simulation.ProtocolManager, N)
	for i := 0; i < N; i++ {
		backends[i] = simulation.New(nodeKeys[i], genesisBlock)
		backends[i].Start()
		defer backends[i].Stop()
	}

	// 2. Add peers for each backend
	for i := 0; i < N; i++ {
		for j := 0; j < N; j++ {
			if i != j {
				backends[i].AddPeer(backends[j].SelfPeer())
			}
		}
	}

	log.Info("Start")

	time.Sleep(3 * time.Second)
	for _, backend := range backends {
		backend.TryNewRequest()
	}
}
