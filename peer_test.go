package p2p

import (
	"bytes"
	"encoding/binary"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/cbeuw/connutil"
	"github.com/libsv/go-p2p/chaincfg/chainhash"
	"github.com/libsv/go-p2p/test"
	"github.com/libsv/go-p2p/wire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLittleEndian(t *testing.T) {
	le := binary.LittleEndian.Uint32([]byte{0x50, 0xcc, 0x0b, 0x00})

	require.Equal(t, uint32(773200), le)
}

func Test_connect(t *testing.T) {
	t.Run("connect", func(t *testing.T) {
		_, p, _ := newTestPeer(t)
		assert.True(t, p.Connected())
	})
}

func Test_incomingConnection(t *testing.T) {
	t.Run("incoming", func(t *testing.T) {
		peerConn, p, _ := newIncomingTestPeer(t)

		doHandshake(t, p, peerConn)

		// we need to wait for at least 10 milliseconds for the veracks to finish
		time.Sleep(20 * time.Millisecond)

		// we should be connected, even if we have not sent a version message
		assert.True(t, p.Connected())
	})
}

func TestString(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		_, p, _ := newTestPeer(t)
		assert.Equal(t, "MockPeerHandler:0000", p.String())
	})
}

func TestDisconnected(t *testing.T) {
	t.Run("disconnected", func(t *testing.T) {
		_, p, _ := newTestPeer(t)
		assert.True(t, p.Connected())

		p.disconnect()
		assert.False(t, p.Connected())
	})
}

func TestWriteMsg(t *testing.T) {
	t.Run("write message - ping", func(t *testing.T) {
		myConn, _, _ := newTestPeer(t)

		err := wire.WriteMessage(myConn, wire.NewMsgPing(1), wire.ProtocolVersion, wire.MainNet)
		require.NoError(t, err)

		msg, n, err := wire.ReadMessage(myConn, wire.ProtocolVersion, wire.MainNet)
		assert.NotEqual(t, 0, n)
		assert.NoError(t, err)
		assert.Equal(t, wire.CmdPong, msg.Command())
	})

	t.Run("write message - tx inv", func(t *testing.T) {
		myConn, p, _ := newTestPeer(t)

		invMsg := wire.NewMsgInv()
		hash, err := chainhash.NewHashFromStr(tx1)
		require.NoError(t, err)
		err = invMsg.AddInvVect(wire.NewInvVect(wire.InvTypeTx, hash))
		require.NoError(t, err)

		// let peer write to us
		err = p.WriteMsg(invMsg)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		msg, n, err := wire.ReadMessage(myConn, wire.ProtocolVersion, wire.MainNet)
		assert.NotEqual(t, 0, n)
		assert.NoError(t, err)
		assert.Equal(t, wire.CmdInv, msg.Command())
		assert.Equal(t, wire.InvTypeTx, msg.(*wire.MsgInv).InvList[0].Type)
		assert.Equal(t, tx1, msg.(*wire.MsgInv).InvList[0].Hash.String())
	})
}

func TestShutdown(t *testing.T) {
	tt := []struct {
		name string
	}{
		{
			name: "Shutdown",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			peerConn, myConn := connutil.AsyncPipe()

			peerHandler := NewMockPeerHandler()
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
			p, err := NewPeer(
				logger,
				"MockPeerHandler:0000",
				peerHandler,
				wire.MainNet,
				WithDialer(func(network, address string) (net.Conn, error) {
					return peerConn, nil
				}),
				WithRetryReadWriteMessageInterval(200*time.Millisecond),
			)
			require.NoError(t, err)

			doHandshake(t, p, myConn)

			// wait for the peer to be connected
		connectLoop:
			for {
				select {
				case <-time.NewTicker(10 * time.Millisecond).C:
					if p.Connected() {
						break connectLoop
					}
				case <-time.NewTimer(1 * time.Second).C:
					t.Fatal("peer did not connect")
				}
			}

			invMsg := wire.NewMsgInv()
			hash, err := chainhash.NewHashFromStr(tx1)
			require.NoError(t, err)
			err = invMsg.AddInvVect(wire.NewInvVect(wire.InvTypeTx, hash))
			require.NoError(t, err)

			t.Log("shutdown")
			p.Shutdown()
		})
	}
}

