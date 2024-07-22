package test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/libsv/go-p2p"

	"github.com/libsv/go-p2p/wire"
	"github.com/stretchr/testify/require"
)

func TestNewPeerManager(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	t.Run("announce transaction", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

		pm := p2p.NewPeerManager(logger, wire.TestNet)
		require.NotNil(t, pm)

		peerHandler := &p2p.PeerHandlerIMock{
			HandleTransactionsGetFunc: func(msgs []*wire.InvVect, peer p2p.PeerI) ([][]byte, error) {
				return [][]byte{TX1RawBytes}, nil
			},
		}

		peer, err := p2p.NewPeer(logger, "localhost:"+p2pPortBinding, peerHandler, wire.TestNet)
		require.NoError(t, err)

		err = pm.AddPeer(peer)
		require.NoError(t, err)

		t.Log("expect that peer has connected")
	connectLoop:
		for {
			select {
			case <-time.NewTicker(200 * time.Millisecond).C:
				if peer.Connected() {
					break connectLoop
				}
			case <-time.NewTimer(5 * time.Second).C:
				t.Fatal("peer did not disconnect")
			}
		}

		pm.AnnounceTransaction(TX1Hash, []p2p.PeerI{peer})

		time.Sleep(100 * time.Millisecond)

		peer.Shutdown()
	})
}
