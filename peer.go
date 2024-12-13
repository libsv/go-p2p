package p2p

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ordishs/go-utils"
	"github.com/ordishs/go-utils/batcher"

	"github.com/libsv/go-p2p/bsvutil"
	"github.com/libsv/go-p2p/chaincfg/chainhash"
	"github.com/libsv/go-p2p/wire"
)

const (
	defaultMaximumMessageSize     = 32 * 1024 * 1024
	defaultBatchDelayMilliseconds = 200

	commandKey = "cmd"
	hashKey    = "hash"
	errKey     = "err"
	typeKey    = "type"

	sentMsg     = "Sent"
	receivedMsg = "Recv"

	nrWriteHandlersDefault               = 10
	retryReadWriteMessageIntervalDefault = 1 * time.Second
	retryReadWriteMessageAttempts        = 5
	reconnectInterval                    = 10 * time.Second

	pingIntervalDefault                   = 2 * time.Minute
	connectionHealthTickerDurationDefault = 3 * time.Minute
	LevelTrace                            = slog.LevelDebug - 4
)

type Block struct {
	Hash         *chainhash.Hash `json:"hash,omitempty"`          // Little endian
	PreviousHash *chainhash.Hash `json:"previous_hash,omitempty"` // Little endian
	MerkleRoot   *chainhash.Hash `json:"merkle_root,omitempty"`   // Little endian
	Height       uint64          `json:"height,omitempty"`
	Size         uint64          `json:"size,omitempty"`
	TxCount      uint64          `json:"tx_count,omitempty"`
}

type Peer struct {
	address                       string
	network                       wire.BitcoinNet
	mu                            sync.RWMutex
	readConn                      net.Conn
	writeConn                     net.Conn
	incomingConn                  net.Conn
	dial                          func(network, address string) (net.Conn, error)
	peerHandler                   PeerHandlerI
	writeChan                     chan wire.Message
	pingPongAlive                 chan struct{}
	logger                        *slog.Logger
	sentVerAck                    atomic.Bool
	receivedVerAck                atomic.Bool
	batchDelay                    time.Duration
	invBatcher                    *batcher.Batcher[chainhash.Hash]
	dataBatcher                   *batcher.Batcher[chainhash.Hash]
	maximumMessageSize            int64
	isHealthy                     atomic.Bool
	userAgentName                 *string
	userAgentVersion              *string
	retryReadWriteMessageInterval time.Duration
	nrWriteHandlers               int
	isUnhealthyCh                 chan struct{}
	pingInterval                  time.Duration
	connectionHealthThreshold     time.Duration
	ctx                           context.Context

	cancelReadHandler  context.CancelFunc
	cancelWriteHandler context.CancelFunc
	cancelAll          context.CancelFunc

	readerWg        *sync.WaitGroup
	writerWg        *sync.WaitGroup
	reconnectingWg  *sync.WaitGroup
	healthMonitorWg *sync.WaitGroup

	buffSize int
}

// NewPeer returns a new bitcoin peer for the provided address and configuration.
func NewPeer(logger *slog.Logger, address string, peerHandler PeerHandlerI, network wire.BitcoinNet, options ...PeerOptions) (*Peer, error) {
	writeChan := make(chan wire.Message, 10000)

	peerLogger := logger.With(
		slog.Group("peer",
			slog.String("network", network.String()),
			slog.String("address", address),
		),
	)

	p := &Peer{
		network:                       network,
		address:                       address,
		writeChan:                     writeChan,
		pingPongAlive:                 make(chan struct{}, 1),
		isUnhealthyCh:                 make(chan struct{}),
		peerHandler:                   peerHandler,
		logger:                        peerLogger,
		dial:                          net.Dial,
		nrWriteHandlers:               nrWriteHandlersDefault,
		maximumMessageSize:            defaultMaximumMessageSize,
		batchDelay:                    defaultBatchDelayMilliseconds * time.Millisecond,
		retryReadWriteMessageInterval: retryReadWriteMessageIntervalDefault,
		pingInterval:                  pingIntervalDefault,
		connectionHealthThreshold:     connectionHealthTickerDurationDefault,
		writerWg:                      &sync.WaitGroup{},
		readerWg:                      &sync.WaitGroup{},
		reconnectingWg:                &sync.WaitGroup{},
		healthMonitorWg:               &sync.WaitGroup{},
		buffSize:                      4096,
	}

	var err error
	for _, option := range options {
		err = option(p)
		if err != nil {
			return nil, fmt.Errorf("failed to apply option, %v", err)
		}
	}

	p.start()

	return p, nil
}

