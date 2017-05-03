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
	"fmt"

	"github.com/ethereum/go-ethereum/consensus/pbft"
)

// Start implements core.Engine.Start
func (c *core) Start() error {
	go func() {
		for event := range c.events.Chan() {
			// A real event arrived, process interesting content
			switch ev := event.Data.(type) {
			case pbft.CheckpointEvent:
				// TODO: we only implement sequence and digest now
				c.sendCheckpoint(&pbft.Checkpoint{
					Sequence: ev.BlockNumber,
					Digest:   ev.BlockHash.Bytes(),
				})
			case pbft.ConnectionEvent:

			case pbft.RequestEvent:
				c.handleRequest(&pbft.Request{
					Payload: ev.Payload,
				}, c.backend.Validators().GetByAddress(c.address))
			case pbft.MessageEvent:
				c.handleMsg(ev.Payload, c.backend.Validators().GetByAddress(ev.Address))
			case backlogEvent:
				c.handle(ev.msg, ev.src)
			}
		}
	}()
	return nil
}

// Stop implements core.Engine.Stop
func (c *core) Stop() error {
	c.events.Unsubscribe()
	return nil
}

func (c *core) handleMsg(payload []byte, src pbft.Validator) error {
	logger := c.logger.New("address", c.address.Hex(), "from", src.Address().Hex())
	var msg pbft.Message

	err := pbft.Decode(payload, &msg)
	if err != nil {
		logger.Error("Failed to decode message", "error", err)
		return err
	}

	return c.handle(&msg, src)
}

func (c *core) handle(msg *pbft.Message, src pbft.Validator) error {
	logger := c.logger.New("address", c.address.Hex(), "from", src.Address().Hex())

	testBacklog := func(err error) error {
		if err == errFutureMessage {
			c.storeBacklog(msg, src)
			return nil
		}

		return err
	}

	switch msg.Code {
	case pbft.MsgPreprepare:
		m, ok := msg.Msg.(*pbft.Preprepare)
		if !ok {
			return fmt.Errorf("failed to decode Preprepare")
		}
		return testBacklog(c.handlePreprepare(m, src))
	case pbft.MsgPrepare:
		m, ok := msg.Msg.(*pbft.Subject)
		if !ok {
			return fmt.Errorf("failed to decode Prepare")
		}
		return testBacklog(c.handlePrepare(m, src))
	case pbft.MsgCommit:
		m, ok := msg.Msg.(*pbft.Subject)
		if !ok {
			return fmt.Errorf("failed to decode Commit")
		}
		return testBacklog(c.handleCommit(m, src))
	case pbft.MsgCheckpoint:
		m, ok := msg.Msg.(*pbft.Checkpoint)
		if !ok {
			return fmt.Errorf("failed to decode Commit")
		}
		return c.handleCheckpoint(m, src)
	case pbft.MsgViewChange:
	case pbft.MsgNewView:
	default:
		logger.Error("Invalid message", "msg", msg)
	}

	return nil
}
