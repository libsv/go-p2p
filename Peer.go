package p2p

import (
	"bufio"
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

	"github.com/libsv/go-p2p/bsvutil"
	"github.com/libsv/go-p2p/chaincfg/chainhash"
	"github.com/libsv/go-p2p/wire"
	"github.com/ordishs/go-utils"
	"github.com/ordishs/go-utils/batcher"
)

var (
	pingInterval = 2 * time.Minute
)

const (
	defaultMaximumMessageSize     = 32 * 1024 * 1024
	defaultBatchDelayMilliseconds = 200
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
	address            string
	network            wire.BitcoinNet
	mu                 sync.RWMutex
	readConn           net.Conn
	writeConn          net.Conn
	incomingConn       net.Conn
	dial               func(network, address string) (net.Conn, error)
	peerHandler        PeerHandlerI
	writeChan          chan wire.Message
	quit               chan struct{}
	logger             *slog.Logger
	sentVerAck         atomic.Bool
	receivedVerAck     atomic.Bool
	batchDelay         time.Duration
	invBatcher         *batcher.Batcher[chainhash.Hash]
	dataBatcher        *batcher.Batcher[chainhash.Hash]
	maximumMessageSize int64
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
		network:            network,
		address:            address,
		writeChan:          writeChan,
		peerHandler:        peerHandler,
		logger:             peerLogger,
		dial:               net.Dial,
		maximumMessageSize: defaultMaximumMessageSize,
		batchDelay:         defaultBatchDelayMilliseconds * time.Millisecond,
	}

	for _, option := range options {
		option(p)
	}

	p.initialize()

	return p, nil
}

func (p *Peer) initialize() {

	go p.pingHandler()
	for i := 0; i < 10; i++ {
		// start 10 workers that will write to the peer
		// locking is done in the net.write in the wire/message handler
		// this reduces the wait on the writer when processing writes (for example HandleTransactionSent)
		go p.writeChannelHandler()
	}

	go func() {
		err := p.connect()
		if err != nil {
			p.logger.Warn("Failed to connect to peer", slog.String("err", err.Error()))
		}
	}()

	if p.incomingConn != nil {
		p.logger.Info("Incoming connection from peer")
	} else {
		// reconnect if disconnected, but only on outgoing connections
		go func() {
			for range time.NewTicker(10 * time.Second).C {
				// logger.Debug("cing connection to peer %s, connected = %t, connecting = %t", address, p.Connected(), p.Connecting())
				if !p.Connected() && !p.Connecting() {
					err := p.connect()
					if err != nil {
						p.logger.Warn("Failed to connect to peer", slog.String("err", err.Error()))
					}
				}
			}
		}()
	}

	p.invBatcher = batcher.New(500, p.batchDelay, p.sendInvBatch, true)
	p.dataBatcher = batcher.New(500, p.batchDelay, p.sendDataBatch, true)
}

func (p *Peer) disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p._disconnect()
}

func (p *Peer) _disconnect() {
	if p.readConn != nil {
		_ = p.readConn.Close()
	}

	p.readConn = nil
	p.writeConn = nil
	p.sentVerAck.Store(false)
	p.receivedVerAck.Store(false)
}