func (p *Peer) start() {

	p.logger.Info("Starting peer")

	ctx, cancelAll := context.WithCancel(context.Background())
	p.cancelAll = cancelAll
	p.ctx = ctx

	p.startMonitorPingPong()

	p.invBatcher = batcher.New(500, p.batchDelay, p.sendInvBatch, true)
	p.dataBatcher = batcher.New(500, p.batchDelay, p.sendDataBatch, true)

	if p.incomingConn != nil {
		go func() {
			err := p.connectAndStartReadWriteHandlers()
			if err != nil {
				p.logger.Warn("Failed to connect to peer", slog.String(errKey, err.Error()))
			}
		}()
		p.logger.Info("Incoming connection from peer")
		return
	}

	// reconnect if disconnected, but only on outgoing connections
	p.reconnect()
}

func (p *Peer) disconnectLock() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.disconnect()
}

func (p *Peer) reconnect() {
	p.reconnectingWg.Add(1)

	go func() {
		defer func() {
			p.reconnectingWg.Done()
		}()
		connectErr := p.connectAndStartReadWriteHandlers()
		if connectErr != nil {
			p.logger.Warn("Failed to connect to peer", slog.String(errKey, connectErr.Error()))
		}

		ticker := time.NewTicker(reconnectInterval)
		for {
			select {
			case <-ticker.C:
				if p.Connected() || p.Connecting() {
					continue
				}
				p.logger.Info("Reconnecting")

				connectErr = p.connectAndStartReadWriteHandlers()
				if connectErr != nil {
					p.logger.Warn("Failed to connect to peer", slog.String(errKey, connectErr.Error()))
					continue
				}
			case <-p.ctx.Done():
				return
			}
		}
	}()
}

func (p *Peer) disconnect() {
	if p.readConn != nil {
		_ = p.readConn.Close()
	}

	p.readConn = nil
	p.writeConn = nil
	p.sentVerAck.Store(false)
	p.receivedVerAck.Store(false)
}

func (p *Peer) connectAndStartReadWriteHandlers() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.incomingConn == nil {
		if p.readConn != nil || p.writeConn != nil {
			p.disconnect()
		}
		p.readConn = nil
	}

	p.sentVerAck.Store(false)
	p.receivedVerAck.Store(false)

	if p.incomingConn != nil {
		p.readConn = p.incomingConn
	} else {
		p.logger.Info("Connecting")
		conn, err := p.dial("tcp", p.address)
		if err != nil {
			return fmt.Errorf("could not dial node: %v", err)
		}

		// open the read connection, so we can receive messages
		p.readConn = conn
	}

	writerCtx, cancelWriter := context.WithCancel(p.ctx)
	p.cancelWriteHandler = cancelWriter
	for i := 0; i < p.nrWriteHandlers; i++ {
		// start 10 workers that will write to the peer
		// locking is done in the net.write in the wire/message handler
		// this reduces the wait on the writer when processing writes (for example HandleTransactionSent)
		p.startWriteChannelHandler(writerCtx, i+1)
	}

	readerCtx, cancelReader := context.WithCancel(p.ctx)
	p.cancelReadHandler = cancelReader
	p.startReadHandler(readerCtx)

	// write version message to our peer directly and not through the write channel,
	// write channel is not ready to send message until the VERACK handshake is done
	msg := p.versionMessage(p.address)

	// here we can write to the readConn, since we are in the process of connecting and this is the
	// only one that is already open. Opening the writeConn signals that we are done with the handshake
	if err := wire.WriteMessage(p.readConn, msg, wire.ProtocolVersion, p.network); err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}
	p.logger.Log(p.ctx, LevelTrace, sentMsg, slog.String(commandKey, strings.ToUpper(msg.Command())))

	startWaitTime := time.Now()
	for {
		if p.receivedVerAck.Load() && p.sentVerAck.Load() {
			break
		}
		// wait for maximum 30 seconds
		if time.Since(startWaitTime) > 30*time.Second {
			return fmt.Errorf("timeout waiting for VERACK")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// set the connection which allows us to send messages
	p.writeConn = p.readConn

	p.logger.Info("Connection established")

	return nil
}

func (p *Peer) Network() wire.BitcoinNet {
	return p.network
}

func (p *Peer) Connected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.readConn != nil && p.writeConn != nil
}

