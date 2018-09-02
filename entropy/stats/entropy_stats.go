package entropystats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/entropyio/go-entropy/blockchain"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/timeutil"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/entropy"
	"github.com/entropyio/go-entropy/event"
	"github.com/entropyio/go-entropy/logger"
	"github.com/entropyio/go-entropy/rpc"
	"github.com/entropyio/go-entropy/server/p2p"
	"golang.org/x/net/websocket"
)

var log = logger.NewLogger("[stats]")

const (
	// historyUpdateRange is the number of blocks a node should report upon login or
	// history request.
	historyUpdateRange = 50

	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
)

type txPool interface {
	// SubscribeNewTxsEvent should return an event subscription of
	// NewTxsEvent and send events to the given channel.
	SubscribeNewTxsEvent(chan<- blockchain.NewTxsEvent) event.Subscription
}

type blockChain interface {
	SubscribeChainHeadEvent(ch chan<- blockchain.ChainHeadEvent) event.Subscription
}

// Service implements an Entropy netstats reporting daemon that pushes local
// chain statistics up to a monitoring server.
type Service struct {
	server *p2p.Server      // Peer-to-peer server to retrieve networking infos
	entropy    *entropy.Entropy // Full Entropy service if monitoring a full node
	//les    *les.LightEntropy // Light Entropy service if monitoring a light node
	engine consensus.Engine // Consensus engine to retrieve variadic block fields

	node string // Name of the node to display on the monitoring page
	pass string // Password to authorize access to the monitoring page
	host string // Remote address of the monitoring service

	pongCh chan struct{} // Pong notifications are fed into this channel
	histCh chan []uint64 // History request block numbers are fed into this channel
}

// New returns a monitoring service ready for stats reporting.
func New(url string, ethServ *entropy.Entropy /*lesServ *les.LightEntropy*/) (*Service, error) {
	// Parse the netstats connection url
	re := regexp.MustCompile("([^:@]*)(:([^@]*))?@(.+)")
	parts := re.FindStringSubmatch(url)
	if len(parts) != 5 {
		return nil, fmt.Errorf("invalid netstats url: \"%s\", should be nodename:secret@host:port", url)
	}
	// Assemble and return the stats service
	var engine consensus.Engine
	if ethServ != nil {
		engine = ethServ.Engine()
	} else {
		//engine = lesServ.Engine()
	}
	return &Service{
		entropy: ethServ,
		//les:    lesServ,
		engine: engine,
		node:   parts[1],
		pass:   parts[3],
		host:   parts[4],
		pongCh: make(chan struct{}),
		histCh: make(chan []uint64, 1),
	}, nil
}

// Protocols implements node.Service, returning the P2P network protocols used
// by the stats service (nil as it doesn't use the devp2p overlay network).
func (s *Service) Protocols() []p2p.Protocol { return nil }

// APIs implements node.Service, returning the RPC API endpoints provided by the
// stats service (nil as it doesn't provide any user callable APIs).
func (s *Service) APIs() []rpc.API { return nil }

// Start implements node.Service, starting up the monitoring and reporting daemon.
func (s *Service) Start(server *p2p.Server) error {
	s.server = server
	go s.loop()

	log.Info("Stats daemon started")
	return nil
}

// Stop implements node.Service, terminating the monitoring and reporting daemon.
func (s *Service) Stop() error {
	log.Info("Stats daemon stopped")
	return nil
}

