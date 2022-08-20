// Package catalyst implements the temporary eth1/eth2 RPC integration.
package catalyst

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/beacon"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/hexutil"
	"github.com/entropyio/go-entropy/database/rawdb"
	"github.com/entropyio/go-entropy/entropy"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/rpc"
	"github.com/entropyio/go-entropy/server/node"
	"sync"
	"time"
)

var log = logger.NewLogger("[catalyst]")

// Register adds catalyst APIs to the full node.
func Register(stack *node.Node, backend *entropy.Entropy) error {
	log.Warning("Catalyst mode enabled", "protocol", "eth")
	stack.RegisterAPIs([]rpc.API{
		{
			Namespace:     "engine",
			Version:       "1.0",
			Service:       NewConsensusAPI(backend),
			Authenticated: true,
		},
	})
	return nil
}

type ConsensusAPI struct {
	entropy      *entropy.Entropy
	remoteBlocks *headerQueue  // Cache of remote payloads received
	localBlocks  *payloadQueue // Cache of local payloads generated
	// Lock for the forkChoiceUpdated method
	forkChoiceLock sync.Mutex
}

// NewConsensusAPI creates a new consensus api for the given backend.
// The underlying blockchain needs to have a valid terminal total difficulty set.
func NewConsensusAPI(entropy *entropy.Entropy) *ConsensusAPI {
	if entropy.BlockChain().Config().TerminalTotalDifficulty == nil {
		panic("Catalyst started without valid total difficulty")
	}
	return &ConsensusAPI{
		entropy:      entropy,
		remoteBlocks: newHeaderQueue(),
		localBlocks:  newPayloadQueue(),
	}
}