func (p *Peer) Connecting() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.readConn != nil && p.writeConn == nil
}

func (p *Peer) WriteMsg(msg wire.Message) error {
	utils.SafeSend(p.writeChan, msg)
	return nil
}

func (p *Peer) String() string {
	return p.address
}

type readMessageResult struct {
	msg wire.Message
	err error
}

func (p *Peer) readMessage(ctx context.Context, r io.Reader, pver uint32, bsvnet wire.BitcoinNet) (wire.Message, error) {
	readMessageFinished := make(chan readMessageResult, 1)

	go func() {
		msg, _, err := wire.ReadMessage(r, pver, bsvnet)
		readMessageFinished <- readMessageResult{msg, err}
	}()

	// ensure read message doesn't block
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case readMsg := <-readMessageFinished:
		if readMsg.err != nil {
			return nil, readMsg.err
		}
		return readMsg.msg, nil
	}
}

func (p *Peer) readRetry(ctx context.Context, r io.Reader, pver uint32, bsvnet wire.BitcoinNet) (wire.Message, error) {
	msg, err := p.readMessage(ctx, r, pver, bsvnet)
	if err == nil {
		return msg, nil
	}

	if errors.Is(err, context.Canceled) {
		return nil, err
	} else if errors.Is(err, io.EOF) {
		p.logger.Error("Failed to read message: EOF", slog.String(errKey, err.Error()))
	} else {
		p.logger.Error("Failed to read message", slog.String(errKey, err.Error()))
	}

	counter := 0
	ticker := time.NewTicker(p.retryReadWriteMessageInterval)

	for {
		counter++
		if counter >= retryReadWriteMessageAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			msg, err = p.readMessage(ctx, r, pver, bsvnet)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return nil, err
				}
				if errors.Is(err, io.EOF) {
					p.logger.Error("Failed to read message: EOF", slog.String(errKey, err.Error()))
					continue
				}

				p.logger.Error("Failed to read message", slog.String(errKey, err.Error()))
				continue
			}

			return msg, nil
		}
	}

	return nil, err
}

