package gasprice

import (
	"context"
	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/rpc"
	lru "github.com/hashicorp/golang-lru"
	"math/big"
	"sort"
	"sync"
)

var log = logger.NewLogger("[gasprice]")

const sampleNumber = 3 // Number of transactions sampled in a block

var (
	DefaultMaxPrice    = big.NewInt(500 * config.GWei)
	DefaultIgnorePrice = big.NewInt(2 * config.Wei)
)

type Config struct {
	Blocks           int
	Percentile       int
	MaxHeaderHistory int
	MaxBlockHistory  int
	Default          *big.Int `toml:",omitempty"`
	MaxPrice         *big.Int `toml:",omitempty"`
	IgnorePrice      *big.Int `toml:",omitempty"`
}

// OracleBackend includes all necessary background APIs for oracle.
type OracleBackend interface {
	HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Header, error)
	BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*model.Block, error)
	GetReceipts(ctx context.Context, hash common.Hash) (model.Receipts, error)
	PendingBlockAndReceipts() (*model.Block, model.Receipts)
	ChainConfig() *config.ChainConfig
	SubscribeChainHeadEvent(ch chan<- blockchain.ChainHeadEvent) event.Subscription
}

// Oracle recommends gas prices based on the content of recent
// blocks. Suitable for both light and full clients.
type Oracle struct {
	backend     OracleBackend
	lastHead    common.Hash
	lastPrice   *big.Int
	maxPrice    *big.Int
	ignorePrice *big.Int
	cacheLock   sync.RWMutex
	fetchLock   sync.Mutex

	checkBlocks, percentile           int
	maxHeaderHistory, maxBlockHistory int
	historyCache                      *lru.Cache
}

// NewOracle returns a new gasprice oracle which can recommend suitable
// gasprice for newly created transaction.
func NewOracle(backend OracleBackend, config Config) *Oracle {
	blocks := config.Blocks
	if blocks < 1 {
		blocks = 1
		log.Warning("Sanitizing invalid gasprice oracle sample blocks", "provided", config.Blocks, "updated", blocks)
	}
	percent := config.Percentile
	if percent < 0 {
		percent = 0
		log.Warning("Sanitizing invalid gasprice oracle sample percentile", "provided", config.Percentile, "updated", percent)
	} else if percent > 100 {
		percent = 100
		log.Warning("Sanitizing invalid gasprice oracle sample percentile", "provided", config.Percentile, "updated", percent)
	}
	maxPrice := config.MaxPrice
	if maxPrice == nil || maxPrice.Int64() <= 0 {
		maxPrice = DefaultMaxPrice
		log.Warning("Sanitizing invalid gasprice oracle price cap", "provided", config.MaxPrice, "updated", maxPrice)
	}
	ignorePrice := config.IgnorePrice
	if ignorePrice == nil || ignorePrice.Int64() <= 0 {
		ignorePrice = DefaultIgnorePrice
		log.Warning("Sanitizing invalid gasprice oracle ignore price", "provided", config.IgnorePrice, "updated", ignorePrice)
	} else if ignorePrice.Int64() > 0 {
		log.Info("Gasprice oracle is ignoring threshold set", "threshold", ignorePrice)
	}
	maxHeaderHistory := config.MaxHeaderHistory
	if maxHeaderHistory < 1 {
		maxHeaderHistory = 1
		log.Warning("Sanitizing invalid gasprice oracle max header history", "provided", config.MaxHeaderHistory, "updated", maxHeaderHistory)
	}
	maxBlockHistory := config.MaxBlockHistory
	if maxBlockHistory < 1 {
		maxBlockHistory = 1
		log.Warning("Sanitizing invalid gasprice oracle max block history", "provided", config.MaxBlockHistory, "updated", maxBlockHistory)
	}

	cache, _ := lru.New(2048)
	headEvent := make(chan blockchain.ChainHeadEvent, 1)
	backend.SubscribeChainHeadEvent(headEvent)
	go func() {
		var lastHead common.Hash
		for ev := range headEvent {
			if ev.Block.ParentHash() != lastHead {
				cache.Purge()
			}
			lastHead = ev.Block.Hash()
		}
	}()

	return &Oracle{
		backend:          backend,
		lastPrice:        config.Default,
		maxPrice:         maxPrice,
		ignorePrice:      ignorePrice,
		checkBlocks:      blocks,
		percentile:       percent,
		maxHeaderHistory: maxHeaderHistory,
		maxBlockHistory:  maxBlockHistory,
		historyCache:     cache,
	}
}

