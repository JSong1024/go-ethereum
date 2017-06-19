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

type ProposerPolicy int

const (
	RoundRobin ProposerPolicy = iota
)

type Config struct {
	RequestTimeoutMsec uint64         // The timeout for each PBFT round. This timeout should be larger than BlockPauseTime.
	BlockPeriod        uint64         // Default minimum difference between two consecutive block's timestamps in second
	BlockPauseTime     uint64         // Pause time when zero tx in previous block, values should be larger than pbft_block_period
	ProposerPolicy     ProposerPolicy // The policy for proposer, the detail is not determined
	CheckPointPeriod   int            // Synchronizes the mapping's checkpoint to the blocks on each round
}

var DefaultConfig = &Config{
	RequestTimeoutMsec: 10000,
	BlockPeriod:        1,
	BlockPauseTime:     1,
	ProposerPolicy:     RoundRobin,
	CheckPointPeriod:   100,
}