func TestRestart(t *testing.T) {
	tt := []struct {
		name string
	}{
		{
			name: "Restart",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			peerConn, myConn := connutil.AsyncPipe()

			peerHandler := NewMockPeerHandler()
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
			p, err := NewPeer(
				logger,
				"MockPeerHandler:0000",
				peerHandler,
				wire.MainNet,
				WithDialer(func(network, address string) (net.Conn, error) {
					return peerConn, nil
				}),
				WithRetryReadWriteMessageInterval(200*time.Millisecond),
			)
			require.NoError(t, err)

			t.Log("handshake 1")
			handshakeFinished := make(chan struct{})
			go func() {
				doHandshake(t, p, myConn)
				handshakeFinished <- struct{}{}
			}()

			select {
			case <-handshakeFinished:
				t.Log("handshake 1 finished")
			case <-time.After(5 * time.Second):
				t.Fatal("handshake 1 timeout")
			}
			// wait for the peer to be connected
		connectLoop:
			for {
				select {
				case <-time.NewTicker(10 * time.Millisecond).C:
					if p.Connected() {
						break connectLoop
					}
				case <-time.NewTimer(1 * time.Second).C:
					t.Fatal("peer did not connect")
				}
			}

			invMsg := wire.NewMsgInv()
			hash, err := chainhash.NewHashFromStr(tx1)
			require.NoError(t, err)
			err = invMsg.AddInvVect(wire.NewInvVect(wire.InvTypeTx, hash))
			require.NoError(t, err)

			t.Log("restart")
			p.Restart()

			// wait for the peer to be disconnected
		disconnectLoop:
			for {
				select {
				case <-time.NewTicker(10 * time.Millisecond).C:
					if !p.Connected() {
						break disconnectLoop
					}
				case <-time.NewTimer(5 * time.Second).C:
					t.Fatal("peer did not disconnect")
				}
			}

			//time.Sleep(15 * time.Second)
			// recreate connection
			p.mu.Lock()
			peerConn, myConn = connutil.AsyncPipe()
			p.mu.Unlock()
			t.Log("new connection created")

			time.Sleep(5 * time.Second)
			t.Log("handshake 2")

			go func() {
				doHandshake(t, p, myConn)
				handshakeFinished <- struct{}{}
			}()

			select {
			case <-handshakeFinished:
				t.Log("handshake 2 finished")
			case <-time.After(5 * time.Second):
				t.Fatal("handshake 2 timeout")
			}

			t.Log("reconnect")
			// wait for the peer to be reconnected
		reconnectLoop:
			for {
				select {
				case <-time.NewTicker(10 * time.Millisecond).C:
					if p.Connected() {
						break reconnectLoop
					}
				case <-time.NewTimer(1 * time.Second).C:
					t.Fatal("peer did not reconnect")
				}
			}
		})
	}
}