// SuggestTipCap returns a tip cap so that newly created transaction can have a
// very high chance to be included in the following blocks.
//
// Note, for legacy transactions and the legacy eth_gasPrice RPC call, it will be
// necessary to add the basefee to the returned number to fall back to the legacy
// behavior.
func (oracle *Oracle) SuggestTipCap(ctx context.Context) (*big.Int, error) {
	head, _ := oracle.backend.HeaderByNumber(ctx, rpc.LatestBlockNumber)
	headHash := head.Hash()

	// If the latest gasprice is still available, return it.
	oracle.cacheLock.RLock()
	lastHead, lastPrice := oracle.lastHead, oracle.lastPrice
	oracle.cacheLock.RUnlock()
	if headHash == lastHead {
		return new(big.Int).Set(lastPrice), nil
	}
	oracle.fetchLock.Lock()
	defer oracle.fetchLock.Unlock()

	// Try checking the cache again, maybe the last fetch fetched what we need
	oracle.cacheLock.RLock()
	lastHead, lastPrice = oracle.lastHead, oracle.lastPrice
	oracle.cacheLock.RUnlock()
	if headHash == lastHead {
		return new(big.Int).Set(lastPrice), nil
	}
	var (
		sent, exp int
		number    = head.Number.Uint64()
		result    = make(chan results, oracle.checkBlocks)
		quit      = make(chan struct{})
		results   []*big.Int
	)
	for sent < oracle.checkBlocks && number > 0 {
		go oracle.getBlockValues(ctx, model.MakeSigner(oracle.backend.ChainConfig(), big.NewInt(int64(number))), number, sampleNumber, oracle.ignorePrice, result, quit)
		sent++
		exp++
		number--
	}
	for exp > 0 {
		res := <-result
		if res.err != nil {
			close(quit)
			return new(big.Int).Set(lastPrice), res.err
		}
		exp--
		// Nothing returned. There are two special cases here:
		// - The block is empty
		// - All the transactions included are sent by the miner itself.
		// In these cases, use the latest calculated price for sampling.
		if len(res.values) == 0 {
			res.values = []*big.Int{lastPrice}
		}
		// Besides, in order to collect enough data for sampling, if nothing
		// meaningful returned, try to query more blocks. But the maximum
		// is 2*checkBlocks.
		if len(res.values) == 1 && len(results)+1+exp < oracle.checkBlocks*2 && number > 0 {
			go oracle.getBlockValues(ctx, model.MakeSigner(oracle.backend.ChainConfig(), big.NewInt(int64(number))), number, sampleNumber, oracle.ignorePrice, result, quit)
			sent++
			exp++
			number--
		}
		results = append(results, res.values...)
	}
	price := lastPrice
	if len(results) > 0 {
		sort.Sort(bigIntArray(results))
		price = results[(len(results)-1)*oracle.percentile/100]
	}
	if price.Cmp(oracle.maxPrice) > 0 {
		price = new(big.Int).Set(oracle.maxPrice)
	}
	oracle.cacheLock.Lock()
	oracle.lastHead = headHash
	oracle.lastPrice = price
	oracle.cacheLock.Unlock()

	return new(big.Int).Set(price), nil
}

type results struct {
	values []*big.Int
	err    error
}

type txSorter struct {
	txs     []*model.Transaction
	baseFee *big.Int
}

func newSorter(txs []*model.Transaction, baseFee *big.Int) *txSorter {
	return &txSorter{
		txs:     txs,
		baseFee: baseFee,
	}
}

func (s *txSorter) Len() int { return len(s.txs) }
func (s *txSorter) Swap(i, j int) {
	s.txs[i], s.txs[j] = s.txs[j], s.txs[i]
}
func (s *txSorter) Less(i, j int) bool {
	// It's okay to discard the error because a tx would never be
	// accepted into a block with an invalid effective tip.
	tip1, _ := s.txs[i].EffectiveGasTip(s.baseFee)
	tip2, _ := s.txs[j].EffectiveGasTip(s.baseFee)
	return tip1.Cmp(tip2) < 0
}

// getBlockPrices calculates the lowest transaction gas price in a given block
// and sends it to the result channel. If the block is empty or all transactions
// are sent by the miner itself(it doesn't make any sense to include this kind of
// transaction prices for sampling), nil gasprice is returned.
func (oracle *Oracle) getBlockValues(ctx context.Context, signer model.Signer, blockNum uint64, limit int, ignoreUnder *big.Int, result chan results, quit chan struct{}) {
	block, err := oracle.backend.BlockByNumber(ctx, rpc.BlockNumber(blockNum))
	if block == nil {
		select {
		case result <- results{nil, err}:
		case <-quit:
		}
		return
	}
	// Sort the transaction by effective tip in ascending sort.
	txs := make([]*model.Transaction, len(block.Transactions()))
	copy(txs, block.Transactions())
	sorter := newSorter(txs, block.BaseFee())
	sort.Sort(sorter)

	var prices []*big.Int
	for _, tx := range sorter.txs {
		tip, _ := tx.EffectiveGasTip(block.BaseFee())
		if ignoreUnder != nil && tip.Cmp(ignoreUnder) == -1 {
			continue
		}
		sender, err := model.Sender(signer, tx)
		if err == nil && sender != block.Coinbase() {
			prices = append(prices, tip)
			if len(prices) >= limit {
				break
			}
		}
	}
	select {
	case result <- results{prices, nil}:
	case <-quit:
	}
}

type bigIntArray []*big.Int

func (s bigIntArray) Len() int           { return len(s) }
func (s bigIntArray) Less(i, j int) bool { return s[i].Cmp(s[j]) < 0 }
func (s bigIntArray) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