func (p *Peer) connect() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.incomingConn == nil {
		if p.readConn != nil || p.writeConn != nil {
			p._disconnect()
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

	go p.readHandler()

	// write version message to our peer directly and not through the write channel,
	// write channel is not ready to send message until the VERACK handshake is done
	msg := p.versionMessage(p.address)

	// here we can write to the readConn, since we are in the process of connecting and this is the
	// only one that is already open. Opening the writeConn signals that we are done with the handshake
	if err := wire.WriteMessage(p.readConn, msg, wire.ProtocolVersion, p.network); err != nil {
		return fmt.Errorf("failed to write message: %v", err)
	}
	p.logger.Debug("Sent", slog.String("command", strings.ToUpper(msg.Command())))

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

func (p *Peer) readHandler() {
	readConn := p.readConn

	if readConn == nil {
		p.logger.Error("no connection")
		return
	}

	reader := bufio.NewReader(&io.LimitedReader{R: readConn, N: p.maximumMessageSize})
	for {
		msg, b, err := wire.ReadMessage(reader, wire.ProtocolVersion, p.network)
		if err != nil {
			if errors.Is(err, io.EOF) {
				p.logger.Error("failed to read message: EOF", slog.Int("bytes", len(b)), slog.String("msg", string(b)), slog.String("err", err.Error()))
				p.disconnect()
				break
			}

			p.logger.Error("failed to read message", slog.Int("bytes", len(b)), slog.String("msg", string(b)), slog.String("err", err.Error()))
			continue
		}

		wireLogger := p.logger.With(slog.String("cmd", strings.ToUpper(msg.Command())))

		// we could check this based on type (switch msg.(type)) but that would not allow
		// us to override the default behaviour for a specific message type
		switch msg.Command() {
		case wire.CmdVersion:
			wireLogger.Debug("Recv")
			if p.sentVerAck.Load() {
				wireLogger.Warn("Received version message after sending verack")
				continue
			}

			verackMsg := wire.NewMsgVerAck()
			if err = wire.WriteMessage(readConn, verackMsg, wire.ProtocolVersion, p.network); err != nil {
				wireLogger.Error("failed to write message", slog.String("err", err.Error()))
			}
			wireLogger.Debug("Sent message", slog.String("verack", strings.ToUpper(verackMsg.Command())))
			p.sentVerAck.Store(true)

		case wire.CmdPing:
			pingMsg := msg.(*wire.MsgPing)
			p.writeChan <- wire.NewMsgPong(pingMsg.Nonce)

		case wire.CmdInv:
			invMsg := msg.(*wire.MsgInv)
			wireLogger.Debug("Recv INV", slog.Int("items", len(invMsg.InvList)))
			for _, inv := range invMsg.InvList {
				wireLogger.Debug(inv.Hash.String())
			}

			go func(invList []*wire.InvVect) {
				for _, invVect := range invList {
					switch invVect.Type {
					case wire.InvTypeTx:
						if err = p.peerHandler.HandleTransactionAnnouncement(invVect, p); err != nil {
							wireLogger.Error("Unable to process tx", slog.String("hash", invVect.Hash.String()), slog.String("err", err.Error()))
						}
					case wire.InvTypeBlock:
						if err = p.peerHandler.HandleBlockAnnouncement(invVect, p); err != nil {
							wireLogger.Error("Unable to process block", slog.String("hash", invVect.Hash.String()), slog.String("err", err.Error()))
						}
					}
				}
			}(invMsg.InvList)

		case wire.CmdGetData:
			dataMsg := msg.(*wire.MsgGetData)
			if dataMsg == nil {
				continue
			}
			for _, inv := range dataMsg.InvList {
				wireLogger.Debug("Recv", slog.String("hash", inv.Hash.String()))
			}
			p.handleGetDataMsg(dataMsg, wireLogger)

		case wire.CmdTx:
			txMsg := msg.(*wire.MsgTx)
			if txMsg == nil {
				continue
			}
			wireLogger.Debug("Recv", slog.String("hash", txMsg.TxHash().String()), slog.Int("size", txMsg.SerializeSize()))
			if err = p.peerHandler.HandleTransaction(txMsg, p); err != nil {
				wireLogger.Error("Unable to process tx", slog.String("hash", txMsg.TxHash().String()), slog.String("err", err.Error()))
			}

		case wire.CmdBlock:
			msgBlock, ok := msg.(*wire.MsgBlock)
			if ok {
				wireLogger.Info("Recv", slog.String("hash", msgBlock.Header.BlockHash().String()))

				err = p.peerHandler.HandleBlock(msgBlock, p)
				if err != nil {
					wireLogger.Error("Unable to process block", slog.String("hash", msgBlock.Header.BlockHash().String()), slog.String("err", err.Error()))
				}
				continue
			}

			// Please note that this is the BlockMessage, not the wire.MsgBlock
			blockMsg, ok := msg.(*BlockMessage)
			if !ok {
				wireLogger.Error("Unable to cast block message, calling with generic wire.Message")
				err = p.peerHandler.HandleBlock(msg, p)
				if err != nil {
					wireLogger.Error("Unable to process block message", slog.String("err", err.Error()))
				}
				continue
			}

			wireLogger.Info("Recv", slog.String("hash", blockMsg.Header.BlockHash().String()))

			err = p.peerHandler.HandleBlock(blockMsg, p)
			if err != nil {
				wireLogger.Error("Unable to process block", slog.String("hash", blockMsg.Header.BlockHash().String()), slog.String("err", err.Error()))
			}

		case wire.CmdReject:
			rejMsg := msg.(*wire.MsgReject)
			if err = p.peerHandler.HandleTransactionRejection(rejMsg, p); err != nil {
				wireLogger.Error("Unable to process block", slog.String("hash", rejMsg.Hash.String()), slog.String("err", err.Error()))
			}

		case wire.CmdVerAck:
			wireLogger.Debug("Recv")
			p.receivedVerAck.Store(true)

		default:
			wireLogger.Debug("command ignored")
		}
	}

}

func (p *Peer) handleGetDataMsg(dataMsg *wire.MsgGetData, logger *slog.Logger) {
	for _, invVect := range dataMsg.InvList {
		switch invVect.Type {
		case wire.InvTypeTx:
			logger.Debug("Request for TX", slog.String("hash", invVect.Hash.String()))

			txBytes, err := p.peerHandler.HandleTransactionGet(invVect, p)
			if err != nil {
				logger.Warn("Unable to fetch tx from store", slog.String("hash", invVect.Hash.String()), slog.String("err", err.Error()))
				continue
			}

			if txBytes == nil {
				logger.Warn("tx does not exist", slog.String("hash", invVect.Hash.String()), slog.String("err", err.Error()))
				continue
			}

			tx, err := bsvutil.NewTxFromBytes(txBytes)
			if err != nil {
				logger.Error("failed to parse tx", slog.String("hash", invVect.Hash.String()), slog.String("hex string", hex.EncodeToString(txBytes)), slog.String("err", err.Error()))
				continue
			}

			p.writeChan <- tx.MsgTx()

		case wire.InvTypeBlock:
			logger.Info("Request for Block", slog.String("hash", invVect.Hash.String()))

		default:
			logger.Warn("Unknown type", slog.String("type", invVect.Type.String()))
		}
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
		p.logger.Error("failed to add invVect to INV message", slog.String("err", err.Error()))
		return
	}

	if err := p.WriteMsg(invMsg); err != nil {
		p.logger.Error("failed to send INV for block", slog.String("err", err.Error()))
	} else {
		p.logger.Info("Sent INV for block", slog.String("hash", blockHash.String()))
	}
}

func (p *Peer) RequestBlock(blockHash *chainhash.Hash) {
	dataMsg := wire.NewMsgGetData()

	iv := wire.NewInvVect(wire.InvTypeBlock, blockHash)
	if err := dataMsg.AddInvVect(iv); err != nil {
		p.logger.Error("failed to add invVect to GETDATA message", slog.String("err", err.Error()))
		return
	}

	if err := p.WriteMsg(dataMsg); err != nil {
		p.logger.Error("failed to send block data message", slog.String("err", err.Error()))
	} else {
		p.logger.Info("Sent GETDATA for block", slog.String("hash", blockHash.String()))
	}
}

func (p *Peer) sendInvBatch(batch []*chainhash.Hash) {
	invMsg := wire.NewMsgInvSizeHint(uint(len(batch)))

	for _, hash := range batch {
		iv := wire.NewInvVect(wire.InvTypeTx, hash)
		_ = invMsg.AddInvVect(iv)
	}

	p.writeChan <- invMsg

	batchLogger := p.logger.With(slog.Int("items", len(batch)))
	for _, hash := range batch {
		batchLogger.Debug("Sent INV", slog.String("hash", hash.String()))
	}
}

func (p *Peer) sendDataBatch(batch []*chainhash.Hash) {
	dataMsg := wire.NewMsgGetData()

	for _, hash := range batch {
		iv := wire.NewInvVect(wire.InvTypeTx, hash)
		_ = dataMsg.AddInvVect(iv)
	}

	if err := p.WriteMsg(dataMsg); err != nil {
		p.logger.Error("failed to send tx data message", slog.String("err", err.Error()))
	} else {
		p.logger.Info("Sent GETDATA", slog.Int("items", len(batch)))
	}
}

func (p *Peer) writeChannelHandler() {
	for msg := range p.writeChan {
		// wait for the write connection to be ready
		for {
			p.mu.RLock()
			writeConn := p.writeConn
			p.mu.RUnlock()

			if writeConn != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if err := wire.WriteMessage(p.writeConn, msg, wire.ProtocolVersion, p.network); err != nil {
			if errors.Is(err, io.EOF) {
				panic("WRITE EOF")
			}
			p.logger.Error("Failed to write message: %v", slog.String("err", err.Error()))
		}

		go func(msg wire.Message) {
			if msg.Command() == wire.CmdTx {
				hash := msg.(*wire.MsgTx).TxHash()
				if err := p.peerHandler.HandleTransactionSent(msg.(*wire.MsgTx), p); err != nil {
					p.logger.Error("Unable to process tx", slog.String("hash", hash.String()), slog.String("err", err.Error()))
				}
			}

			switch m := msg.(type) {
			case *wire.MsgTx:
				p.logger.Debug("Sent", slog.String("cmd", strings.ToUpper(msg.Command())), slog.String("hash", m.TxHash().String()))
			case *wire.MsgBlock:
				p.logger.Debug("Sent", slog.String("cmd", strings.ToUpper(msg.Command())), slog.String("hash", m.BlockHash().String()))
			case *wire.MsgGetData:
				p.logger.Debug("Sent", slog.String("cmd", strings.ToUpper(msg.Command())), slog.String("hash", m.InvList[0].Hash.String()))
			case *wire.MsgInv:
			default:
				p.logger.Debug("Sent", slog.String("cmd", strings.ToUpper(msg.Command())))
			}
		}(msg)
	}
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
		p.logger.Error("RandomUint64: failed to generate nonce", slog.String("err", err.Error()))
	}

	msg := wire.NewMsgVersion(me, you, nonce, lastBlock)

	return msg
}

// pingHandler periodically pings the peer.  It must be run as a goroutine.
func (p *Peer) pingHandler() {
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

out:
	for {
		select {
		case <-pingTicker.C:
			nonce, err := wire.RandomUint64()
			if err != nil {
				p.logger.Error("Not sending ping", slog.String("err", err.Error()))
				continue
			}
			p.writeChan <- wire.NewMsgPing(nonce)

		case <-p.quit:
			break out
		}
	}
}
