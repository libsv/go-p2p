package better_p2p

import (
	"github.com/libsv/go-p2p/wire"
)

//go:generate moq -pkg mocks -out ./mocks/peer_mock.go . PeerI
//go:generate moq -pkg mocks -out ./mocks/message_handler_mock.go . MessageHandlerI

type PeerI interface {
	Restart() (ok bool)
	Shutdown()
	Connected() bool
	IsUnhealthyCh() <-chan struct{}
	WriteMsg(msg wire.Message)
	Network() wire.BitcoinNet
	String() string
}

type MessageHandlerI interface {
	// should be fire & forget
	OnReceive(msg wire.Message, peer PeerI)
	// should be fire & forget
	OnSend(msg wire.Message, peer PeerI)
}
