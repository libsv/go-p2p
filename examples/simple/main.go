package main

import (
	"os"
	"os/signal"

	"github.com/libsv/go-p2p"
	"github.com/libsv/go-p2p/wire"
	"github.com/ordishs/gocore"
)

func main() {
	logger := gocore.Log("simple-p2p-test")

	// Make sure to use the correct network here.
	//   For mainnet use wire.MainNet
	//   For testnet use wire.TestNet
	//   For regtest use wire.TestNet
	network := wire.TestNet

	peerManager := p2p.NewPeerManager(logger, network)

	peerHandler := SimplePeerHandler{
		logger: logger,
	}

	peer, err := p2p.NewPeer(logger, "localhost:58333", peerHandler, network)
	if err != nil {
		logger.Fatalf("failed to create peer: %s", err)
	}

	if err = peerManager.AddPeer(peer); err != nil {
		logger.Fatalf("failed to add peer: %s", err)
	}

	// setup signal catching
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	<-signalChan
	os.Exit(1)
}