func TestReconnect(t *testing.T) {
	tt := []struct {
		name       string
		cancelRead bool
	}{
		{
			name:       "writer connection breaks - reconnect",
			cancelRead: true,
		},
		{
			name: "reader connection breaks - reconnect",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			peerConn, myConn := connutil.AsyncPipe()

			peerHandler := NewMockPeerHandler()
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
			peer, err := NewPeer(
				logger,
				"MockPeerHandler:0000",
				peerHandler,
				wire.MainNet,
				WithDialer(func(network, address string) (net.Conn, error) {
					return peerConn, nil
				}),
				WithRetryReadWriteMessageInterval(200*time.Millisecond),
			)
			require.NoError(t, err)

			t.Log("handshake 1")
			handshakeFinished := make(chan struct{})
			go func() {
				doHandshake(t, peer, myConn)
				handshakeFinished <- struct{}{}
			}()

			select {
			case <-handshakeFinished:
				t.Log("handshake 1 finished")
			case <-time.After(5 * time.Second):
				t.Fatal("handshake 1 timeout")
			}

			t.Log("expect that peer has connected")
		connectLoop:
			for {
				select {
				case <-time.NewTicker(10 * time.Millisecond).C:
					if peer.Connected() {
						break connectLoop
					}
				case <-time.NewTimer(1 * time.Second).C:
					t.Fatal("peer did not connect")
				}
			}

			invMsg := wire.NewMsgInv()
			hash, err := chainhash.NewHashFromStr(tx1)
			require.NoError(t, err)
			err = invMsg.AddInvVect(wire.NewInvVect(wire.InvTypeTx, hash))
			require.NoError(t, err)

			if tc.cancelRead {
				// cancel reader so that writer will disconnect
				stopReadHandler(peer)
			} else {
				// cancel writer so that reader will disconnect
				stopWriteHandler(peer)
			}

			// break connection
			err = myConn.Close()
			require.NoError(t, err)

			err = peer.WriteMsg(invMsg)
			require.NoError(t, err)

			t.Log("expect that peer has disconnected")
		disconnectLoop:
			for {
				select {
				case <-time.NewTicker(200 * time.Millisecond).C:
					if !peer.Connected() {
						break disconnectLoop
					}
				case <-time.NewTimer(6 * time.Second).C:
					t.Fatal("peer did not disconnect")
				}
			}

			// recreate connection
			peer.mu.Lock()
			peerConn, myConn = connutil.AsyncPipe()
			peer.mu.Unlock()
			t.Log("new connection created")
			time.Sleep(5 * time.Second)

			t.Log("handshake 2")

			go func() {
				doHandshake(t, peer, myConn)
				handshakeFinished <- struct{}{}
			}()

			select {
			case <-handshakeFinished:
				t.Log("handshake 2 finished")
			case <-time.After(5 * time.Second):
				t.Fatal("handshake 2 timeout")
			}

			t.Log("expect that peer has re-established connection")
		reconnectLoop:
			for {
				select {
				case <-time.NewTicker(200 * time.Millisecond).C:
					if peer.Connected() {
						break reconnectLoop
					}
				case <-time.NewTimer(2 * time.Second).C:
					t.Fatal("peer did not reconnect")
				}
			}

			t.Log("shutdown")
			shutdownFinished := make(chan struct{})
			go func() {
				peer.Shutdown()
				shutdownFinished <- struct{}{}
			}()

			select {
			case <-shutdownFinished:
				t.Log("shutdown finished")
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown timeout")
			}

			err = myConn.Close()
			require.NoError(t, err)
		})
	}
}