// loop keeps trying to connect to the netstats server, reporting chain events
// until termination.
func (s *Service) loop() {
	// Subscribe to chain events to execute updates on
	var blockchainObj blockChain
	var txpool txPool
	if s.entropy != nil {
		blockchainObj = s.entropy.BlockChain()
		txpool = s.entropy.TxPool()
	} else {
		//blockchain = s.les.BlockChain()
		//txpool = s.les.TxPool()
	}

	chainHeadCh := make(chan blockchain.ChainHeadEvent, chainHeadChanSize)
	headSub := blockchainObj.SubscribeChainHeadEvent(chainHeadCh)
	defer headSub.Unsubscribe()

	txEventCh := make(chan blockchain.NewTxsEvent, txChanSize)
	txSub := txpool.SubscribeNewTxsEvent(txEventCh)
	defer txSub.Unsubscribe()

	// Start a goroutine that exhausts the subsciptions to avoid events piling up
	var (
		quitCh = make(chan struct{})
		headCh = make(chan *model.Block, 1)
		txCh   = make(chan struct{}, 1)
	)
	go func() {
		var lastTx timeutil.AbsTime

	HandleLoop:
		for {
			select {
			// Notify of chain head events, but drop if too frequent
			case head := <-chainHeadCh:
				select {
				case headCh <- head.Block:
				default:
				}

				// Notify of new transaction events, but drop if too frequent
			case <-txEventCh:
				if time.Duration(timeutil.Now()-lastTx) < time.Second {
					continue
				}
				lastTx = timeutil.Now()

				select {
				case txCh <- struct{}{}:
				default:
				}

				// node stopped
			case <-txSub.Err():
				break HandleLoop
			case <-headSub.Err():
				break HandleLoop
			}
		}
		close(quitCh)
	}()
	// Loop reporting until termination
	for {
		// Resolve the URL, defaulting to TLS, but falling back to none too
		path := fmt.Sprintf("%s/api", s.host)
		urls := []string{path}

		if !strings.Contains(path, "://") { // url.Parse and url.IsAbs is unsuitable (https://github.com/golang/go/issues/19779)
			urls = []string{"wss://" + path, "ws://" + path}
		}
		// Establish a websocket connection to the server on any supported URL
		var (
			conf *websocket.Config
			conn *websocket.Conn
			err  error
		)
		for _, url := range urls {
			if conf, err = websocket.NewConfig(url, "http://localhost/"); err != nil {
				continue
			}
			conf.Dialer = &net.Dialer{Timeout: 5 * time.Second}
			if conn, err = websocket.DialConfig(conf); err == nil {
				break
			}
		}
		if err != nil {
			log.Warning("Stats server unreachable", "err", err)
			time.Sleep(10 * time.Second)
			continue
		}
		// Authenticate the client with the server
		if err = s.login(conn); err != nil {
			log.Warning("Stats login failed", "err", err)
			conn.Close()
			time.Sleep(10 * time.Second)
			continue
		}
		go s.readLoop(conn)

		// Send the initial stats so our node looks decent from the get go
		if err = s.report(conn); err != nil {
			log.Warning("Initial stats report failed", "err", err)
			conn.Close()
			continue
		}
		// Keep sending status updates until the connection breaks
		fullReport := time.NewTicker(15 * time.Second)

		for err == nil {
			select {
			case <-quitCh:
				conn.Close()
				return

			case <-fullReport.C:
				if err = s.report(conn); err != nil {
					log.Warning("Full stats report failed", "err", err)
				}
			case list := <-s.histCh:
				if err = s.reportHistory(conn, list); err != nil {
					log.Warning("Requested history report failed", "err", err)
				}
			case head := <-headCh:
				if err = s.reportBlock(conn, head); err != nil {
					log.Warning("Block stats report failed", "err", err)
				}
				if err = s.reportPending(conn); err != nil {
					log.Warning("Post-block transaction stats report failed", "err", err)
				}
			case <-txCh:
				if err = s.reportPending(conn); err != nil {
					log.Warning("Transaction stats report failed", "err", err)
				}
			}
		}
		// Make sure the connection is closed
		conn.Close()
	}
}

// readLoop loops as long as the connection is alive and retrieves data packets
// from the network socket. If any of them match an active request, it forwards
// it, if they themselves are requests it initiates a reply, and lastly it drops
// unknown packets.
func (s *Service) readLoop(conn *websocket.Conn) {
	// If the read loop exists, close the connection
	defer conn.Close()

	for {
		// Retrieve the next generic network packet and bail out on error
		var msg map[string][]interface{}
		if err := websocket.JSON.Receive(conn, &msg); err != nil {
			log.Warning("Failed to decode stats server message", "err", err)
			return
		}
		log.Debug("Received message from stats server", "msg", msg)
		if len(msg["emit"]) == 0 {
			log.Warning("Stats server sent non-broadcast", "msg", msg)
			return
		}
		command, ok := msg["emit"][0].(string)
		if !ok {
			log.Warning("Invalid stats server message type", "type", msg["emit"][0])
			return
		}
		// If the message is a ping reply, deliver (someone must be listening!)
		if len(msg["emit"]) == 2 && command == "node-pong" {
			select {
			case s.pongCh <- struct{}{}:
				// Pong delivered, continue listening
				continue
			default:
				// Ping routine dead, abort
				log.Warning("Stats server pinger seems to have died")
				return
			}
		}
		// If the message is a history request, forward to the event processor
		if len(msg["emit"]) == 2 && command == "history" {
			// Make sure the request is valid and doesn't crash us
			request, ok := msg["emit"][1].(map[string]interface{})
			if !ok {
				log.Warning("Invalid stats history request", "msg", msg["emit"][1])
				s.histCh <- nil
				continue // Ethstats sometime sends invalid history requests, ignore those
			}
			list, ok := request["list"].([]interface{})
			if !ok {
				log.Warning("Invalid stats history block list", "list", request["list"])
				return
			}
			// Convert the block number list to an integer list
			numbers := make([]uint64, len(list))
			for i, num := range list {
				n, ok := num.(float64)
				if !ok {
					log.Warning("Invalid stats history block number", "number", num)
					return
				}
				numbers[i] = uint64(n)
			}
			select {
			case s.histCh <- numbers:
				continue
			default:
			}
		}
		// Report anything else and continue
		log.Info("Unknown stats message", "msg", msg)
	}
}

