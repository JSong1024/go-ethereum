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
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/pbft"
	pbftCore "github.com/ethereum/go-ethereum/consensus/pbft/core"
	"github.com/ethereum/go-ethereum/consensus/pbft/validator"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	// errUnknownBlock is returned when the list of signers is requested for a block
	// that is not part of the local blockchain.
	errUnknownBlock = errors.New("unknown block")
	// errUnauthorized is returned if a header is signed by a non authorized entity.
	errUnauthorized = errors.New("unauthorized")
	// errInvalidDifficulty is returned if the difficulty of a block is not 1
	errInvalidDifficulty = errors.New("invalid difficulty")
	// errNotProposer is returned when I'm not a proposer
	errNotProposer = errors.New("not a proposer")
	// errViewChanged is returned when we receive a view change event
	errViewChanged = errors.New("view changed")
	// errOtherBlockCommitted is returned when other block is committed.
	errOtherBlockCommitted = errors.New("other block is committed")
	// errInvalidPeer is returned when a message from invalid peer comes
	errInvalidPeer = errors.New("invalid peer")
	// errInvalidExtraDataFormat is returned when the extra data format is incorrect
	errInvalidExtraDataFormat = errors.New("invalid extra data format")
	// errInvalidMixDigest is returned if a block's mix digest is non zero.
	errInvalidMixDigest = errors.New("non-zero mix digest")
	// errInvalidCoinbase is returned if a block's coinbase is non zero.
	errInvalidCoinbase = errors.New("non-zero coinbase")
	// errInvalidUncleHash is returned if a block contains an non-empty uncle list.
	errInvalidUncleHash = errors.New("non empty uncle hash")
)
var (
	defaultDifficulty = big.NewInt(1)
	nilUncleHash      = types.CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
)

// Author retrieves the Ethereum address of the account that minted the given
// block, which may be different from the header's coinbase if a consensus
// engine is based on signatures.
func (sb *simpleBackend) Author(header *types.Header) (common.Address, error) {
	return sb.ecrecover(header)
}

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
func (sb *simpleBackend) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return sb.verifyHeader(chain, header, nil)
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (sb *simpleBackend) verifyHeader(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	if header.Number == nil {
		return errUnknownBlock
	}

	// Don't waste time checking blocks from the future
	if header.Time.Cmp(big.NewInt(time.Now().Unix())) > 0 {
		return consensus.ErrFutureBlock
	}

	// Check that the extra-data contains both the vanity and signature
	length := len(header.Extra)
	if length < extraVanity+extraSeal {
		return errInvalidExtraDataFormat
	}
	if !sb.valSet.CheckFormat(sb.getValidatorBytes(header)) {
		return errInvalidExtraDataFormat
	}

	// Ensure that the coinbase is zero
	if header.Coinbase != (common.Address{}) {
		return errInvalidCoinbase
	}
	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixDigest != (common.Hash{}) {
		return errInvalidMixDigest
	}
	// Ensure that the block doesn't contain any uncles which are meaningless in PBFT
	if header.UncleHash != nilUncleHash {
		return errInvalidUncleHash
	}
	// Ensure that the block's difficulty is meaningful (may not be correct at this point)
	if header.Difficulty == nil || header.Difficulty.Cmp(defaultDifficulty) != 0 {
		return errInvalidDifficulty
	}

	return sb.verifyCascadingFields(chain, header, parents)
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (sb *simpleBackend) verifyCascadingFields(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	// The genesis block is the always valid dead-end
	number := header.Number.Uint64()
	if number == 0 {
		return nil
	}
	// Ensure that the block's timestamp isn't too close to it's parent
	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}
	return sb.VerifySeal(chain, header)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (sb *simpleBackend) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			err := sb.verifyHeader(chain, header, headers[:i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of a given engine.
func (sb *simpleBackend) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return errors.New("uncles not allowed")
	}
	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (sb *simpleBackend) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	// Verifying the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}

	// Resolve the authorization key and check against signers
	signer, err := sb.ecrecover(header)
	if err != nil {
		return err
	}
	if v := sb.valSet.GetByAddress(signer); v == nil {
		return errUnauthorized
	}
	// Ensure that the difficulty corresponts to the turn-ness of the signer
	if header.Difficulty.Cmp(defaultDifficulty) != 0 {
		return errInvalidDifficulty
	}
	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (sb *simpleBackend) Prepare(chain consensus.ChainReader, header *types.Header) error {
	// unused fields, force to set to empty
	header.Coinbase = common.Address{}
	header.Nonce = types.BlockNonce{}
	header.MixDigest = common.Hash{}

	// copy the parent extra data as the header extra data
	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Extra = parent.Extra
	// use the same difficulty for all blocks
	header.Difficulty = defaultDifficulty
	return nil
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
//
// Note, the block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *simpleBackend) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	// No block rewards in PBFT, so the state remains as is and uncles are dropped
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = nilUncleHash

	// Assemble and return the final block for sealing
	return types.NewBlock(header, txs, nil, receipts), nil
}

func (sb *simpleBackend) closeChannels() {
	if sb.viewChange != nil {
		close(sb.viewChange)
		sb.viewChange = nil
	}

	if sb.commit != nil {
		close(sb.commit)
		sb.commit = nil
	}
}

func (sb *simpleBackend) newChannels() {
	sb.viewChange = make(chan bool, 1)
	sb.commit = make(chan common.Hash, 1)
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (sb *simpleBackend) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	if !sb.IsProposer() {
		return nil, errNotProposer
	}

	sb.newChannels()
	defer sb.closeChannels()

	// step 1. sign the hash
	header := block.Header()
	sighash, err := sb.Sign(sigHash(header).Bytes())
	if err != nil {
		return nil, err
	}
	copy(header.Extra[len(header.Extra)-extraSeal:], sighash)
	block = block.WithSeal(header)
	// step 2. feed block into PBFT engine
	b, e := rlp.EncodeToBytes(block)
	if e != nil {
		return nil, e
	}
	go sb.EventMux().Post(pbft.RequestEvent{
		Payload: b,
	})

	for {
		select {
		case needNewProposal := <-sb.viewChange:
			if needNewProposal {
				return nil, errViewChanged
			}
			// if we don't need to change block, we keep waiting events.
		case hash := <-sb.commit:
			if block.Hash() == hash {
				return block, nil
			}
			return nil, errOtherBlockCommitted
		}
	}
}

// APIs returns the RPC APIs this consensus engine provides.
func (sb *simpleBackend) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "pbft",
		Version:   "1.0",
		Service:   &API{chain: chain, backend: sb},
		Public:    true,
	}}
}

