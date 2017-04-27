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
	"reflect"
	"testing"
	"time"
)

func TestNewRequest(t *testing.T) {
	N := uint64(4)

	sys := newTestSystem(N)

	for i := uint64(0); i < N; i++ {
		backend := sys.NewBackend(uint64(i))

		c := New(backend).(*core)
		c.N = int64(N)

		backend.engine = c
	}

	for _, backend := range sys.backends {
		backend.Start(nil)
		backend.engine.Start() // start PBFT core
	}

	sys.run()
	defer sys.stop()

	request1 := []byte("request 1")
	sys.backends[0].NewRequest(request1)

	select {
	case <-time.After(1 * time.Second):
	}

	request2 := []byte("request 2")
	sys.backends[0].NewRequest(request2)

	select {
	case <-time.After(1 * time.Second):
	}

	for _, backend := range sys.backends {
		if len(backend.commitMsgs) != 2 {
			t.Error("expected execution of requests should be 2, id:", backend.ID())
		}
		if !reflect.DeepEqual(request1, backend.commitMsgs[0].Payload) {
			t.Error("payload is not the same (1)")
		}
		if !reflect.DeepEqual(request2, backend.commitMsgs[1].Payload) {
			t.Error("payload is not the same (2)")
		}
	}
}