// nodeInfo is the collection of metainformation about a node that is displayed
// on the monitoring page.
type nodeInfo struct {
	Name     string `json:"name"`
	Node     string `json:"node"`
	Port     int    `json:"port"`
	Network  string `json:"net"`
	Protocol string `json:"protocol"`
	API      string `json:"api"`
	Os       string `json:"os"`
	OsVer    string `json:"os_v"`
	Client   string `json:"client"`
	History  bool   `json:"canUpdateHistory"`
}

// authMsg is the authentication infos needed to login to a monitoring server.
type authMsg struct {
	ID     string   `json:"id"`
	Info   nodeInfo `json:"info"`
	Secret string   `json:"secret"`
}

// login tries to authorize the client at the remote server.
func (s *Service) login(conn *websocket.Conn) error {
	// Construct and send the login authentication
	infos := s.server.NodeInfo()

	var network, protocol string
	if info := infos.Protocols["entropy"]; info != nil {
		network = fmt.Sprintf("%d", info.(*entropy.NodeInfo).Network)
		protocol = fmt.Sprintf("entropy/%d", entropy.ProtocolVersions[0])
	} else {
		//network = fmt.Sprintf("%d", infos.Protocols["les"].(*les.NodeInfo).Network)
		//protocol = fmt.Sprintf("les/%d", les.ClientProtocolVersions[0])
	}
	auth := &authMsg{
		ID: s.node,
		Info: nodeInfo{
			Name:     s.node,
			Node:     infos.Name,
			Port:     infos.Ports.Listener,
			Network:  network,
			Protocol: protocol,
			API:      "No",
			Os:       runtime.GOOS,
			OsVer:    runtime.GOARCH,
			Client:   "0.1.1",
			History:  true,
		},
		Secret: s.pass,
	}
	login := map[string][]interface{}{
		"emit": {"hello", auth},
	}
	if err := websocket.JSON.Send(conn, login); err != nil {
		return err
	}
	// Retrieve the remote ack or connection termination
	var ack map[string][]string
	if err := websocket.JSON.Receive(conn, &ack); err != nil || len(ack["emit"]) != 1 || ack["emit"][0] != "ready" {
		return errors.New("unauthorized")
	}
	return nil
}

// report collects all possible data to report and send it to the stats server.
// This should only be used on reconnects or rarely to avoid overloading the
// server. Use the individual methods for reporting subscribed events.
func (s *Service) report(conn *websocket.Conn) error {
	if err := s.reportLatency(conn); err != nil {
		return err
	}
	if err := s.reportBlock(conn, nil); err != nil {
		return err
	}
	if err := s.reportPending(conn); err != nil {
		return err
	}
	if err := s.reportStats(conn); err != nil {
		return err
	}
	return nil
}

// reportLatency sends a ping request to the server, measures the RTT time and
// finally sends a latency update.
func (s *Service) reportLatency(conn *websocket.Conn) error {
	// Send the current time to the ethstats server
	start := time.Now()

	ping := map[string][]interface{}{
		"emit": {"node-ping", map[string]string{
			"id":         s.node,
			"clientTime": start.String(),
		}},
	}
	if err := websocket.JSON.Send(conn, ping); err != nil {
		return err
	}
	// Wait for the pong request to arrive back
	select {
	case <-s.pongCh:
		// Pong delivered, report the latency
	case <-time.After(5 * time.Second):
		// Ping timeout, abort
		return errors.New("ping timed out")
	}
	latency := strconv.Itoa(int((time.Since(start) / time.Duration(2)).Nanoseconds() / 1000000))

	// Send back the measured latency
	log.Debug("Sending measured latency to ethstats", "latency", latency)

	stats := map[string][]interface{}{
		"emit": {"latency", map[string]string{
			"id":      s.node,
			"latency": latency,
		}},
	}
	return websocket.JSON.Send(conn, stats)
}