// ForkchoiceUpdatedV1 has several responsibilities:
// If the method is called with an empty head block:
// 		we return success, which can be used to check if the catalyst mode is enabled
// If the total difficulty was not reached:
// 		we return INVALID
// If the finalizedBlockHash is set:
// 		we check if we have the finalizedBlockHash in our db, if not we start a sync
// We try to set our blockchain to the headBlock
// If there are payloadAttributes:
// 		we try to assemble a block with the payloadAttributes and return its payloadID
func (api *ConsensusAPI) ForkchoiceUpdatedV1(update beacon.ForkchoiceStateV1, payloadAttributes *beacon.PayloadAttributesV1) (beacon.ForkChoiceResponse, error) {
	api.forkChoiceLock.Lock()
	defer api.forkChoiceLock.Unlock()

	log.Debug("Engine API request received", "method", "ForkchoiceUpdated", "head", update.HeadBlockHash, "finalized", update.FinalizedBlockHash, "safe", update.SafeBlockHash)
	if update.HeadBlockHash == (common.Hash{}) {
		log.Warning("Forkchoice requested update to zero hash")
		return beacon.STATUS_INVALID, nil // TODO(karalabe): Why does someone send us this?
	}

	// Check whether we have the block yet in our database or not. If not, we'll
	// need to either trigger a sync, or to reject this forkchoice update for a
	// reason.
	block := api.entropy.BlockChain().GetBlockByHash(update.HeadBlockHash)
	if block == nil {
		// If the head hash is unknown (was not given to us in a newPayload request),
		// we cannot resolve the header, so not much to do. This could be extended in
		// the future to resolve from the `eth` network, but it's an unexpected case
		// that should be fixed, not papered over.
		header := api.remoteBlocks.get(update.HeadBlockHash)
		if header == nil {
			log.Warning("Forkchoice requested unknown head", "hash", update.HeadBlockHash)
			return beacon.STATUS_SYNCING, nil
		}
		// Header advertised via a past newPayload request. Start syncing to it.
		// Before we do however, make sure any legacy sync in switched off so we
		// don't accidentally have 2 cycles running.
		if merger := api.entropy.Merger(); !merger.TDDReached() {
			merger.ReachTTD()
			api.entropy.Downloader().Cancel()
		}
		log.Info("Forkchoice requested sync to new head", "number", header.Number, "hash", header.Hash())
		if err := api.entropy.Downloader().BeaconSync(api.entropy.SyncMode(), header); err != nil {
			return beacon.STATUS_SYNCING, err
		}
		return beacon.STATUS_SYNCING, nil
	}
	// Block is known locally, just sanity check that the beacon client does not
	// attempt to push us back to before the merge.
	if block.Difficulty().BitLen() > 0 || block.NumberU64() == 0 {
		var (
			td  = api.entropy.BlockChain().GetTd(update.HeadBlockHash, block.NumberU64())
			ptd = api.entropy.BlockChain().GetTd(block.ParentHash(), block.NumberU64()-1)
			ttd = api.entropy.BlockChain().Config().TerminalTotalDifficulty
		)
		if td == nil || (block.NumberU64() > 0 && ptd == nil) {
			log.Error("TDs unavailable for TTD check", "number", block.NumberU64(), "hash", update.HeadBlockHash, "td", td, "parent", block.ParentHash(), "ptd", ptd)
			return beacon.STATUS_INVALID, errors.New("TDs unavailable for TDD check")
		}
		if td.Cmp(ttd) < 0 {
			log.Error("Refusing beacon update to pre-merge", "number", block.NumberU64(), "hash", update.HeadBlockHash, "diff", block.Difficulty(), "age", common.PrettyAge(time.Unix(int64(block.Time()), 0)))
			return beacon.ForkChoiceResponse{PayloadStatus: beacon.INVALID_TERMINAL_BLOCK, PayloadID: nil}, nil
		}
		if block.NumberU64() > 0 && ptd.Cmp(ttd) >= 0 {
			log.Error("Parent block is already post-ttd", "number", block.NumberU64(), "hash", update.HeadBlockHash, "diff", block.Difficulty(), "age", common.PrettyAge(time.Unix(int64(block.Time()), 0)))
			return beacon.ForkChoiceResponse{PayloadStatus: beacon.INVALID_TERMINAL_BLOCK, PayloadID: nil}, nil
		}
	}
	valid := func(id *beacon.PayloadID) beacon.ForkChoiceResponse {
		return beacon.ForkChoiceResponse{
			PayloadStatus: beacon.PayloadStatusV1{Status: beacon.VALID, LatestValidHash: &update.HeadBlockHash},
			PayloadID:     id,
		}
	}
	if rawdb.ReadCanonicalHash(api.entropy.ChainDb(), block.NumberU64()) != update.HeadBlockHash {
		// Block is not canonical, set head.
		if latestValid, err := api.entropy.BlockChain().SetCanonical(block); err != nil {
			return beacon.ForkChoiceResponse{PayloadStatus: beacon.PayloadStatusV1{Status: beacon.INVALID, LatestValidHash: &latestValid}}, err
		}
	} else if api.entropy.BlockChain().CurrentBlock().Hash() == update.HeadBlockHash {
		// If the specified head matches with our local head, do nothing and keep
		// generating the payload. It's a special corner case that a few slots are
		// missing and we are requested to generate the payload in slot.
	} else {
		// If the head block is already in our canonical chain, the beacon client is
		// probably resyncing. Ignore the update.
		log.Info("Ignoring beacon update to old head", "number", block.NumberU64(), "hash", update.HeadBlockHash, "age", common.PrettyAge(time.Unix(int64(block.Time()), 0)), "have", api.entropy.BlockChain().CurrentBlock().NumberU64())
		return valid(nil), nil
	}
	api.entropy.SetSynced()

	// If the beacon client also advertised a finalized block, mark the local
	// chain final and completely in PoS mode.
	if update.FinalizedBlockHash != (common.Hash{}) {
		if merger := api.entropy.Merger(); !merger.PoSFinalized() {
			merger.FinalizePoS()
		}
		// If the finalized block is not in our canonical tree, somethings wrong
		finalBlock := api.entropy.BlockChain().GetBlockByHash(update.FinalizedBlockHash)
		if finalBlock == nil {
			log.Warning("Final block not available in database", "hash", update.FinalizedBlockHash)
			return beacon.STATUS_INVALID, beacon.InvalidForkChoiceState.With(errors.New("final block not available in database"))
		} else if rawdb.ReadCanonicalHash(api.entropy.ChainDb(), finalBlock.NumberU64()) != update.FinalizedBlockHash {
			log.Warning("Final block not in canonical chain", "number", block.NumberU64(), "hash", update.HeadBlockHash)
			return beacon.STATUS_INVALID, beacon.InvalidForkChoiceState.With(errors.New("final block not in canonical chain"))
		}
		// Set the finalized block
		api.entropy.BlockChain().SetFinalized(finalBlock)
	}
	// Check if the safe block hash is in our canonical tree, if not somethings wrong
	if update.SafeBlockHash != (common.Hash{}) {
		safeBlock := api.entropy.BlockChain().GetBlockByHash(update.SafeBlockHash)
		if safeBlock == nil {
			log.Warning("Safe block not available in database")
			return beacon.STATUS_INVALID, beacon.InvalidForkChoiceState.With(errors.New("safe block not available in database"))
		}
		if rawdb.ReadCanonicalHash(api.entropy.ChainDb(), safeBlock.NumberU64()) != update.SafeBlockHash {
			log.Warning("Safe block not in canonical chain")
			return beacon.STATUS_INVALID, beacon.InvalidForkChoiceState.With(errors.New("safe block not in canonical chain"))
		}
	}
	// If payload generation was requested, create a new block to be potentially
	// sealed by the beacon client. The payload will be requested later, and we
	// might replace it arbitrarily many times in between.
	if payloadAttributes != nil {
		// Create an empty block first which can be used as a fallback
		empty, err := api.entropy.Miner().GetSealingBlockSync(update.HeadBlockHash, payloadAttributes.Timestamp, payloadAttributes.SuggestedFeeRecipient, payloadAttributes.Random, true)
		if err != nil {
			log.Error("Failed to create empty sealing payload", "err", err)
			return valid(nil), beacon.InvalidPayloadAttributes.With(err)
		}
		// Send a request to generate a full block in the background.
		// The result can be obtained via the returned channel.
		resCh, err := api.entropy.Miner().GetSealingBlockAsync(update.HeadBlockHash, payloadAttributes.Timestamp, payloadAttributes.SuggestedFeeRecipient, payloadAttributes.Random, false)
		if err != nil {
			log.Error("Failed to create async sealing payload", "err", err)
			return valid(nil), beacon.InvalidPayloadAttributes.With(err)
		}
		id := computePayloadId(update.HeadBlockHash, payloadAttributes)
		api.localBlocks.put(id, &payload{empty: empty, result: resCh})
		return valid(&id), nil
	}
	return valid(nil), nil
}

