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
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func TestHandleCheckpoint(t *testing.T) {
	N := uint64(4)
	F := uint64(1)
	preprepare := &istanbul.Preprepare{
		View: &istanbul.View{
			Round:    big.NewInt(0),
			Sequence: big.NewInt(3),
		},
	}
	system := NewTestSystemWithBackend(N, F)
	c := system.backends[0].engine.(*core)
	c.current = newSnapshot(&istanbul.View{
		Sequence: big.NewInt(3),
		Round:    big.NewInt(0),
	}, system.backends[0].Validators())
	c.snapshots = append(c.snapshots, newSnapshot(&istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(1),
	}, system.backends[0].Validators()), newSnapshot(&istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(2),
	}, system.backends[0].Validators()))

	testCases := []struct {
		system      *testSystem
		subject     *istanbul.Subject
		src         istanbul.Validator
		snapshot    *snapshot
		expectedErr error
	}{
		// empty subject
		{system, &istanbul.Subject{View: &istanbul.View{Sequence: big.NewInt(0), Round: big.NewInt(0)}}, system.backends[0].Validators().List()[0], nil, errInvalidMessage},
		// current sequence
		{system, &istanbul.Subject{View: &istanbul.View{Sequence: preprepare.View.Sequence}}, system.backends[0].Validators().List()[0], c.current, nil},
		// old sequence
		{system, &istanbul.Subject{View: &istanbul.View{Sequence: big.NewInt(2)}}, system.backends[0].Validators().List()[0], c.snapshots[1], nil},
		// old sequence without snapshot
		{system, &istanbul.Subject{View: &istanbul.View{Sequence: big.NewInt(0)}}, system.backends[0].Validators().List()[0], nil, errInvalidMessage},
		// future sequence
		{system, &istanbul.Subject{View: &istanbul.View{Sequence: big.NewInt(4)}}, system.backends[0].Validators().List()[0], nil, errInvalidMessage},
	}

	for _, test := range testCases {
		var oldSize int
		if test.snapshot != nil {
			oldSize = test.snapshot.Checkpoints.Size()
		}

		cp, err := Encode(test.subject)
		if err != nil && test.subject != nil {
			t.Errorf("encode error, got: %v", err)
			continue
		}
		msg := &message{
			Msg:     cp,
			Address: test.src.Address(),
		}
		err = c.handleCheckpoint(msg, test.src)
		if err != test.expectedErr {
			t.Errorf("unexpected error, got: %v, expected: %v", err, test.expectedErr)
			continue
		}
		if err == nil {
			newSize := test.snapshot.Checkpoints.Size()
			if newSize != oldSize+1 {
				t.Errorf("unexpected checkpoint size, old: %v, new: %v", oldSize, newSize)
			}
		}
	}
}

func TestBuildStableCheckpoint(t *testing.T) {
	N := uint64(1)
	F := uint64(0)
	system := NewTestSystemWithBackend(N, F)
	c := system.backends[0].engine.(*core)
	v := system.backends[0].Validators().List()[0]
	proposal := makeBlock(1)
	view := &istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(1),
	}
	expectedStableSnapshot := newSnapshot(view, system.backends[0].Validators())
	expectedStableSnapshot.Preprepare = &istanbul.Preprepare{
		View:     view,
		Proposal: proposal,
	}
	view = &istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(2),
	}
	s := newSnapshot(view, system.backends[0].Validators())
	s.Preprepare = &istanbul.Preprepare{
		View:     view,
		Proposal: proposal,
	}
	c.snapshots = append(c.snapshots, expectedStableSnapshot, s)

	sub := &istanbul.Subject{View: expectedStableSnapshot.Preprepare.View}
	b, _ := Encode(sub)
	msg := &message{Msg: b, Address: v.Address()}
	if err := expectedStableSnapshot.Checkpoints.Add(msg); err != nil {
		t.Errorf("unexpected error, got: %v", err)
	}
	c.buildStableCheckpoint()
	if length := len(c.snapshots); length != 1 {
		t.Errorf("unexpected snapshots length, got: %v, expected: 1", length)
	}
	var stableCheckpoint *snapshot
	if err := c.backend.Restore(keyStableCheckpoint, &stableCheckpoint); err != nil {
		t.Errorf("restore stable checkpoint failed, error: %v", err)
	}
	if stableCheckpoint == nil {
		t.Errorf("cannot get the stable checkpoint")
	} else if stableCheckpoint.Sequence().Cmp(expectedStableSnapshot.Sequence()) != 0 {
		t.Errorf("unexpected stable check point, got: %v, expected: %v", stableCheckpoint.Sequence(), expectedStableSnapshot.Sequence())
	}
}
