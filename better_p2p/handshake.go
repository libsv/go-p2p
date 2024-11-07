package better_p2p

/* alternative Handshake implementation */

import (
	"context"
	"log/slog"
	"net"
	"time"

	"github.com/libsv/go-p2p/wire"
)

//nolint:staticcheck
func handshakeWith(p *Peer) (ok bool) {
	/* 1. send VER
	 * 2. wait for VERACK
	 * 3. wait for VER from node
	 * 4. send VERACK
	 */

	// send VerMsg
	ok = sendVerMsg(p)
	if !ok {
		return false
	}

	// wait for ACK, and VER from node send VERACK
	return finalizeHandshake(p)
}

//nolint:staticcheck
func sendVerMsg(p *Peer) (ok bool) {
	me := wire.NewNetAddress(&net.TCPAddr{IP: nil, Port: 0}, wire.SFNodeNetwork) // shouldn't be mode configurable?

	nAddr, _ := net.ResolveTCPAddr("tcp", p.address) // address was validate already, we can omit error
	you := wire.NewNetAddress(nAddr, wire.SFNodeNetwork)

	nonce, err := wire.RandomUint64()
	if err != nil {
		p.l.Warn("Handshake: failed to generate nonce, send VER with 0 nonce", slog.String(errKey, err.Error()))
	}

	const lastBlock = int32(0)
	verMsg := wire.NewMsgVersion(me, you, nonce, lastBlock)

	if p.userAgentName != nil && p.userAgentVersion != nil {
		err = verMsg.AddUserAgent(*p.userAgentName, *p.userAgentVersion)
		if err != nil {
			p.l.Warn("Handshake: failed to add user agent, send VER without user agent", slog.String(errKey, err.Error()))
		}
	}

	err = wire.WriteMessage(p.lConn, verMsg, wire.ProtocolVersion, p.network)
	if err != nil {
		p.l.Error("Handshake failed.",
			slog.String("reason", "failed to write VER message"),
			slog.String(errKey, err.Error()),
		)

		return false
	}

	p.l.Debug("Sent", slogUpperString(commandKey, verMsg.Command()))
	return true
}

//nolint:staticcheck
func finalizeHandshake(p *Peer) (ok bool) {

	// do not block goroutine on read
	readCtx, readCancel := context.WithCancel(p.execCtx)
	defer readCancel()

	read := make(chan readMessageResult, 1)
	readController := make(chan struct{}, 1)

	go func(ctx context.Context) {
		for {
			select {
			case <-readCtx.Done():
				return

			case <-readController:
				msg, _, err := wire.ReadMessage(p.lConn, wire.ProtocolVersion, p.network)
				read <- readMessageResult{msg, err}
			}
		}
	}(readCtx)

	receivedVerAck := false
	sentVerAck := false

	// wait for ACK, and VER from node send VERACK
handshakeLoop:
	for {
		// "read" next message
		readController <- struct{}{}

		select {
		// peer was stopped
		case <-p.execCtx.Done():
			return false

		case <-time.After(1 * time.Minute):
			p.l.Error("Handshake failed.", slog.String("reason", "handshake timeout"))
			return false

		case result := <-read:
			if result.err != nil {
				p.l.Error("Handshake failed.", slog.String(errKey, result.err.Error()))
				return false
			}

			receivedVerAck, sentVerAck = onHandshakeMsg(p, result.msg, receivedVerAck, sentVerAck)

			if receivedVerAck && sentVerAck {
				break handshakeLoop
			}
		}
	}

	// if we exit the handshake loop, the handshake has completed successfully
	return true
}

//nolint:staticcheck
func onHandshakeMsg(p *Peer, msg wire.Message, outReceivedVerAck, outSentVerAck bool) (bool, bool) {

	switch msg.Command() {
	case wire.CmdVerAck:
		p.l.Debug("Handshake: received VERACK")
		outReceivedVerAck = true

	case wire.CmdVersion:
		p.l.Debug("Handshake: received VER")
		if outSentVerAck {
			p.l.Warn("Handshake: received version message after sending verack.")
			return outReceivedVerAck, outSentVerAck
		}

		// send VERACK to node
		ackMsg := wire.NewMsgVerAck()
		err := wire.WriteMessage(p.lConn, ackMsg, wire.ProtocolVersion, p.network)
		if err != nil {
			p.l.Error("Handshake failed.",
				slog.String("reason", "failed to write VERACK message"),
				slog.String(errKey, err.Error()),
			)

			return outReceivedVerAck, outSentVerAck
		}

		p.l.Debug("Handshake: sent VERACK")
		outSentVerAck = true

	default:
		p.l.Warn("Handshake: received unexpected message. Message was ignored", slogUpperString(commandKey, msg.Command()))
	}

	return outReceivedVerAck, outSentVerAck
}
