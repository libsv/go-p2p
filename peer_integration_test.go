package p2p

import (
	"encoding/hex"
	"fmt"
	"github.com/libsv/go-p2p/chaincfg/chainhash"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/libsv/go-p2p/wire"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
)

const (
	p2pPortBinding = "18335"
)

//go:generate moq -out ./peer_handler_gen_mock.go . PeerHandlerI

var (
	pool     *dockertest.Pool
	resource *dockertest.Resource
	pwd      string

	TX1            = "b042f298deabcebbf15355aa3a13c7d7cfe96c44ac4f492735f936f8e50d06f6"
	TX1Hash, _     = chainhash.NewHashFromStr(TX1)
	TX1Raw         = "01000000010000000000000000000000000000000000000000000000000000000000000000ffffffff1a0386c40b2f7461616c2e636f6d2f00cf47ad9c7af83836000000ffffffff0117564425000000001976a914522cf9e7626d9bd8729e5a1398ece40dad1b6a2f88ac00000000"
	TX1RawBytes, _ = hex.DecodeString(TX1Raw)
)

func TestMain(m *testing.M) {
	var err error

	pool, err = dockertest.NewPool("")
	if err != nil {
		log.Fatalf("failed to create pool: %v", err)
	}

	const p2pPort = "18333"
	pwd, err = os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %s", err)
	}

	resource, err = pool.RunWithOptions(&dockertest.RunOptions{
		Repository:   "bitcoinsv/bitcoin-sv",
		Tag:          "1.1.0",
		Env:          []string{},
		ExposedPorts: []string{p2pPortBinding, p2pPort},
		PortBindings: map[docker.Port][]docker.PortBinding{
			p2pPort: {
				{HostIP: "0.0.0.0", HostPort: p2pPortBinding},
			},
		},
		Cmd:  []string{"/entrypoint.sh", "bitcoind"},
		Name: "node",
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
		config.Mounts = []docker.HostMount{
			{
				Target: "/data/bitcoin.conf",
				Source: fmt.Sprintf("%s/peer_integration_test_config/bitcoin.conf", pwd),
				Type:   "bind",
			},
		}
	})
	if err != nil {
		log.Fatalf("failed to create resource: %v", err)
	}

	code := m.Run()

	err = pool.Purge(resource)
	if err != nil {
		log.Fatalf("failed to purge pool: %v", err)
	}

	os.Exit(code)
}

func TestNewPeer(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	t.Run("break and re-establish peer connection", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

		peerHandler := NewMockPeerHandler()

		time.Sleep(5 * time.Second)

		peer, err := NewPeer(logger, "localhost:"+p2pPortBinding, peerHandler, wire.TestNet, WithUserAgent("agent", "0.0.1"))
		require.NoError(t, err)

		time.Sleep(5 * time.Second)

		require.True(t, peer.Connected())

		dockerClient := pool.Client

		// restart container and break connection
		err = dockerClient.RestartContainer(resource.Container.ID, 10)
		require.NoError(t, err)

		time.Sleep(6 * time.Second)

		// expect that peer has disconnected
		require.False(t, peer.Connected())

		// wait longer than the reconnect interval and expect that peer has re-established connection
		time.Sleep(reconnectInterval + 2*time.Second)
		require.True(t, peer.Connected())

		require.NoError(t, err)
		peer.Shutdown()
	})

	t.Run("announce transaction", func(t *testing.T) {

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

		pm := NewPeerManager(logger, wire.TestNet)
		require.NotNil(t, pm)

		peerHandler := &PeerHandlerIMock{
			HandleTransactionGetFunc: func(msg *wire.InvVect, peer PeerI) ([]byte, error) {
				return TX1RawBytes, nil
			},
		}

		time.Sleep(5 * time.Second)

		peer, err := NewPeer(logger, "localhost:"+p2pPortBinding, peerHandler, wire.TestNet)
		require.NoError(t, err)

		err = pm.AddPeer(peer)
		require.NoError(t, err)

		time.Sleep(5 * time.Second)

		require.True(t, peer.Connected())

		pm.AnnounceTransaction(TX1Hash, []PeerI{peer})

		time.Sleep(10 * time.Second)

		peer.Shutdown()
	})
}