// ExchangeTransitionConfigurationV1 checks the given configuration against
// the configuration of the node.
func (api *ConsensusAPI) ExchangeTransitionConfigurationV1(config beacon.TransitionConfigurationV1) (*beacon.TransitionConfigurationV1, error) {
	if config.TerminalTotalDifficulty == nil {
		return nil, errors.New("invalid terminal total difficulty")
	}
	ttd := api.entropy.BlockChain().Config().TerminalTotalDifficulty
	if ttd.Cmp(config.TerminalTotalDifficulty.ToInt()) != 0 {
		log.Warning("Invalid TTD configured", "geth", ttd, "beacon", config.TerminalTotalDifficulty)
		return nil, fmt.Errorf("invalid ttd: execution %v consensus %v", ttd, config.TerminalTotalDifficulty)
	}

	if config.TerminalBlockHash != (common.Hash{}) {
		if hash := api.entropy.BlockChain().GetCanonicalHash(uint64(config.TerminalBlockNumber)); hash == config.TerminalBlockHash {
			return &beacon.TransitionConfigurationV1{
				TerminalTotalDifficulty: (*hexutil.Big)(ttd),
				TerminalBlockHash:       config.TerminalBlockHash,
				TerminalBlockNumber:     config.TerminalBlockNumber,
			}, nil
		}
		return nil, fmt.Errorf("invalid terminal block hash")
	}
	return &beacon.TransitionConfigurationV1{TerminalTotalDifficulty: (*hexutil.Big)(ttd)}, nil
}

// GetPayloadV1 returns a cached payload by id.
func (api *ConsensusAPI) GetPayloadV1(payloadID beacon.PayloadID) (*beacon.ExecutableDataV1, error) {
	log.Debug("Engine API request received", "method", "GetPayload", "id", payloadID)
	data := api.localBlocks.get(payloadID)
	if data == nil {
		return nil, beacon.UnknownPayload
	}
	return data, nil
}

