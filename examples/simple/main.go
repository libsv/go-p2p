package main

import (
	"log/slog"
	"os"
	"os/signal"

	"github.com/libsv/go-p2p"
	"github.com/libsv/go-p2p/wire"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Make sure to use the correct network here.
	//   For mainnet use wire.MainNet
	//   For testnet use wire.TestNet
	//   For regtest use wire.TestNet
	network := wire.TestNet

	peerManager := p2p.NewPeerManager(logger, network)

	peerHandler := &SimplePeerHandler{
		logger: logger,
	}

	peer, err := p2p.NewPeer("localhost:58333", peerHandler, network, p2p.WithLogger(logger))
	if err != nil {
		logger.Error("failed to create peer", slog.String("err", err.Error()))
	}

	if err = peerManager.AddPeer(peer); err != nil {
		logger.Error("failed to add peer", slog.String("err", err.Error()))
	}

	// setup signal catching
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	<-signalChan
	os.Exit(1)
}
