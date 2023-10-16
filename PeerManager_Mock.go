package p2p

import (
	"context"
	"log/slog"

	"github.com/libsv/go-p2p/chaincfg/chainhash"
)

type TestLogger struct {
}

func (h *TestLogger) Enabled(_ context.Context, _ slog.Level) bool {
	return false
}

func (h *TestLogger) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

func (h *TestLogger) WithAttrs(_ []slog.Attr) slog.Handler {
	return &TestLogger{}
}

func (h *TestLogger) WithGroup(_ string) slog.Handler {
	return &TestLogger{}
}

func (h *TestLogger) Handler() slog.Handler {
	return &TestLogger{}
}

type PeerManagerMock struct {
	Peers                 []PeerI
	AnnouncedTransactions []*chainhash.Hash
	RequestTransactions   []*chainhash.Hash
	AnnouncedBlocks       []*chainhash.Hash
	RequestBlocks         []*chainhash.Hash
	peerCreator           func(peerAddress string, peerHandler PeerHandlerI) (PeerI, error)
}

func NewPeerManagerMock() *PeerManagerMock {
	return &PeerManagerMock{
		Peers:                 make([]PeerI, 0),
		AnnouncedTransactions: make([]*chainhash.Hash, 0),
		RequestTransactions:   make([]*chainhash.Hash, 0),
		AnnouncedBlocks:       make([]*chainhash.Hash, 0),
		RequestBlocks:         make([]*chainhash.Hash, 0),
	}
}

func (p *PeerManagerMock) RequestTransaction(hash *chainhash.Hash) PeerI {
	p.RequestTransactions = append(p.RequestTransactions, hash)
	return nil
}

func (p *PeerManagerMock) AnnounceTransaction(hash *chainhash.Hash, _ []PeerI) []PeerI {
	p.AnnouncedTransactions = append(p.AnnouncedTransactions, hash)
	return nil
}

func (p *PeerManagerMock) AnnounceBlock(blockHash *chainhash.Hash, _ []PeerI) []PeerI {
	p.AnnouncedBlocks = append(p.AnnouncedBlocks, blockHash)
	return nil
}

func (p *PeerManagerMock) RequestBlock(blockHash *chainhash.Hash) PeerI {
	p.RequestBlocks = append(p.RequestBlocks, blockHash)
	return nil
}

func (p *PeerManagerMock) PeerCreator(peerCreator func(peerAddress string, peerHandler PeerHandlerI) (PeerI, error)) {
	p.peerCreator = peerCreator
}

func (p *PeerManagerMock) AddPeer(peer PeerI) error {
	p.Peers = append(p.Peers, peer)
	return nil
}

func (p *PeerManagerMock) GetPeers() []PeerI {
	peers := make([]PeerI, 0, len(p.Peers))
	peers = append(peers, p.Peers...)

	return peers
}