func (p *Peer) startReadHandler(ctx context.Context) {
	p.readerWg.Add(1)

	go func() {
		p.logger.Debug("Starting read handler")

		defer func() {
			p.logger.Debug("Shutting down read handler")
			p.readerWg.Done()
		}()

		readConn := p.readConn
		var msg wire.Message
		var err error

		if readConn == nil {
			p.logger.Error("no connection")
			return
		}

		reader := bufio.NewReaderSize(&io.LimitedReader{R: readConn, N: p.maximumMessageSize}, p.buffSize)
		for {
			select {
			case <-ctx.Done():
				p.logger.Debug("Read handler canceled")
				return
			default:
				msg, err = p.readRetry(ctx, reader, wire.ProtocolVersion, p.network)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						p.logger.Debug("Retrying to read canceled")
						return
					}

					p.logger.Error("Retrying to read failed", slog.String(errKey, err.Error()))

					// stop all read and write handlers
					p.cancelWriteHandler()
					p.cancelReadHandler()

					p.disconnectLock()

					return
				}

				commandLogger := p.logger.With(slog.String(commandKey, strings.ToUpper(msg.Command())))

				// we could check this based on type (switch msg.(type)) but that would not allow
				// us to override the default behaviour for a specific message type
				switch msg.Command() {
				case wire.CmdVersion:
					commandLogger.Debug(receivedMsg)
					if p.sentVerAck.Load() {
						commandLogger.Warn("Received version message after sending verack")
						continue
					}

					verackMsg := wire.NewMsgVerAck()
					if err = wire.WriteMessage(readConn, verackMsg, wire.ProtocolVersion, p.network); err != nil {
						commandLogger.Error("failed to write message", slog.String(errKey, err.Error()))
					}
					commandLogger.Debug(sentMsg, slog.String(commandKey, strings.ToUpper(verackMsg.Command())))
					p.sentVerAck.Store(true)

				case wire.CmdPing:
					commandLogger.Debug(receivedMsg, slog.String(commandKey, strings.ToUpper(wire.CmdPing)))
					p.pingPongAlive <- struct{}{}

					pingMsg, ok := msg.(*wire.MsgPing)
					if !ok {
						continue
					}
					p.writeChan <- wire.NewMsgPong(pingMsg.Nonce)

				case wire.CmdInv:
					invMsg, ok := msg.(*wire.MsgInv)
					if !ok {
						continue
					}
					for _, inv := range invMsg.InvList {
						commandLogger.Debug(receivedMsg, slog.String(hashKey, inv.Hash.String()), slog.String(typeKey, inv.Type.String()))
					}

					go func(invList []*wire.InvVect, routineLogger *slog.Logger) {
						for _, invVect := range invList {
							switch invVect.Type {
							case wire.InvTypeTx:
								if err = p.peerHandler.HandleTransactionAnnouncement(invVect, p); err != nil {
									commandLogger.Error("Unable to process tx", slog.String(hashKey, invVect.Hash.String()), slog.String(typeKey, invVect.Type.String()), slog.String(errKey, err.Error()))
								}
							case wire.InvTypeBlock:
								if err = p.peerHandler.HandleBlockAnnouncement(invVect, p); err != nil {
									commandLogger.Error("Unable to process block", slog.String(hashKey, invVect.Hash.String()), slog.String(typeKey, invVect.Type.String()), slog.String(errKey, err.Error()))
								}
							}
						}
					}(invMsg.InvList, commandLogger)

				case wire.CmdGetData:
					dataMsg, ok := msg.(*wire.MsgGetData)
					if !ok {
						continue
					}
					for _, inv := range dataMsg.InvList {
						commandLogger.Debug(receivedMsg, slog.String(hashKey, inv.Hash.String()), slog.String(typeKey, inv.Type.String()))
					}
					p.handleGetDataMsg(dataMsg, commandLogger)

				case wire.CmdTx:
					txMsg, ok := msg.(*wire.MsgTx)
					if !ok {
						continue
					}
					commandLogger.Debug(receivedMsg, slog.String(hashKey, txMsg.TxHash().String()), slog.Int("size", txMsg.SerializeSize()))
					if err = p.peerHandler.HandleTransaction(txMsg, p); err != nil {
						commandLogger.Error("Unable to process tx", slog.String(hashKey, txMsg.TxHash().String()), slog.String(errKey, err.Error()))
					}

				case wire.CmdBlock:
					msgBlock, ok := msg.(*wire.MsgBlock)
					if ok {
						commandLogger.Info(receivedMsg, slog.String(hashKey, msgBlock.Header.BlockHash().String()))

						err = p.peerHandler.HandleBlock(msgBlock, p)
						if err != nil {
							commandLogger.Error("Unable to process block", slog.String(hashKey, msgBlock.Header.BlockHash().String()), slog.String(errKey, err.Error()))
						}
						continue
					}

					// Please note that this is the BlockMessage, not the wire.MsgBlock
					blockMsg, ok := msg.(*BlockMessage)
					if !ok {
						commandLogger.Error("Unable to cast block message, calling with generic wire.Message")
						err = p.peerHandler.HandleBlock(msg, p)
						if err != nil {
							commandLogger.Error("Unable to process block message", slog.String(errKey, err.Error()))
						}
						continue
					}

					commandLogger.Info(receivedMsg, slog.String(hashKey, blockMsg.Header.BlockHash().String()))

					err = p.peerHandler.HandleBlock(blockMsg, p)
					if err != nil {
						commandLogger.Error("Unable to process block", slog.String(hashKey, blockMsg.Header.BlockHash().String()), slog.String(errKey, err.Error()))
					}

				case wire.CmdReject:
					rejMsg, ok := msg.(*wire.MsgReject)
					if !ok {
						continue
					}
					if err = p.peerHandler.HandleTransactionRejection(rejMsg, p); err != nil {
						commandLogger.Error("Unable to process block", slog.String(hashKey, rejMsg.Hash.String()), slog.String(errKey, err.Error()))
					}

				case wire.CmdVerAck:
					commandLogger.Debug(receivedMsg)
					p.receivedVerAck.Store(true)

				case wire.CmdPong:
					commandLogger.Debug(receivedMsg, slog.String(commandKey, strings.ToUpper(wire.CmdPong)))
					p.pingPongAlive <- struct{}{}

				default:
					commandLogger.Debug("command ignored")
				}
			}
		}
	}()
}