// blockStats is the information to report about individual blocks.
type blockStats struct {
	Number     *big.Int       `json:"number"`
	Hash       common.Hash    `json:"hash"`
	ParentHash common.Hash    `json:"parentHash"`
	Timestamp  *big.Int       `json:"timestamp"`
	Miner      common.Address `json:"miner"`
	GasUsed    uint64         `json:"gasUsed"`
	GasLimit   uint64         `json:"gasLimit"`
	Diff       string         `json:"difficulty"`
	TotalDiff  string         `json:"totalDifficulty"`
	Txs        []txStats      `json:"transactions"`
	TxHash     common.Hash    `json:"transactionsRoot"`
	Root       common.Hash    `json:"stateRoot"`
	Uncles     uncleStats     `json:"uncles"`
}

// txStats is the information to report about individual transactions.
type txStats struct {
	Hash common.Hash `json:"hash"`
}

// uncleStats is a custom wrapper around an uncle array to force serializing
// empty arrays instead of returning null for them.
type uncleStats []*model.Header

func (s uncleStats) MarshalJSON() ([]byte, error) {
	if uncles := ([]*model.Header)(s); len(uncles) > 0 {
		return json.Marshal(uncles)
	}
	return []byte("[]"), nil
}

// reportBlock retrieves the current chain head and reports it to the stats server.
func (s *Service) reportBlock(conn *websocket.Conn, block *model.Block) error {
	// Gather the block details from the header or block chain
	details := s.assembleBlockStats(block)

	// Assemble the block report and send it to the server
	log.Debug("Sending new block to ethstats", "number", details.Number, "hash", details.Hash)

	stats := map[string]interface{}{
		"id":    s.node,
		"block": details,
	}
	report := map[string][]interface{}{
		"emit": {"block", stats},
	}
	return websocket.JSON.Send(conn, report)
}

// assembleBlockStats retrieves any required metadata to report a single block
// and assembles the block stats. If block is nil, the current head is processed.
func (s *Service) assembleBlockStats(block *model.Block) *blockStats {
	// Gather the block infos from the local blockchain
	var (
		header *model.Header
		td     *big.Int
		txs    []txStats
		uncles []*model.Header
	)
	if s.entropy != nil {
		// Full nodes have all needed information available
		if block == nil {
			block = s.entropy.BlockChain().CurrentBlock()
		}
		header = block.Header()
		td = s.entropy.BlockChain().GetTd(header.Hash(), header.Number.Uint64())

		txs = make([]txStats, len(block.Transactions()))
		for i, tx := range block.Transactions() {
			txs[i].Hash = tx.Hash()
		}
		uncles = block.Uncles()
	} else {
		// Light nodes would need on-demand lookups for transactions/uncles, skip
		if block != nil {
			header = block.Header()
		} else {
			//header = s.les.BlockChain().CurrentHeader()
		}
		//td = s.les.BlockChain().GetTd(header.Hash(), header.Number.Uint64())
		txs = []txStats{}
	}
	// Assemble and return the block stats
	author, _ := s.engine.Author(header)

	return &blockStats{
		Number:     header.Number,
		Hash:       header.Hash(),
		ParentHash: header.ParentHash,
		Timestamp:  header.Time,
		Miner:      author,
		GasUsed:    header.GasUsed,
		GasLimit:   header.GasLimit,
		Diff:       header.Difficulty.String(),
		TotalDiff:  td.String(),
		Txs:        txs,
		TxHash:     header.TxHash,
		Root:       header.Root,
		Uncles:     uncles,
	}
}

