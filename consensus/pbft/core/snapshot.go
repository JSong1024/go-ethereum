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
	"io"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/consensus/pbft"
	"github.com/ethereum/go-ethereum/rlp"
)

func newSnapshot(view *pbft.View, validatorSet pbft.ValidatorSet) *snapshot {
	return &snapshot{
		round:       view.Round,
		sequence:    view.Sequence,
		Preprepare:  nil,
		Prepares:    newMessageSet(validatorSet),
		Commits:     newMessageSet(validatorSet),
		Checkpoints: newMessageSet(validatorSet),
		mu:          new(sync.Mutex),
	}
}

type snapshot struct {
	round       *big.Int
	sequence    *big.Int
	Preprepare  *pbft.Preprepare
	Prepares    *messageSet
	Commits     *messageSet
	Checkpoints *messageSet

	mu *sync.Mutex
}

func (s *snapshot) Proposal() pbft.Proposal {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Preprepare != nil {
		return s.Preprepare.Proposal
	}

	return nil
}

func (s *snapshot) SetRound(r *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.round = new(big.Int).Set(r)
}

func (s *snapshot) Round() *big.Int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.round
}

func (s *snapshot) SetSequence(seq *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sequence = seq
}

func (s *snapshot) Sequence() *big.Int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.sequence
}

// The DecodeRLP method should read one value from the given
// Stream. It is not forbidden to read less or more, but it might
// be confusing.
func (s *snapshot) DecodeRLP(stream *rlp.Stream) error {
	var ss struct {
		Round       *big.Int
		Sequence    *big.Int
		Preprepare  *pbft.Preprepare
		Prepares    *messageSet
		Commits     *messageSet
		Checkpoints *messageSet
	}

	if err := stream.Decode(&ss); err != nil {
		return err
	}
	s.round = ss.Round
	s.sequence = ss.Sequence
	s.Preprepare = ss.Preprepare
	s.Prepares = ss.Prepares
	s.Commits = ss.Commits
	s.Checkpoints = ss.Checkpoints
	s.mu = new(sync.Mutex)

	return nil
}

// EncodeRLP should write the RLP encoding of its receiver to w.
// If the implementation is a pointer method, it may also be
// called for nil pointers.
//
// Implementations should generate valid RLP. The data written is
// not verified at the moment, but a future version might. It is
// recommended to write only a single value but writing multiple
// values or no value at all is also permitted.
func (s *snapshot) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		s.round,
		s.sequence,
		s.Preprepare,
		s.Prepares,
		s.Commits,
		s.Checkpoints,
	})
}
