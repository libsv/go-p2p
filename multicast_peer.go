package p2p

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/libsv/go-p2p/chaincfg/chainhash"
	"github.com/libsv/go-p2p/wire"
)

var _ PeerI = (*McastPeer)(nil)

type McastGroup struct {
	MsgType string
	// IPv6 bool			// TODO
	Addr      string
	Interface string
	// WriteOnly bool		// potential optimization
}

type McastPeer struct {
	execWg        sync.WaitGroup
	execCtx       context.Context
	cancelExecCtx context.CancelFunc

	network          wire.BitcoinNet
	mcastGroups      []*McastGroup
	mcastConnections map[string]*net.UDPConn

	l  *slog.Logger
	mh PeerHandlerI

	writeCh    chan wire.Message
	nWriters   uint8
	maxMsgSize int64

	isUnhealthyCh chan struct{}
	startMu       sync.Mutex
	connected     atomic.Bool

	_string string
}

func StartNewMcastPeer(logger *slog.Logger, msgHandler PeerHandlerI, network wire.BitcoinNet, groups []*McastGroup, options ...McastPeerOptions) *McastPeer {
	l := logger.With(
		slog.Group("mcast-peer",
			slog.String("network", network.String()),
		),
	)

	ctx, cancelFn := context.WithCancel(context.Background())

	p := &McastPeer{
		execCtx:       ctx,
		cancelExecCtx: cancelFn,
		l:             l,
		mh:            msgHandler,
		network:       network,
		maxMsgSize:    defaultMaximumMessageSize,
		nWriters:      1,

		isUnhealthyCh: make(chan struct{}),

		mcastGroups:      groups,
		mcastConnections: make(map[string]*net.UDPConn),
	}

	for _, option := range options {
		option(p)
	}

	if p.writeCh == nil {
		p.writeCh = make(chan wire.Message, 1000)
	}

	p.connect()
	return p
}

func (p *McastPeer) Connected() bool {
	return p.connected.Load()
}

func (p *McastPeer) AnnounceTransaction(txHash *chainhash.Hash) {
	invMsg := wire.NewMsgInv()

	iv := wire.NewInvVect(wire.InvTypeTx, txHash)
	_ = invMsg.AddInvVect(iv)

	_ = p.WriteMsg(invMsg)
}
func (p *McastPeer) RequestTransaction(txHash *chainhash.Hash) {
	//panic("not implemented")
	// no requesting in multicast
}
func (p *McastPeer) AnnounceBlock(blockHash *chainhash.Hash) {
	invMsg := wire.NewMsgInv()

	iv := wire.NewInvVect(wire.InvTypeBlock, blockHash)
	_ = invMsg.AddInvVect(iv)

	_ = p.WriteMsg(invMsg)
}
func (p *McastPeer) RequestBlock(blockHash *chainhash.Hash) {
	//panic("not implemented")
	// no requesting in multicast
}

func (p *McastPeer) WriteMsg(msg wire.Message) error {
	p.writeCh <- msg
	return nil
}

func (p *McastPeer) Network() wire.BitcoinNet {
	return p.network
}
func (p *McastPeer) IsHealthy() bool {
	return p.connected.Load()
}

func (p *McastPeer) IsUnhealthyCh() <-chan struct{} {
	return p.isUnhealthyCh
}

func (p *McastPeer) Shutdown() {
	p.startMu.Lock()
	defer p.startMu.Unlock()

	p.l.Info("Shutting down")
	p.disconnect()
	p.l.Info("Shutdown complete")
}

func (p *McastPeer) Restart() {
	p.startMu.Lock()
	defer p.startMu.Unlock()

	p.l.Info("Restarting")
	p.disconnect()
	p.connect()
}

func (p *McastPeer) String() string {
	if p._string != "" {
		return p._string
	}

	b := strings.Builder{}
	for cmd, c := range p.mcastConnections {
		b.WriteString(fmt.Sprintf("[%s-%s]", cmd, c.RemoteAddr().String()))
	}

	p._string = b.String()
	return p._string
}

