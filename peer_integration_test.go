package p2p

import (
	"fmt"
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

var (
	pool     *dockertest.Pool
	resource *dockertest.Resource
	pwd      string
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

		pm := NewPeerManager(logger, wire.TestNet)
		require.NotNil(t, pm)

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
}
