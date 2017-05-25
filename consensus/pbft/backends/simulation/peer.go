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

package simulation

import (
	"crypto/rand"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
)

func newPeer(newPeerID uint64) *peer {
	// Create a message pipe to communicate through
	in, out := p2p.MsgPipe()

	// Generate a random id and create the peer
	var id discover.NodeID
	rand.Read(id[:])

	p := &peer{
		id:  newPeerID,
		in:  in,
		out: out,
		p:   p2p.NewPeer(id, id.String(), nil),
	}

	return p
}

type peer struct {
	id  uint64
	in  *p2p.MsgPipeRW
	out *p2p.MsgPipeRW
	p   *p2p.Peer
}

func (p *peer) ID() uint64 {
	return p.id
}

// close terminates the local side of the peer, notifying the remote protocol
// manager of termination.
func (p *peer) close() {
	p.in.Close()
	p.out.Close()
}

func (p *peer) ReadMsg() (p2p.Msg, error) {
	return p.out.ReadMsg()
}

func (p *peer) WriteMsg(msg p2p.Msg) error {
	return p.in.WriteMsg(msg)
}

func (p *peer) Address() common.Address {
	return common.BigToAddress(big.NewInt(int64(p.id)))
}
