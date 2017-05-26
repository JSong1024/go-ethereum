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

package simple

import (
	"crypto/ecdsa"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/pbft"
	pbftCore "github.com/ethereum/go-ethereum/consensus/pbft/core"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	extraVanity = 32 // Fixed number of extra-data prefix bytes reserved for signer vanity
	extraSeal   = 65 // Fixed number of extra-data suffix bytes reserved for signer seal
)

func New(config *pbft.Config, eventMux *event.TypeMux, privateKey *ecdsa.PrivateKey, db ethdb.Database) consensus.PBFT {
	backend := &simpleBackend{
		config:       config,
		peerSet:      newPeerSet(),
		eventMux:     eventMux,
		pbftEventMux: new(event.TypeMux),
		privateKey:   privateKey,
		address:      crypto.PubkeyToAddress(privateKey.PublicKey),
		logger:       log.New("backend", "simple"),
		db:           db,
		commitCh:     make(chan common.Hash, 1),
	}
	return backend
}

// ----------------------------------------------------------------------------
type simpleBackend struct {
	config       *pbft.Config
	peerSet      *peerSet
	valSet       pbft.ValidatorSet
	eventMux     *event.TypeMux
	pbftEventMux *event.TypeMux
	privateKey   *ecdsa.PrivateKey
	address      common.Address
	core         pbftCore.Engine
	logger       log.Logger
	quitSync     chan struct{}
	db           ethdb.Database
	timeout      uint64
	chain        consensus.ChainReader
	inserter     func(block *types.Block) error

	// the channels for pbft engine notifications
	commitCh          chan common.Hash
	proposedBlockHash common.Hash
	sealMu            sync.Mutex
}

// Address implements pbft.Backend.Address
func (sb *simpleBackend) Address() common.Address {
	return sb.address
}

// Validators implements pbft.Backend.Validators
func (sb *simpleBackend) Validators() pbft.ValidatorSet {
	return sb.valSet
}

func (sb *simpleBackend) Send(payload []byte, target common.Address) error {
	peer := sb.peerSet.GetByAddress(target)
	if peer == nil {
		return errInvalidPeer
	}

	go sb.eventMux.Post(pbft.ConsensusDataEvent{
		PeerID: peer.ID(),
		Data:   payload,
	})
	return nil
}

// Broadcast implements pbft.Backend.Send
func (sb *simpleBackend) Broadcast(payload []byte) error {
	pbftMsg := pbft.MessageEvent{
		Payload: payload,
	}

	// send to self
	go sb.pbftEventMux.Post(pbftMsg)

	// send to other peers
	for _, peer := range sb.peerSet.List() {
		go sb.eventMux.Post(pbft.ConsensusDataEvent{
			PeerID: peer.ID(),
			Data:   payload,
		})
	}
	return nil
}

// Commit implements pbft.Backend.Commit
func (sb *simpleBackend) Commit(proposal pbft.Proposal) error {
	sb.logger.Info("Committed", "address", sb.Address().Hex(), "hash", proposal.Hash(), "number", proposal.Number().Uint64())
	// step1: update validator set from extra data of block
	// step2: insert chain
	block := &types.Block{}
	block, ok := proposal.(*types.Block)
	if !ok {
		sb.logger.Error("Failed to commit proposal since RequestContext cannot cast to *types.Block")
		return errCastingRequest
	}
	// - if the proposed and committed blocks are the same, send the proposed hash
	//   to commit channel, which is being watched inside the engine.Seal() function.
	// - otherwise, we try to insert the block.
	// -- if success, the ChainHeadEvent event will be broadcasted, try to build
	//    the next block and the previous Seal() will be stopped.
	// -- otherwise, a error will be returned and a round change event will be fired.
	if sb.proposedBlockHash == block.Hash() {
		// feed block hash to Seal() and wait the Seal() result
		sb.commitCh <- block.Hash()
		// TODO: how do we check the block is inserted correctly?
		return nil
	}
	return sb.inserter(block)
}

// RoundChanged implements pbft.Backend.RoundChanged
func (sb *simpleBackend) RoundChanged(needNewProposal bool) error {
	if needNewProposal {
		// if I proposed a block before, fire a ChainHeadEvent to trigger next Seal()
		// and the previous Seal() will be stopped.
		if !common.EmptyHash(sb.proposedBlockHash) {
			go sb.eventMux.Post(core.ChainHeadEvent{})
		}
	}
	return nil
}

// Hash implements pbft.Backend.Hash
func (sb *simpleBackend) Hash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

// EventMux implements pbft.Backend.EventMux
func (sb *simpleBackend) EventMux() *event.TypeMux {
	// not implemented
	return sb.pbftEventMux
}

// Verify implements pbft.Backend.Verify
func (sb *simpleBackend) Verify(proposal pbft.Proposal) error {
	// decode the proposal to block
	block := &types.Block{}
	block, ok := proposal.(*types.Block)
	if !ok {
		sb.logger.Error("Failed to commit proposal since RequestContext cannot cast to *types.Block")
		return errCastingRequest
	}
	// verify the header of proposed block
	return sb.VerifyHeader(sb.chain, block.Header(), false)
}

// Sign implements pbft.Backend.Sign
func (sb *simpleBackend) Sign(data []byte) ([]byte, error) {
	hashData := crypto.Keccak256([]byte(data))
	return crypto.Sign(hashData, sb.privateKey)
}

// CheckSignature implements pbft.Backend.CheckSignature
func (sb *simpleBackend) CheckSignature(data []byte, address common.Address, sig []byte) error {
	signer, err := sb.getSignatureAddress(data, sig)
	if err != nil {
		log.Error("CheckSignature", "error", err)
		return err
	}
	//Compare derived addresses
	if signer != address {
		return pbft.ErrInvalidSignature
	}
	return nil
}

// CheckValidatorSignature implements pbft.Backend.CheckValidatorSignature
func (sb *simpleBackend) CheckValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	// 1. Get signature address
	signer, err := sb.getSignatureAddress(data, sig)
	if err != nil {
		log.Error("CheckValidatorSignature", "error", err)
		return common.Address{}, err
	}

	// 2. Check validator
	if _, val := sb.valSet.GetByAddress(signer); val != nil {
		return val.Address(), nil
	}

	return common.Address{}, pbft.ErrNoMatchingValidator
}

// get the signer address from the signature
func (sb *simpleBackend) getSignatureAddress(data []byte, sig []byte) (common.Address, error) {
	//1. Keccak data
	hashData := crypto.Keccak256([]byte(data))
	//2. Recover public key
	pubkey, err := crypto.SigToPub(hashData, sig)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pubkey), nil
}