func Test_readHandler(t *testing.T) {
	t.Run("read message - inv tx", func(t *testing.T) {
		myConn, _, peerHandler := newTestPeer(t)

		invMsg := wire.NewMsgInv()
		err := invMsg.AddInvVect(wire.NewInvVect(wire.InvTypeTx, &chainhash.Hash{}))
		require.NoError(t, err)

		err = wire.WriteMessage(myConn, invMsg, wire.ProtocolVersion, wire.MainNet)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		txAnnouncements := peerHandler.GetTransactionAnnouncement()
		blockAnnouncements := peerHandler.GetBlockAnnouncement()
		require.Equal(t, 1, len(txAnnouncements))
		require.Equal(t, 0, len(blockAnnouncements))
		assert.Equal(t, chainhash.Hash{}, txAnnouncements[0].Hash)
	})

	t.Run("read message - inv block", func(t *testing.T) {
		myConn, _, peerHandler := newTestPeer(t)

		invMsg := wire.NewMsgInv()
		err := invMsg.AddInvVect(wire.NewInvVect(wire.InvTypeBlock, &chainhash.Hash{}))
		require.NoError(t, err)

		err = wire.WriteMessage(myConn, invMsg, wire.ProtocolVersion, wire.MainNet)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		txAnnouncements := peerHandler.GetTransactionAnnouncement()
		blockAnnouncements := peerHandler.GetBlockAnnouncement()
		require.Equal(t, 0, len(txAnnouncements))
		require.Equal(t, 1, len(blockAnnouncements))
		assert.Equal(t, chainhash.Hash{}, blockAnnouncements[0].Hash)
	})

	t.Run("read message - get data tx", func(t *testing.T) {
		myConn, _, peerHandler := newTestPeer(t)

		msg := wire.NewMsgGetData()
		hash, err := chainhash.NewHashFromStr(tx1)
		require.NoError(t, err)
		err = msg.AddInvVect(wire.NewInvVect(wire.InvTypeTx, hash))
		require.NoError(t, err)

		// set the bytes our peer handler should return
		peerHandler.transactionGetBytes = map[string][]byte{
			tx1: test.TX1RawBytes,
		}

		err = wire.WriteMessage(myConn, msg, wire.ProtocolVersion, wire.MainNet)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		txMsg, n, err := wire.ReadMessage(myConn, wire.ProtocolVersion, wire.MainNet)
		assert.NotEqual(t, 0, n)
		assert.NoError(t, err)
		assert.Equal(t, wire.CmdTx, txMsg.Command())

		require.Equal(t, 1, len(peerHandler.transactionGet))
		assert.Equal(t, tx1, peerHandler.transactionGet[0].Hash.String())
		buf := bytes.NewBuffer(make([]byte, 0, txMsg.(*wire.MsgTx).SerializeSize()))
		err = txMsg.(*wire.MsgTx).Serialize(buf)
		require.NoError(t, err)
		assert.Equal(t, test.TX1RawBytes, buf.Bytes())
	})

	t.Run("read message - tx", func(t *testing.T) {
		myConn, _, peerHandler := newTestPeer(t)

		msg := wire.NewMsgTx(1)
		err := msg.Deserialize(bytes.NewReader(test.TX1RawBytes))
		require.NoError(t, err)

		err = wire.WriteMessage(myConn, msg, wire.ProtocolVersion, wire.MainNet)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		transactions := peerHandler.GetTransaction()
		require.Equal(t, 1, len(transactions))
		transaction := transactions[0]
		buf := bytes.NewBuffer(make([]byte, 0, transaction.SerializeSize()))
		err = transaction.Serialize(buf)
		require.NoError(t, err)
		assert.Equal(t, test.TX1RawBytes, buf.Bytes())
	})

	t.Run("read message - block", func(t *testing.T) {
		myConn, _, peerHandler := newTestPeer(t)

		msg := wire.NewMsgBlock(&wire.BlockHeader{
			Version:    1,
			PrevBlock:  chainhash.Hash{},
			MerkleRoot: chainhash.Hash{},
			Timestamp:  time.Time{},
			Bits:       123,
			Nonce:      321,
		})
		tx := wire.NewMsgTx(1)
		err := tx.Deserialize(bytes.NewReader(test.TX1RawBytes))
		require.NoError(t, err)
		err = msg.AddTransaction(tx)
		require.NoError(t, err)

		tx2 := wire.NewMsgTx(1)
		err = tx2.Deserialize(bytes.NewReader(test.TX2RawBytes))
		require.NoError(t, err)
		err = msg.AddTransaction(tx2)
		require.NoError(t, err)

		err = wire.WriteMessage(myConn, msg, wire.ProtocolVersion, wire.MainNet)
		require.NoError(t, err)

		time.Sleep(20 * time.Millisecond)

		blocks := peerHandler.GetBlock()
		require.Equal(t, 1, len(blocks))
		txs := peerHandler.GetBlockTransactions(0)
		assert.Equal(t, 2, len(txs))

		// read the transactions
		expectedTxBytes := []*chainhash.Hash{test.TX1Hash, test.TX2Hash}
		blockTransactions := peerHandler.GetBlockTransactions(0)
		for i, txMsg := range blockTransactions {
			assert.Equal(t, expectedTxBytes[i], txMsg)
		}
	})

	t.Run("read message - rejection", func(t *testing.T) {
		myConn, _, peerHandler := newTestPeer(t)

		msg := wire.NewMsgReject("block", wire.RejectDuplicate, "duplicate block")
		err := wire.WriteMessage(myConn, msg, wire.ProtocolVersion, wire.MainNet)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		rejections := peerHandler.GetTransactionRejection()
		require.Equal(t, 1, len(rejections))
		assert.Equal(t, "block", rejections[0].Cmd)
		assert.Equal(t, wire.RejectDuplicate, rejections[0].Code)
		assert.Equal(t, "duplicate block", rejections[0].Reason)
	})
}

