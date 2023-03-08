package p2p

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libsv/go-p2p/bsvutil"
	"github.com/libsv/go-p2p/chaincfg/chainhash"
	"github.com/libsv/go-p2p/wire"
	"github.com/ordishs/go-utils/batcher"

	"github.com/ordishs/go-utils"
	"github.com/ordishs/gocore"
)

func init() {
	// override the default wire block handler with our own that streams and stores only the transaction ids
	setPeerBlockHandler()
}

var (
	pingInterval = 2 * time.Minute
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
	address        string
	network        wire.BitcoinNet
	mu             sync.RWMutex
	readConn       net.Conn
	writeConn      net.Conn
	incomingConn   net.Conn
	dial           func(network, address string) (net.Conn, error)
	peerHandler    PeerHandlerI
	writeChan      chan wire.Message
	quit           chan struct{}
	logger         utils.Logger
	sentVerAck     atomic.Bool
	receivedVerAck atomic.Bool
	batchDelay     time.Duration
	invBatcher     *batcher.Batcher[chainhash.Hash]
	dataBatcher    *batcher.Batcher[chainhash.Hash]
}

// NewPeer returns a new bitcoin peer for the provided address and configuration.
func NewPeer(logger utils.Logger, address string, peerHandler PeerHandlerI, network wire.BitcoinNet, options ...PeerOptions) (*Peer, error) {
	writeChan := make(chan wire.Message, 10000)

	p := &Peer{
		network:     network,
		address:     address,
		writeChan:   writeChan,
		peerHandler: peerHandler,
		logger:      logger,
		dial:        net.Dial,
	}

	for _, option := range options {
		option(p)
	}

	go p.pingHandler()
	for i := 0; i < 10; i++ {
		// start 10 workers that will write to the peer
		// locking is done in the net.write in the wire/message handler
		// this reduces the wait on the writer when processing writes (for example HandleTransactionSent)
		go p.writeChannelHandler()
	}

	if p.incomingConn != nil {
		p.logger.Infof("[%s] Incoming connection from peer on %s", p.address, p.network)
		go func() {
			err := p.connect()
			if err != nil {
				logger.Warnf("Failed to connect to peer %s: %v", address, err)
			}
		}()
	} else {
		// reconnect if disconnected, but only on outgoing connections
		err := p.connect()
		if err != nil {
			logger.Warnf("Failed to connect to peer %s: %v", address, err)
		}

		go func() {
			for range time.NewTicker(10 * time.Second).C {
				// logger.Debugf("checking connection to peer %s, connected = %t, connecting = %t", address, p.Connected(), p.Connecting())
				if !p.Connected() && !p.Connecting() {
					err = p.connect()
					if err != nil {
						logger.Warnf("Failed to connect to peer %s: %v", address, err)
					}
				}
			}
		}()
	}

	if p.batchDelay == 0 {
		batchDelayMillis, _ := gocore.Config().GetInt("peerManager_batchDelay_millis", 100)
		p.batchDelay = time.Duration(batchDelayMillis) * time.Millisecond
	}
	p.invBatcher = batcher.New(500, p.batchDelay, p.sendInvBatch, true)
	p.dataBatcher = batcher.New(500, p.batchDelay, p.sendDataBatch, true)

	return p, nil
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
		p.logger.Infof("[%s] Connecting to peer on %s", p.address, p.network)
		conn, err := p.dial("tcp", p.address)
		if err != nil {
			return fmt.Errorf("could not dial node [%s]: %v", p.address, err)
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
	p.logger.Debugf("[%s] Sent %s", p.address, strings.ToUpper(msg.Command()))

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

	p.logger.Infof("[%s] Connected to peer on %s", p.address, p.network)

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

	if readConn != nil {
		reader := bufio.NewReader(&io.LimitedReader{R: readConn, N: 32 * 1024 * 1024})
		for {
			msg, b, err := wire.ReadMessage(reader, wire.ProtocolVersion, p.network)
			if err != nil {
				if errors.Is(err, io.EOF) {
					p.logger.Errorf(fmt.Sprintf("READ EOF whilst reading from %s [%d bytes], are you on the right network?\n%s", p.address, len(b), string(b)))
					p.disconnect()
					break
				}
				p.logger.Errorf("[%s] Failed to read message: %v", p.address, err)
				continue
			}

			// we could check this based on type (switch msg.(type)) but that would not allow
			// us to override the default behaviour for a specific message type
			switch msg.Command() {
			case wire.CmdVersion:
				p.logger.Debugf("[%s] Recv %s", p.address, strings.ToUpper(msg.Command()))
				if p.sentVerAck.Load() {
					p.logger.Warnf("[%s] Received version message after sending verack", p.address)
					continue
				}

				verackMsg := wire.NewMsgVerAck()
				if err = wire.WriteMessage(readConn, verackMsg, wire.ProtocolVersion, p.network); err != nil {
					p.logger.Errorf("[%s] failed to write message: %v", p.address, err)
				}
				p.logger.Debugf("[%s] Sent %s", p.address, strings.ToUpper(verackMsg.Command()))
				p.sentVerAck.Store(true)

			case wire.CmdPing:
				pingMsg := msg.(*wire.MsgPing)
				p.writeChan <- wire.NewMsgPong(pingMsg.Nonce)

			case wire.CmdInv:
				invMsg := msg.(*wire.MsgInv)
				if p.logger.LogLevel() == int(gocore.DEBUG) {
					p.logger.Debugf("[%s] Recv INV (%d items)", p.address, len(invMsg.InvList))
					for _, inv := range invMsg.InvList {
						p.logger.Debugf("        [%s] %s", p.address, inv.Hash.String())
					}
				}

				go func(invList []*wire.InvVect) {
					for _, invVect := range invList {
						switch invVect.Type {
						case wire.InvTypeTx:
							if err = p.peerHandler.HandleTransactionAnnouncement(invVect, p); err != nil {
								p.logger.Errorf("[%s] Unable to process tx %s: %v", p.address, invVect.Hash.String(), err)
							}
						case wire.InvTypeBlock:
							if err = p.peerHandler.HandleBlockAnnouncement(invVect, p); err != nil {
								p.logger.Errorf("[%s] Unable to process block %s: %v", p.address, invVect.Hash.String(), err)
							}
						}
					}
				}(invMsg.InvList)

			case wire.CmdGetData:
				dataMsg := msg.(*wire.MsgGetData)
				p.logger.Infof("[%s] Recv GETDATA (%d items)", p.address, len(dataMsg.InvList))
				if p.logger.LogLevel() == int(gocore.DEBUG) {
					for _, inv := range dataMsg.InvList {
						p.logger.Debugf("        [%s] %s", p.address, inv.Hash.String())
					}
				}
				p.handleGetDataMsg(dataMsg)

			case wire.CmdTx:
				txMsg := msg.(*wire.MsgTx)
				p.logger.Infof("Recv TX %s (%d bytes)", txMsg.TxHash().String(), txMsg.SerializeSize())
				if err = p.peerHandler.HandleTransaction(txMsg, p); err != nil {
					p.logger.Errorf("Unable to process tx %s: %v", txMsg.TxHash().String(), err)
				}

			case wire.CmdBlock:
				// Please note that this is the BlockMessage, not the wire.MsgBlock. See init() above.
				blockMsg, ok := msg.(*BlockMessage)
				if !ok {
					p.logger.Errorf("Unable to cast block message")
					continue
				}

				p.logger.Infof("[%s] Recv %s: %s", p.address, strings.ToUpper(msg.Command()), blockMsg.Header.BlockHash().String())

				err := p.peerHandler.HandleBlock(blockMsg, p)
				if err != nil {
					p.logger.Errorf("[%s] Unable to process block %s: %v", p.address, blockMsg.Header.BlockHash().String(), err)
				}

			case wire.CmdReject:
				rejMsg := msg.(*wire.MsgReject)
				if err = p.peerHandler.HandleTransactionRejection(rejMsg, p); err != nil {
					p.logger.Errorf("[%s] Unable to process block %s: %v", p.address, rejMsg.Hash.String(), err)
				}

			case wire.CmdVerAck:
				p.logger.Debugf("[%s] Recv %s", p.address, strings.ToUpper(msg.Command()))
				p.receivedVerAck.Store(true)

			default:
				p.logger.Debugf("[%s] Ignored %s", p.address, strings.ToUpper(msg.Command()))
			}
		}
	}
}

func (p *Peer) handleGetDataMsg(dataMsg *wire.MsgGetData) {
	for _, invVect := range dataMsg.InvList {
		switch invVect.Type {
		case wire.InvTypeTx:
			p.logger.Debugf("[%s] Request for TX: %s\n", p.address, invVect.Hash.String())

			txBytes, err := p.peerHandler.HandleTransactionGet(invVect, p)
			if err != nil {
				p.logger.Warnf("[%s] Unable to fetch tx %s from store: %v", p.address, invVect.Hash.String(), err)
				continue
			}

			if txBytes == nil {
				p.logger.Warnf("[%s] Unable to fetch tx %s from store: %v", p.address, invVect.Hash.String(), err)
				continue
			}

			tx, err := bsvutil.NewTxFromBytes(txBytes)
			if err != nil {
				log.Print(err) // Log and handle the error
				continue
			}

			p.writeChan <- tx.MsgTx()

		case wire.InvTypeBlock:
			p.logger.Infof("[%s] Request for Block: %s\n", p.address, invVect.Hash.String())

		default:
			p.logger.Warnf("[%s] Unknown type: %d\n", p.address, invVect.Type)
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
		p.logger.Infof("ERROR adding invVect to INV message: %v", err)
		return
	}

	if err := p.WriteMsg(invMsg); err != nil {
		p.logger.Infof("[%s] ERROR sending INV for block: %v", p.String(), err)
	} else {
		p.logger.Infof("[%s] Sent INV for block %v", p.String(), blockHash)
	}
}

func (p *Peer) RequestBlock(blockHash *chainhash.Hash) {
	dataMsg := wire.NewMsgGetData()

	iv := wire.NewInvVect(wire.InvTypeTx, blockHash)
	if err := dataMsg.AddInvVect(iv); err != nil {
		p.logger.Infof("ERROR adding invVect to GETDATA message: %v", err)
		return
	}

	if err := p.WriteMsg(dataMsg); err != nil {
		p.logger.Infof("[%s] ERROR sending block data message: %v", p.String(), err)
	} else {
		p.logger.Infof("[%s] Sent GETDATA for block %v", p.String(), blockHash)
	}
}

func (p *Peer) sendInvBatch(batch []*chainhash.Hash) {
	invMsg := wire.NewMsgInvSizeHint(uint(len(batch)))

	for _, hash := range batch {
		iv := wire.NewInvVect(wire.InvTypeTx, hash)
		_ = invMsg.AddInvVect(iv)
	}

	p.writeChan <- invMsg

	p.logger.Infof("[%s] Sent INV (%d items)", p.String(), len(batch))
	for _, hash := range batch {
		p.logger.Debugf("        %v", hash)
	}
}

func (p *Peer) sendDataBatch(batch []*chainhash.Hash) {
	dataMsg := wire.NewMsgGetData()

	for _, hash := range batch {
		iv := wire.NewInvVect(wire.InvTypeTx, hash)
		_ = dataMsg.AddInvVect(iv)
	}

	if err := p.WriteMsg(dataMsg); err != nil {
		p.logger.Infof("[%s] ERROR sending tx data message: %v", p.String(), err)
	} else {
		p.logger.Infof("[%s] Sent GETDATA (%d items)", p.String(), len(batch))
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
			p.logger.Errorf("[%s] Failed to write message: %v", p.address, err)
		}

		go func(msg wire.Message) {
			if msg.Command() == wire.CmdTx {
				hash := msg.(*wire.MsgTx).TxHash()
				if err := p.peerHandler.HandleTransactionSent(msg.(*wire.MsgTx), p); err != nil {
					p.logger.Errorf("[%s] Unable to process tx %s: %v", p.address, hash.String(), err)
				}
			}

			switch m := msg.(type) {
			case *wire.MsgTx:
				p.logger.Debugf("[%s] Sent %s: %s", p.address, strings.ToUpper(msg.Command()), m.TxHash().String())
			case *wire.MsgBlock:
				p.logger.Debugf("[%s] Sent %s: %s", p.address, strings.ToUpper(msg.Command()), m.BlockHash().String())
			case *wire.MsgGetData:
				p.logger.Debugf("[%s] Sent %s: %s", p.address, strings.ToUpper(msg.Command()), m.InvList[0].Hash.String())
			case *wire.MsgInv:
			default:
				p.logger.Debugf("[%s] Sent %s", p.address, strings.ToUpper(msg.Command()))
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
		p.logger.Errorf("[%s] RandomUint64: error generating nonce: %v", p.address, err)
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
				p.logger.Errorf("[%s] Not sending ping to %s: %v", p.address, p, err)
				continue
			}
			p.writeChan <- wire.NewMsgPing(nonce)

		case <-p.quit:
			break out
		}
	}
}