func (p *Peer) handleGetDataMsg(dataMsg *wire.MsgGetData, logger *slog.Logger) {
	txRequests := make([]*wire.InvVect, 0)

	for _, invVect := range dataMsg.InvList {
		switch invVect.Type {
		case wire.InvTypeTx:
			logger.Debug("Request for TX", slog.String(hashKey, invVect.Hash.String()))
			txRequests = append(txRequests, invVect)

		case wire.InvTypeBlock:
			logger.Info("Request for block", slog.String(hashKey, invVect.Hash.String()), slog.String(typeKey, invVect.Type.String()))
			continue

		default:
			logger.Warn("Unknown type", slog.String(hashKey, invVect.Hash.String()), slog.String(typeKey, invVect.Type.String()))
			continue
		}
	}

	rawTxs, err := p.peerHandler.HandleTransactionsGet(txRequests, p)
	if err != nil {
		logger.Warn("Unable to fetch txs from store", slog.Int("count", len(txRequests)), slog.String(errKey, err.Error()))
		// there is no return here because peerHandler.HandleTransactionsGet() may return
		// already found rawTxs together with an error, so we want to process them
	}

	for _, txBytes := range rawTxs {
		if txBytes == nil {
			logger.Warn("tx does not exist")
			continue
		}

		tx, err := bsvutil.NewTxFromBytes(txBytes)
		if err != nil {
			logger.Error("failed to parse tx", slog.String("rawHex", hex.EncodeToString(txBytes)), slog.String(errKey, err.Error()))
			continue
		}

		p.writeChan <- tx.MsgTx()
	}
}

func (p *Peer) AnnounceTransaction(hash *chainhash.Hash) {
	p.invBatcher.Put(hash)
}

func (p *Peer) RequestTransaction(hash *chainhash.Hash) {
	p.dataBatcher.Put(hash)
}

func (p *Peer) AnnounceBlock(blockHash *chainhash.Hash) {
	invMsg := wire.NewMsgInv()

	iv := wire.NewInvVect(wire.InvTypeBlock, blockHash)
	if err := invMsg.AddInvVect(iv); err != nil {
		p.logger.Error("failed to add invVect to INV message", slog.String(typeKey, iv.Type.String()), slog.String(hashKey, blockHash.String()), slog.String(errKey, err.Error()))
		return
	}

	if err := p.WriteMsg(invMsg); err != nil {
		p.logger.Error("failed to send INV message", slog.String(typeKey, iv.Type.String()), slog.String(hashKey, blockHash.String()), slog.String(errKey, err.Error()))
	} else {
		p.logger.Info("Sent INV", slog.String(typeKey, iv.Type.String()), slog.String(hashKey, blockHash.String()))
	}
}

func (p *Peer) RequestBlock(blockHash *chainhash.Hash) {
	dataMsg := wire.NewMsgGetData()

	iv := wire.NewInvVect(wire.InvTypeBlock, blockHash)
	if err := dataMsg.AddInvVect(iv); err != nil {
		p.logger.Error("failed to add invVect to GETDATA message", slog.String(typeKey, iv.Type.String()), slog.String(hashKey, blockHash.String()), slog.String(errKey, err.Error()))
		return
	}

	if err := p.WriteMsg(dataMsg); err != nil {
		p.logger.Error("failed to send GETDATA message", slog.String(hashKey, blockHash.String()), slog.String(typeKey, iv.Type.String()), slog.String(errKey, err.Error()))
	} else {
		p.logger.Log(p.ctx, LevelTrace, "Sent GETDATA", slog.String(hashKey, blockHash.String()), slog.String(typeKey, iv.Type.String()))
	}
}