func (p *McastPeer) connect() {
	p.l.Info("Connecting")

	// connect to groups
	for _, mg := range p.mcastGroups {
		// TODO: validate msg types

		// avoid duplicates
		if _, found := p.mcastConnections[mg.MsgType]; !found {
			continue
		}

		lc, err := joinMcastGroup(mg.Addr, mg.Interface)
		if err != nil { // ignore but maybe should crash?
			p.l.Warn("Failed to join to the group",
				slog.String("mcast-group", mg.Addr),
				slog.String("ifi", mg.Interface),
				slog.String("additional-info", err.Error()),
			)
			continue
		}

		p.mcastConnections[mg.MsgType] = lc
		go p.receiveMessage(lc)
	}

	// run message writers
	for i := uint8(0); i < p.nWriters; i++ {
		go p.sendMessages(i)
	}

	p.connected.Store(true)
	p.l.Info("Ready")
}

func (p *McastPeer) disconnect() {
	p.l.Info("Disconnecting")

	p.cancelExecCtx()
	p.execWg.Wait()

	// close all connections
	for k, c := range p.mcastConnections {
		_ = c.Close()
		delete(p.mcastConnections, k)
	}

	p.connected.Store(false)
	p._string = ""
}

func (p *McastPeer) unhealthyDisconnect() {
	p.disconnect()

	select {
	case p.isUnhealthyCh <- struct{}{}:
	default: // Do not block if nothing is reading from channel
	}

}

func (p *McastPeer) sendMessages(n uint8) {
	p.execWg.Add(1)

	go func() {
		l := p.l.With(slog.Int("instance", int(n)))

		l.Debug("Starting write handler")
		defer p.execWg.Done()

		for {
			select {
			case <-p.execCtx.Done():
				l.Debug("Shutting down write handler")
				return

			case msg := <-p.writeCh:

				cmd := msg.Command()
				c := p.leaseConn(cmd)

				if c == nil {
					l.Warn(fmt.Sprintf("This multicast peer doesn't support %s", cmd))
					continue
				}

				// do not retry
				err := wire.WriteMessage(c, msg, wire.ProtocolVersion, p.network)
				if err != nil {
					l.Error("Failed to send message",
						slogUpperString(commandKey, cmd),
						slog.String("err", err.Error()),
					)

					// stop peer
					go p.unhealthyDisconnect() // avoid DEADLOCK
					return
				}

				// let client react on send (if cmd is Msg.Tx)
				if cmd == wire.CmdTx {
					if msgTx, ok := msg.(*wire.MsgTx); ok {
						go func(msgTx *wire.MsgTx) {

							err := p.mh.HandleTransactionSent(msgTx, p)
							if err != nil {
								hash := msgTx.TxHash()
								l.Error("Unable to process tx",
									slogUpperString(commandKey, cmd),
									slog.String(hashKey, hash.String()),
									slog.String(errKey, err.Error()),
								)
							}
						}(msgTx)
					}
				}

				l.Debug("Sent", slog.String(commandKey, strings.ToUpper(msg.Command())))
			}
		}
	}()
}

func (p *McastPeer) leaseConn(cmdType string) *net.UDPConn {
	return p.mcastConnections[cmdType]
}

