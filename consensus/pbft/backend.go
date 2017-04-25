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

package pbft

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"
)

// Backend provides callbacks for PBFT consensus core
type Backend interface {
	// ID returns self id
	ID() uint64

	// Validators returns validator set
	Validators() *ValidatorSet

	// EventMux is defined to handle request event and pbft message event
	EventMux() *event.TypeMux

	// Send is to send pbft message to peers
	Send([]byte)

	// UpdateState is to update the current pbft state to backend
	UpdateState(*State)

	// Commit is to deliver a final result to write into blockchain
	Commit(*Proposal)

	// ViewChanged is called when view change occurred
	ViewChanged(needNewProposal bool)

	// Verify is to verify the proposal request
	Verify(*Proposal) (bool, error)

	// Sign is to sign the data
	Sign([]byte) ([]byte, error)

	// CheckSignature is to verify the signature is signed from given peer
	CheckSignature(data []byte, addr common.Address, sig []byte) error

	// XXX: Hash, Encode, Decode and SetHandler are workaround functions for developing
	Hash(b interface{}) common.Hash
	Encode(b interface{}) ([]byte, error)
	Decode([]byte, interface{}) error
}
