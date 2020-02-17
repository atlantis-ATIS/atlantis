// Copyright 2018 The go-athereum Authors
// This file is part of the go-athereum library.
//
// The go-athereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-athereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-athereum library. If not, see <http://www.gnu.org/licenses/>.
package storage

import (
	"sync"

	"github.com/athereum/go-athereum/swarm/log"
)

// PutChunks adds chunks  to localstore
// It waits for receive on the stored channel
// It logs but does not fail on delivery error
func PutChunks(store *LocalStore, chunks ...*Chunk) {
	wg := sync.WaitGroup{}
	wg.Add(len(chunks))
	go func() {
		for _, c := range chunks {
			<-c.dbStoredC
			if err := c.GetErrored(); err != nil {
				log.Error("chunk store fail", "err", err, "key", c.Addr)
			}
			wg.Done()
		}
	}()
	for _, c := range chunks {
		go store.Put(c)
	}
	wg.Wait()
}