// NewPayloadV1 creates an Eth1 block, inserts it in the chain, and returns the status of the chain.
func (api *ConsensusAPI) NewPayloadV1(params beacon.ExecutableDataV1) (beacon.PayloadStatusV1, error) {
	log.Debug("Engine API request received", "method", "ExecutePayload", "number", params.Number, "hash", params.BlockHash)
	block, err := beacon.ExecutableDataToBlock(params)
	if err != nil {
		log.Debug("Invalid NewPayload params", "params", params, "error", err)
		return beacon.PayloadStatusV1{Status: beacon.INVALIDBLOCKHASH}, nil
	}
	// If we already have the block locally, ignore the entire execution and just
	// return a fake success.
	if block := api.entropy.BlockChain().GetBlockByHash(params.BlockHash); block != nil {
		log.Warning("Ignoring already known beacon payload", "number", params.Number, "hash", params.BlockHash, "age", common.PrettyAge(time.Unix(int64(block.Time()), 0)))
		hash := block.Hash()
		return beacon.PayloadStatusV1{Status: beacon.VALID, LatestValidHash: &hash}, nil
	}
	// If the parent is missing, we - in theory - could trigger a sync, but that
	// would also entail a reorg. That is problematic if multiple sibling blocks
	// are being fed to us, and even more so, if some semi-distant uncle shortens
	// our live chain. As such, payload execution will not permit reorgs and thus
	// will not trigger a sync cycle. That is fine though, if we get a fork choice
	// update after legit payload executions.
	parent := api.entropy.BlockChain().GetBlock(block.ParentHash(), block.NumberU64()-1)
	if parent == nil {
		// Stash the block away for a potential forced forckchoice update to it
		// at a later time.
		api.remoteBlocks.put(block.Hash(), block.Header())

		// Although we don't want to trigger a sync, if there is one already in
		// progress, try to extend if with the current payload request to relieve
		// some strain from the forkchoice update.
		if err := api.entropy.Downloader().BeaconExtend(api.entropy.SyncMode(), block.Header()); err == nil {
			log.Debug("Payload accepted for sync extension", "number", params.Number, "hash", params.BlockHash)
			return beacon.PayloadStatusV1{Status: beacon.SYNCING}, nil
		}
		// Either no beacon sync was started yet, or it rejected the delivered
		// payload as non-integratable on top of the existing sync. We'll just
		// have to rely on the beacon client to forcefully update the head with
		// a forkchoice update request.
		log.Warning("Ignoring payload with missing parent", "number", params.Number, "hash", params.BlockHash, "parent", params.ParentHash)
		return beacon.PayloadStatusV1{Status: beacon.ACCEPTED}, nil
	}
	// We have an existing parent, do some sanity checks to avoid the beacon client
	// triggering too early
	var (
		ptd  = api.entropy.BlockChain().GetTd(parent.Hash(), parent.NumberU64())
		ttd  = api.entropy.BlockChain().Config().TerminalTotalDifficulty
		gptd = api.entropy.BlockChain().GetTd(parent.ParentHash(), parent.NumberU64()-1)
	)
	if ptd.Cmp(ttd) < 0 {
		log.Warning("Ignoring pre-merge payload", "number", params.Number, "hash", params.BlockHash, "td", ptd, "ttd", ttd)
		return beacon.INVALID_TERMINAL_BLOCK, nil
	}
	if parent.Difficulty().BitLen() > 0 && gptd != nil && gptd.Cmp(ttd) >= 0 {
		log.Error("Ignoring pre-merge parent block", "number", params.Number, "hash", params.BlockHash, "td", ptd, "ttd", ttd)
		return beacon.INVALID_TERMINAL_BLOCK, nil
	}
	if block.Time() <= parent.Time() {
		log.Warning("Invalid timestamp", "parent", block.Time(), "block", block.Time())
		return api.invalid(errors.New("invalid timestamp"), parent), nil
	}
	if !api.entropy.BlockChain().HasBlockAndState(block.ParentHash(), block.NumberU64()-1) {
		api.remoteBlocks.put(block.Hash(), block.Header())
		log.Warning("State not available, ignoring new payload")
		return beacon.PayloadStatusV1{Status: beacon.ACCEPTED}, nil
	}
	log.Debug("Inserting block without sethead", "hash", block.Hash(), "number", block.Number)
	if err := api.entropy.BlockChain().InsertBlockWithoutSetHead(block); err != nil {
		log.Warning("NewPayloadV1: inserting block failed", "error", err)
		return api.invalid(err, parent), nil
	}
	// We've accepted a valid payload from the beacon client. Mark the local
	// chain transitions to notify other subsystems (e.g. downloader) of the
	// behavioral change.
	if merger := api.entropy.Merger(); !merger.TDDReached() {
		merger.ReachTTD()
		api.entropy.Downloader().Cancel()
	}
	hash := block.Hash()
	return beacon.PayloadStatusV1{Status: beacon.VALID, LatestValidHash: &hash}, nil
}

// computePayloadId computes a pseudo-random payloadid, based on the parameters.
func computePayloadId(headBlockHash common.Hash, params *beacon.PayloadAttributesV1) beacon.PayloadID {
	// Hash
	hasher := sha256.New()
	hasher.Write(headBlockHash[:])
	binary.Write(hasher, binary.BigEndian, params.Timestamp)
	hasher.Write(params.Random[:])
	hasher.Write(params.SuggestedFeeRecipient[:])
	var out beacon.PayloadID
	copy(out[:], hasher.Sum(nil)[:8])
	return out
}

// invalid returns a response "INVALID" with the latest valid hash supplied by latest or to the current head
// if no latestValid block was provided.
func (api *ConsensusAPI) invalid(err error, latestValid *model.Block) beacon.PayloadStatusV1 {
	currentHash := api.entropy.BlockChain().CurrentBlock().Hash()
	if latestValid != nil {
		// Set latest valid hash to 0x0 if parent is PoW block
		currentHash = common.Hash{}
		if latestValid.Difficulty().BitLen() == 0 {
			// Otherwise set latest valid hash to parent hash
			currentHash = latestValid.Hash()
		}
	}
	errorMsg := err.Error()
	return beacon.PayloadStatusV1{Status: beacon.INVALID, LatestValidHash: &currentHash, ValidationError: &errorMsg}
}