// reportHistory retrieves the most recent batch of blocks and reports it to the
// stats server.
func (s *Service) reportHistory(conn *websocket.Conn, list []uint64) error {
	// Figure out the indexes that need reporting
	indexes := make([]uint64, 0, historyUpdateRange)
	if len(list) > 0 {
		// Specific indexes requested, send them back in particular
		indexes = append(indexes, list...)
	} else {
		// No indexes requested, send back the top ones
		var head int64
		if s.entropy != nil {
			head = s.entropy.BlockChain().CurrentHeader().Number.Int64()
		} else {
			//head = s.les.BlockChain().CurrentHeader().Number.Int64()
		}
		start := head - historyUpdateRange + 1
		if start < 0 {
			start = 0
		}
		for i := uint64(start); i <= uint64(head); i++ {
			indexes = append(indexes, i)
		}
	}
	// Gather the batch of blocks to report
	history := make([]*blockStats, len(indexes))
	for i, number := range indexes {
		// Retrieve the next block if it's known to us
		var block *model.Block
		if s.entropy != nil {
			block = s.entropy.BlockChain().GetBlockByNumber(number)
		} else {
			//if header := s.les.BlockChain().GetHeaderByNumber(number); header != nil {
			//	block = model.NewBlockWithHeader(header)
			//}
		}
		// If we do have the block, add to the history and continue
		if block != nil {
			history[len(history)-1-i] = s.assembleBlockStats(block)
			continue
		}
		// Ran out of blocks, cut the report short and send
		history = history[len(history)-i:]
		break
	}
	// Assemble the history report and send it to the server
	if len(history) > 0 {
		log.Debug("Sending historical blocks to ethstats", "first", history[0].Number, "last", history[len(history)-1].Number)
	} else {
		log.Debug("No history to send to stats server")
	}
	stats := map[string]interface{}{
		"id":      s.node,
		"history": history,
	}
	report := map[string][]interface{}{
		"emit": {"history", stats},
	}
	return websocket.JSON.Send(conn, report)
}

// pendStats is the information to report about pending transactions.
type pendStats struct {
	Pending int `json:"pending"`
}

// reportPending retrieves the current number of pending transactions and reports
// it to the stats server.
func (s *Service) reportPending(conn *websocket.Conn) error {
	// Retrieve the pending count from the local blockchain
	var pending int
	if s.entropy != nil {
		pending, _ = s.entropy.TxPool().Stats()
	} else {
		//pending = s.les.TxPool().Stats()
	}
	// Assemble the transaction stats and send it to the server
	log.Debug("Sending pending transactions to ethstats", "count", pending)

	stats := map[string]interface{}{
		"id": s.node,
		"stats": &pendStats{
			Pending: pending,
		},
	}
	report := map[string][]interface{}{
		"emit": {"pending", stats},
	}
	return websocket.JSON.Send(conn, report)
}

// nodeStats is the information to report about the local node.
type nodeStats struct {
	Active   bool `json:"active"`
	Syncing  bool `json:"syncing"`
	Mining   bool `json:"mining"`
	Hashrate int  `json:"hashrate"`
	Peers    int  `json:"peers"`
	GasPrice int  `json:"gasPrice"`
	Uptime   int  `json:"uptime"`
}

// reportPending retrieves various stats about the node at the networking and
// mining layer and reports it to the stats server.
func (s *Service) reportStats(conn *websocket.Conn) error {
	// Gather the syncing and mining infos from the local miner instance
	var (
		mining   bool
		hashrate int
		syncing  bool
		gasprice int
	)
	if s.entropy != nil {
		mining = s.entropy.Miner().Mining()
		hashrate = int(s.entropy.Miner().HashRate())

		sync := s.entropy.Downloader().Progress()
		syncing = s.entropy.BlockChain().CurrentHeader().Number.Uint64() >= sync.HighestBlock

		price, _ := s.entropy.APIBackend.SuggestPrice(context.Background())
		gasprice = int(price.Uint64())
	} else {
		//sync := s.les.Downloader().Progress()
		//syncing = s.les.BlockChain().CurrentHeader().Number.Uint64() >= sync.HighestBlock
	}
	// Assemble the node stats and send it to the server
	log.Debug("Sending node details to ethstats")

	stats := map[string]interface{}{
		"id": s.node,
		"stats": &nodeStats{
			Active:   true,
			Mining:   mining,
			Hashrate: hashrate,
			Peers:    s.server.PeerCount(),
			GasPrice: gasprice,
			Syncing:  syncing,
			Uptime:   100,
		},
	}
	report := map[string][]interface{}{
		"emit": {"stats", stats},
	}
	return websocket.JSON.Send(conn, report)
}
