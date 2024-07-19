package test

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/libsv/go-p2p"

	"github.com/libsv/go-p2p/wire"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
)

const (
	p2pPortBinding = "18335"
)

//go:generate moq -out ../peer_handler_gen_mock.go ../ PeerHandlerI

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
				Source: fmt.Sprintf("%s/bitcoin.conf", pwd),
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
		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

		peerHandler := p2p.NewMockPeerHandler()

		peer, err := p2p.NewPeer(logger, "localhost:"+p2pPortBinding, peerHandler, wire.TestNet, p2p.WithUserAgent("agent", "0.0.1"))
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
				t.Fatal("peer did not connect")
			}
		}

		dockerClient := pool.Client

		t.Log("restart container and break connection")
		err = dockerClient.RestartContainer(resource.Container.ID, 10)
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

		require.NoError(t, err)

		t.Log("shutdown")
		peer.Shutdown()
		t.Log("shutdown finished")
	})

}
