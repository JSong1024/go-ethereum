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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/pbft"
)

func (c *core) handleFinalCommitted(ev pbft.FinalCommittedEvent, p pbft.Validator) error {
	logger := c.logger.New("state", c.state, "number", ev.Proposal.Number(), "hash", ev.Proposal.Hash())
	// this block is from consensus
	if c.subject != nil &&
		ev.Proposal.Hash() == c.subject.Digest &&
		c.state == StateCommitted {
		logger.Trace("New block from consensus")

		// send out the checkpoint
		c.sendCheckpoint(&pbft.Subject{
			View: &pbft.View{
				Sequence: new(big.Int).Set(ev.Proposal.Number()),
				Round:    new(big.Int).Set(c.current.Round()),
			},
			Digest: ev.Proposal.Hash(),
		})

		// store snapshot
		c.snapshotsMu.Lock()
		c.snapshots = append(c.snapshots, c.current)
		c.snapshotsMu.Unlock()
	} else { // this block is from geth sync
		logger.Trace("New block from synchronization")
	}

	if ev.Proposal.Number().Cmp(c.current.Sequence()) >= 0 {
		// We build a stable checkpoint every 'CheckPointPeriod' proposal
		if new(big.Int).Mod(c.current.Sequence(), big.NewInt(int64(c.config.CheckPointPeriod))).Int64() == 0 {
			go c.sendEvent(buildCheckpointEvent{})
		}

		c.lastProposer = ev.Proposer
		c.startNewRound(&pbft.View{
			Sequence: new(big.Int).Add(ev.Proposal.Number(), common.Big1),
			Round:    new(big.Int).Set(common.Big0),
		}, false)
	}

	return nil
}
