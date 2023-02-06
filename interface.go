package p2p

import (
	"fmt"

	"github.com/libsv/go-p2p/wire"
)

type Status int32

var (
	StatusSent     Status = 5
	StatusSeen     Status = 6
	StatusRejected Status = 109
)

var (
	ErrPeerNetworkMismatch = fmt.Errorf("peer network mismatch")
)

type PeerManagerI interface {
	AnnounceTransaction(txID []byte, peers []PeerI) []PeerI
	GetTransaction(txID []byte)
	AddPeer(peer PeerI) error
	RemovePeer(peerURL string) error
	GetPeers() []PeerI
}

type PeerI interface {
	Connected() bool
	WriteMsg(msg wire.Message) error
	String() string
	AnnounceTransaction(txID []byte)
	GetTransaction(txID []byte)
	Network() wire.BitcoinNet
}

type PeerHandlerI interface {
	HandleTransactionGet(msg *wire.InvVect, peer PeerI) ([]byte, error)
	HandleTransactionSent(msg *wire.MsgTx, peer PeerI) error
	HandleTransactionAnnouncement(msg *wire.InvVect, peer PeerI) error
	HandleTransactionRejection(rejMsg *wire.MsgReject, peer PeerI) error
	HandleTransaction(msg *wire.MsgTx, peer PeerI) error
	HandleBlockAnnouncement(msg *wire.InvVect, peer PeerI) error
	HandleBlock(msg *BlockMessage, peer PeerI) error
}
