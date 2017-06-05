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
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/pbft"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	elog "github.com/ethereum/go-ethereum/log"
)

var testLogger = elog.New()

type testSystemBackend struct {
	id  uint64
	sys *testSystem

	engine Engine
	peers  *pbft.ValidatorSet
	events *event.TypeMux

	commitMsgs []*pbft.Proposal

	privateKey *ecdsa.PrivateKey
}

// ==============================================
//
// define the functions that needs to be provided for PBFT.

func (self *testSystemBackend) ID() uint64 {
	return self.id
}

// Peers returns all connected peers
func (self *testSystemBackend) Validators() *pbft.ValidatorSet {
	return self.peers
}

func (self *testSystemBackend) EventMux() *event.TypeMux {
	return self.events
}

func (self *testSystemBackend) Send(message []byte) {
	testLogger.Info("enqueuing a message...", "id", self.ID())
	self.sys.queuedMessage <- pbft.MessageEvent{
		ID:      self.ID(),
		Payload: message,
	}
}

func (self *testSystemBackend) UpdateState(state *pbft.State) {
	testLogger.Warn("nothing to happen")
}

func (self *testSystemBackend) Commit(proposal *pbft.Proposal) {
	testLogger.Info("commit message", "id", self.ID())
	self.commitMsgs = append(self.commitMsgs, proposal)
}

func (self *testSystemBackend) Verify(proposal *pbft.Proposal) (bool, error) {
	return true, nil
}

func (self *testSystemBackend) Sign(data []byte) ([]byte, error) {
	hashData := crypto.Keccak256([]byte(data))
	return crypto.Sign(hashData, self.privateKey)
}

func (self *testSystemBackend) CheckSignature([]byte, common.Address, []byte) error {
	return nil
}

func (self *testSystemBackend) Hash(b interface{}) common.Hash {
	return common.StringToHash("Test")
}
func (self *testSystemBackend) Encode(b interface{}) ([]byte, error) {
	return []byte(""), nil

}
func (self *testSystemBackend) Decode([]byte, interface{}) error {
	return nil
}

func (self *testSystemBackend) NewRequest(request []byte) {
	go self.events.Post(pbft.RequestEvent{
		Payload: request,
	})
}

// ==============================================
//
// define the functions that need to be provided for PBFT protocol manager.

func (self *testSystemBackend) AddPeer(peerID string, publicKey *ecdsa.PublicKey) {
	testLogger.Info(fmt.Sprintf("add peer: %s", peerID), "id", self.ID())
}

// Remove a peer
func (self *testSystemBackend) RemovePeer(peerPublicKey string) {
	testLogger.Warn("nothing to happen")
}

// Handle a message from peer
func (self *testSystemBackend) HandleMsg(peerPublicKey string, data []byte) {
	testLogger.Warn("nothing to happen")
}

// Start is initialized peers
func (self *testSystemBackend) Start(chain consensus.ChainReader) {
	peers := make([]*pbft.Validator, len(self.sys.backends))
	for i, backend := range self.sys.backends {
		peers[i] = pbft.NewValidator(
			uint64(i), // use the index as id
			getPublicKeyAddress(backend.privateKey),
		)
	}
	self.peers = pbft.NewValidatorSet(peers)
}

// Stop the engine
func (self *testSystemBackend) Stop() {
	testLogger.Warn("nothing to happen")
}

// ==============================================
//

type testSystem struct {
	backends map[uint64]*testSystemBackend

	queuedMessage chan pbft.MessageEvent
	quit          chan struct{}
}

func newTestSystem() *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)
	return &testSystem{
		backends: make(map[uint64]*testSystemBackend),

		queuedMessage: make(chan pbft.MessageEvent),
		quit:          make(chan struct{}),
	}
}

// run is triggered backend, core, and queue system.
func (t *testSystem) run() {
	// start a queue system
	go func() {
		for {
			select {
			case <-t.quit:
				return
			case queuedMessage := <-t.queuedMessage:
				testLogger.Info("consuming a queue message...", "msg from", queuedMessage.ID)
				for _, backend := range t.backends {
					go backend.EventMux().Post(queuedMessage)
				}
			}
		}
	}()
}

func (t *testSystem) stop() {
	close(t.quit)

	for _, backend := range t.backends {
		backend.engine.Stop()
		backend.Stop()
	}
}

func (t *testSystem) NewBackend(id uint64) *testSystemBackend {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	backend := &testSystemBackend{
		id:         id,
		sys:        t,
		privateKey: privateKey,
		events:     new(event.TypeMux),
	}

	t.backends[id] = backend
	return backend
}

// ==============================================
//

func getPublicKeyAddress(privateKey *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(privateKey.PublicKey)
}