func (p *Peer) sendInvBatch(batch []*chainhash.Hash) {
	invMsg := wire.NewMsgInvSizeHint(uint(len(batch)))

	for _, hash := range batch {
		iv := wire.NewInvVect(wire.InvTypeTx, hash)
		_ = invMsg.AddInvVect(iv)
		p.logger.Log(p.ctx, LevelTrace, "Sent INV", slog.String(hashKey, hash.String()), slog.String(typeKey, wire.InvTypeTx.String()))
	}

	p.writeChan <- invMsg
}

func (p *Peer) sendDataBatch(batch []*chainhash.Hash) {
	dataMsg := wire.NewMsgGetData()

	for _, hash := range batch {
		iv := wire.NewInvVect(wire.InvTypeTx, hash)
		_ = dataMsg.AddInvVect(iv)
		p.logger.Log(p.ctx, LevelTrace, "Sent GETDATA", slog.String(hashKey, hash.String()), slog.String(typeKey, wire.InvTypeTx.String()))
	}

	if err := p.WriteMsg(dataMsg); err != nil {
		p.logger.Error("failed to send tx data message", slog.String(errKey, err.Error()))
	} else {
		p.logger.Log(p.ctx, LevelTrace, "Sent GETDATA", slog.Int("items", len(batch)))
	}
}

func (p *Peer) writeRetry(ctx context.Context, msg wire.Message) error {
	policy := backoff.WithMaxRetries(backoff.NewConstantBackOff(p.retryReadWriteMessageInterval), retryReadWriteMessageAttempts)

	policyContext := backoff.WithContext(policy, ctx)

	operation := func() error {
		return wire.WriteMessage(p.writeConn, msg, wire.ProtocolVersion, p.network)
	}

	notify := func(err error, nextTry time.Duration) {
		p.logger.Error("Failed to write message", slog.Duration("next try", nextTry), slog.String(errKey, err.Error()))
	}

	return backoff.RetryNotify(operation, policyContext, notify)
}

func (p *Peer) startWriteChannelHandler(ctx context.Context, instance int) {
	p.writerWg.Add(1)
	go func() {
		p.logger.Debug("Starting write handler", slog.Int("instance", instance))

		defer func() {
			p.logger.Debug("Shutting down write handler", slog.Int("instance", instance))
			p.writerWg.Done()
		}()

		for {
			select {
			case <-ctx.Done():
				p.logger.Debug("Write handler canceled", slog.Int("instance", instance))
				return
			case msg := <-p.writeChan:
				p.mu.RLock()
				writeConn := p.writeConn
				p.mu.RUnlock()

				if writeConn == nil {
					time.Sleep(100 * time.Millisecond)
					continue
				}

				err := p.writeRetry(ctx, msg)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						p.logger.Debug("Retrying to write canceled", slog.Int("instance", instance))
						return
					}

					p.logger.Error("Failed retrying to write message", slog.Int("instance", instance), slog.String(errKey, err.Error()))

					// stop all read and write handlers
					p.cancelWriteHandler()
					p.cancelReadHandler()

					p.disconnectLock()

					return
				}

				go func(message wire.Message) {
					if message.Command() == wire.CmdTx {
						msgTx, ok := message.(*wire.MsgTx)
						if !ok {
							return
						}
						hash := msgTx.TxHash()
						if err := p.peerHandler.HandleTransactionSent(msgTx, p); err != nil {
							p.logger.Error("Unable to process tx", slog.Int("instance", instance), slog.String(hashKey, hash.String()), slog.String(errKey, err.Error()))
						}
					}

					switch m := message.(type) {
					case *wire.MsgTx:
						p.logger.Log(p.ctx, LevelTrace, sentMsg, slog.String(commandKey, strings.ToUpper(message.Command())), slog.String(hashKey, m.TxHash().String()), slog.String(typeKey, "tx"))
					case *wire.MsgBlock:
						p.logger.Log(p.ctx, LevelTrace, sentMsg, slog.String(commandKey, strings.ToUpper(message.Command())), slog.String(hashKey, m.BlockHash().String()), slog.String(typeKey, "block"))
					case *wire.MsgGetData:
						p.logger.Log(p.ctx, LevelTrace, sentMsg, slog.String(commandKey, strings.ToUpper(message.Command())), slog.String(hashKey, m.InvList[0].Hash.String()), slog.String(typeKey, "getdata"))
					case *wire.MsgInv:
						p.logger.Log(p.ctx, LevelTrace, sentMsg, slog.String(commandKey, strings.ToUpper(message.Command())), slog.String(hashKey, m.InvList[0].Hash.String()), slog.String(typeKey, "inv"))
					default:
						p.logger.Log(p.ctx, LevelTrace, sentMsg, slog.String(commandKey, strings.ToUpper(message.Command())), slog.String(typeKey, "unknown"))
					}
				}(msg)
			}
		}
	}()
}