func (p *McastPeer) receiveMessage(c *net.UDPConn) {
	p.execWg.Add(1)

	go func() {
		l := p.l.With(slog.String("mcast-group", c.RemoteAddr().String()))

		l.Debug("Starting read handler")
		defer p.execWg.Done()

		// TODO: probably has to use adapter to read from udp packages
		reader := bufio.NewReader(&io.LimitedReader{R: c, N: p.maxMsgSize})
		for {
			select {
			case <-p.execCtx.Done():
				l.Debug("Shutting down read handler")
				return

			default:
				// do not retry
				msg, err := nonBlockingMsgRead(p.execCtx, reader, wire.ProtocolVersion, p.network)
				if err != nil {
					l.Error("Read failed", slog.String(errKey, err.Error()))

					// stop peer
					go p.unhealthyDisconnect() // avoid DEADLOCK
					return
				}

				cmd := msg.Command()
				// TODO: think if supported commands should be configured per group
				switch cmd {
				// support INV, TX, BLOCK, REJECT
				case wire.CmdInv:
					invMsg, ok := msg.(*wire.MsgInv)
					if !ok {
						// no warning?
						continue
					}

					if p.l.Handler().Enabled(context.Background(), slog.LevelDebug) {
						// avoid unnecessary args evaluation

						for _, inv := range invMsg.InvList {
							l.Debug(receivedMsg,
								slogUpperString(commandKey, cmd),
								slog.String(hashKey, inv.Hash.String()),
								slog.String(typeKey, inv.Type.String()),
							)
						}
					}

					// handle message
					go func(invVectors []*wire.InvVect) {
						for _, inv := range invVectors {
							switch inv.Type {
							case wire.InvTypeTx:
								if err := p.mh.HandleTransactionAnnouncement(inv, p); err != nil {
									l.Error("Unable to process tx",
										slogUpperString(commandKey, cmd),
										slog.String(hashKey, inv.Hash.String()),
										slog.String(typeKey, inv.Type.String()),
										slog.String(errKey, err.Error()))
								}
							case wire.InvTypeBlock:
								if err = p.mh.HandleBlockAnnouncement(inv, p); err != nil {
									l.Error("Unable to process block",
										slogUpperString(commandKey, cmd),
										slog.String(hashKey, inv.Hash.String()),
										slog.String(typeKey, inv.Type.String()),
										slog.String(errKey, err.Error()))
								}
							}
						}
					}(invMsg.InvList)

				case wire.CmdTx:
					txMsg, ok := msg.(*wire.MsgTx)
					if !ok {
						// no warning?
						continue
					}

					if p.l.Handler().Enabled(context.Background(), slog.LevelDebug) {
						// avoid unnecessary args evaluation

						l.Debug(receivedMsg,
							slogUpperString(commandKey, cmd),
							slog.String(hashKey, txMsg.TxHash().String()),
							slog.Int("size", txMsg.SerializeSize()),
						)
					}

					// handle message
					// no new go routine?
					if err = p.mh.HandleTransaction(txMsg, p); err != nil {
						l.Error("Unable to process tx",
							slogUpperString(commandKey, cmd),
							slog.String(hashKey, txMsg.TxHash().String()),
							slog.String(errKey, err.Error()),
						)
					}

				case wire.CmdBlock:
					// MsgBlock reading can be overridden by client - no need to do interface assertion

					// handle message
					// no new go routine?
					if err = p.mh.HandleBlock(msg, p); err != nil {
						l.Error("Unable to process block",
							slogUpperString(commandKey, cmd),
							slog.String(errKey, err.Error()),
						)
					}

				case wire.CmdReject:
					rejMsg, ok := msg.(*wire.MsgReject)
					if !ok {
						// no warning?
						continue
					}
					if err = p.mh.HandleTransactionRejection(rejMsg, p); err != nil {
						l.Error("Unable to process block",
							slogUpperString(commandKey, cmd),
							slog.String(hashKey, rejMsg.Hash.String()),
							slog.String(errKey, err.Error()),
						)
					}

				default:
					l.Warn("unsupported message", slog.String(commandKey, strings.ToUpper(cmd)))
				}

			}
		}
	}()
}

func slogUpperString(key, val string) slog.Attr {
	return slog.String(key, strings.ToUpper(val))
}

func joinMcastGroup(address, nInterface string) (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp6", address)
	if err != nil {
		return nil, err
	}

	ifi, err := net.InterfaceByName(nInterface)
	if err != nil {
		return nil, err
	}

	lc, err := net.ListenMulticastUDP("udp6", ifi, addr)
	if err != nil {
		return nil, err
	}

	return lc, nil
}

func nonBlockingMsgRead(ctx context.Context, r io.Reader, pver uint32, bsvnet wire.BitcoinNet) (wire.Message, error) {
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
		return readMsg.msg, readMsg.err
	}
}
