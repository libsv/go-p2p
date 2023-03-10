package p2p

import "github.com/libsv/go-p2p/chaincfg/chainhash"

type TestLogger struct{}

func (l TestLogger) LogLevel() int {
	return 0
}
func (l TestLogger) Debugf(format string, args ...interface{}) {}
func (l TestLogger) Infof(format string, args ...interface{})  {}
func (l TestLogger) Warnf(format string, args ...interface{})  {}
func (l TestLogger) Errorf(format string, args ...interface{}) {}
func (l TestLogger) Fatalf(format string, args ...interface{}) {}

type PeerManagerMock struct {
	Peers                 map[string]PeerI
	AnnouncedTransactions []*chainhash.Hash
	RequestTransactions   []*chainhash.Hash
	AnnouncedBlocks       []*chainhash.Hash
	RequestBlocks         []*chainhash.Hash
	peerCreator           func(peerAddress string, peerHandler PeerHandlerI) (PeerI, error)
}

func NewPeerManagerMock() *PeerManagerMock {
	return &PeerManagerMock{
		Peers: make(map[string]PeerI),
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
	p.Peers[peer.String()] = peer
	return nil
}

func (p *PeerManagerMock) RemovePeer(peerURL string) error {
	delete(p.Peers, peerURL)
	return nil
}

func (p *PeerManagerMock) GetPeers() []PeerI {
	peers := make([]PeerI, 0, len(p.Peers))
	for _, peer := range p.Peers {
		peers = append(peers, peer)
	}
	return peers
}