func (p *Peer) versionMessage(address string) *wire.MsgVersion {
	lastBlock := int32(0)

	tcpAddrMe := &net.TCPAddr{IP: nil, Port: 0}
	me := wire.NewNetAddress(tcpAddrMe, wire.SFNodeNetwork)

	parts := strings.Split(address, ":")
	if len(parts) != 2 {
		panic(fmt.Sprintf("Could not parse address %s", address))
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		panic(fmt.Sprintf("Could not parse port %s", parts[1]))
	}

	tcpAddrYou := &net.TCPAddr{IP: net.ParseIP(parts[0]), Port: port}
	you := wire.NewNetAddress(tcpAddrYou, wire.SFNodeNetwork)

	nonce, err := wire.RandomUint64()
	if err != nil {
		p.logger.Error("RandomUint64: failed to generate nonce", slog.String(errKey, err.Error()))
	}

	msg := wire.NewMsgVersion(me, you, nonce, lastBlock)

	if p.userAgentName != nil && p.userAgentVersion != nil {
		err = msg.AddUserAgent(*p.userAgentName, *p.userAgentVersion)
		if err != nil {
			p.logger.Error("Failed to add user agent", slog.String(errKey, err.Error()))
		}
	}

	return msg
}

func (p *Peer) startMonitorPingPong() {
	p.healthMonitorWg.Add(1)

	pingTicker := time.NewTicker(p.pingInterval)

	go func() {
		// if no ping/pong signal is received for certain amount of time, mark peer as unhealthy
		monitorConnectionTicker := time.NewTicker(p.connectionHealthThreshold)

		defer func() {
			p.healthMonitorWg.Done()
			monitorConnectionTicker.Stop()
		}()

		for {
			select {
			case <-pingTicker.C:
				nonce, err := wire.RandomUint64()
				if err != nil {
					p.logger.Error("Failed to create random nonce - not sending ping", slog.String(errKey, err.Error()))
					continue
				}
				p.writeChan <- wire.NewMsgPing(nonce)
			case <-p.pingPongAlive:
				// if ping/pong signal is received reset the ticker
				monitorConnectionTicker.Reset(p.connectionHealthThreshold)
				p.setHealthy()
			case <-monitorConnectionTicker.C:

				p.isHealthy.Store(false)

				select {
				case p.isUnhealthyCh <- struct{}{}:
				default: // Do not block if nothing is reading from channel
				}

				p.logger.Warn("peer unhealthy")
			case <-p.ctx.Done():
				return
			}
		}
	}()
}

func (p *Peer) IsUnhealthyCh() <-chan struct{} {
	return p.isUnhealthyCh
}

func (p *Peer) setHealthy() {

	if p.isHealthy.Load() {
		return
	}

	p.logger.Info("peer healthy")
	p.isHealthy.Store(true)
}

func (p *Peer) IsHealthy() bool {
	return p.isHealthy.Load()
}

func (p *Peer) Restart() {
	p.Shutdown()

	p.start()
}

func (p *Peer) Shutdown() {
	p.logger.Info("Shutting down")

	p.cancelAll()

	p.reconnectingWg.Wait()
	p.healthMonitorWg.Wait()
	p.writerWg.Wait()
	p.readerWg.Wait()

	p.logger.Info("Shutdown complete")
}