// AddPeer implements consensus.PBFT.AddPeer
func (sb *simpleBackend) AddPeer(peerID string, publicKey *ecdsa.PublicKey) error {
	peer := newPeer(peerID, publicKey)
	// check is validator
	if val := sb.valSet.GetByAddress(peer.Address()); val != nil {
		// add to peer set
		sb.peerSet.Add(peer)
		// post connection event to pbft core
		go sb.pbftEventMux.Post(pbft.ConnectionEvent{
			Address: val.Address(),
		})
	}
	return nil
}

// RemovePeer implements consensus.PBFT.RemovePeer
func (sb *simpleBackend) RemovePeer(peerID string) error {
	sb.peerSet.Remove(peerID)
	return nil
}

// HandleMsg implements consensus.PBFT.HandleMsg
func (sb *simpleBackend) HandleMsg(peerID string, data []byte) error {
	peer := sb.peerSet.Get(peerID)
	if peer == nil {
		sb.logger.Error("Not in peer set", "peerID", peerID)
		return errInvalidPeer
	}

	msgEvent, err := Decode(data)
	if err != nil {
		sb.logger.Error("Decode message event failed", "error", err)
		return err
	}

	if val := sb.valSet.GetByAddress(peer.Address()); val == nil {
		sb.logger.Error("Not in validator set", "peerAddr", peer.Address().Hex())
		return errInvalidPeer
	}

	go sb.pbftEventMux.Post(*msgEvent)
	return nil
}

// Start implements consensus.PBFT.Start
func (sb *simpleBackend) Start(chain consensus.ChainReader) error {
	if err := sb.initValidatorSet(chain); err != nil {
		return err
	}
	sb.core = pbftCore.New(sb)
	return sb.core.Start()
}

// Stop implements consensus.PBFT.Stop
func (sb *simpleBackend) Stop() error {
	return sb.core.Stop()
}

func (sb *simpleBackend) initValidatorSet(chain consensus.ChainReader) error {
	header := chain.CurrentHeader()
	// get the validator byte array and feed into validator set
	valSet, r := validator.NewSet(sb.getValidatorBytes(header))
	if !r || valSet == nil {
		return errInvalidExtraDataFormat
	}
	sb.valSet = valSet
	return nil
}

func (sb *simpleBackend) getValidatorBytes(header *types.Header) []byte {
	length := len(header.Extra)
	return header.Extra[extraVanity : length-extraSeal]
}

// FIXME: Need to update this for PBFT
// sigHash returns the hash which is used as input for the PBFT
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func sigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewKeccak256()

	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.UncleHash,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra[:len(header.Extra)-extraSeal], // Yes, this will panic if extra is too short
		header.MixDigest,
		header.Nonce,
	})
	hasher.Sum(hash[:0])
	return hash
}

// ecrecover extracts the Ethereum account address from a signed header.
func (sb *simpleBackend) ecrecover(header *types.Header) (common.Address, error) {
	// Retrieve the signature from the header extra-data
	if len(header.Extra) < extraSeal {
		return common.Address{}, consensus.ErrMissingSignature
	}
	signature := header.Extra[len(header.Extra)-extraSeal:]
	return sb.getSignatureAddress(sigHash(header).Bytes(), signature)
}