func newTestPeer(t *testing.T) (net.Conn, *Peer, *MockPeerHandler) {
	peerConn, myConn := connutil.AsyncPipe()

	peerHandler := NewMockPeerHandler()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	p, err := NewPeer(
		logger,
		"MockPeerHandler:0000",
		peerHandler,
		wire.MainNet,
		WithDialer(func(network, address string) (net.Conn, error) {
			return peerConn, nil
		}),
	)
	require.NoError(t, err)

	doHandshake(t, p, myConn)

	// wait for the peer to be connected
	count := 0
	for {
		if p.Connected() {
			break
		}
		count++
		if count >= 3 {
			t.Error("peer not connected")
		}
		time.Sleep(10 * time.Millisecond)
	}

	return myConn, p, peerHandler
}

func newIncomingTestPeer(t *testing.T) (net.Conn, *Peer, *MockPeerHandler) {
	peerConn, myConn := connutil.AsyncPipe()
	peerHandler := NewMockPeerHandler()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	p, err := NewPeer(
		logger,
		"MockPeerHandler:0000",
		peerHandler,
		wire.MainNet,
		WithIncomingConnection(peerConn),
	)
	require.NoError(t, err)

	return myConn, p, peerHandler
}

func doHandshake(t *testing.T, p *Peer, myConn net.Conn) {
	// first thing we should receive is a version message
	msg, n, err := wire.ReadMessage(myConn, wire.ProtocolVersion, wire.MainNet)
	assert.NotEqual(t, 0, n)
	assert.NoError(t, err)
	vMsg := msg.(*wire.MsgVersion)
	assert.Equal(t, wire.CmdVersion, vMsg.Command())
	assert.Equal(t, int32(wire.ProtocolVersion), vMsg.ProtocolVersion)

	// write the version acknowledge message
	verackMsg := wire.NewMsgVerAck()
	err = wire.WriteMessage(myConn, verackMsg, wire.ProtocolVersion, wire.MainNet)
	require.NoError(t, err)

	// send our version message
	versionMsg := p.versionMessage("MockPeerHandler:0000")
	err = wire.WriteMessage(myConn, versionMsg, wire.ProtocolVersion, wire.MainNet)
	require.NoError(t, err)

	msg, n, err = wire.ReadMessage(myConn, wire.ProtocolVersion, wire.MainNet)
	assert.NotEqual(t, 0, n)
	assert.NoError(t, err)
	assert.Equal(t, wire.CmdVerAck, msg.Command())
}

func stopReadHandler(p *Peer) {
	if p.cancelReadHandler == nil {
		return
	}
	p.logger.Debug("Cancelling read handlers")
	p.cancelReadHandler()
	p.logger.Debug("Waiting for read handlers to stop")
	p.readerWg.Wait()
}

func stopWriteHandler(p *Peer) {
	if p.cancelWriteHandler == nil {
		return
	}
	p.logger.Debug("Cancelling write handlers")
	p.cancelWriteHandler()
	p.logger.Debug("Waiting for writer handlers to stop")
	p.writerWg.Wait()
}
