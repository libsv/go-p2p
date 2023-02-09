package p2p

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
	AnnouncedTransactions [][]byte
	RequestTransactions   [][]byte
	AnnouncedBlocks       [][]byte
	RequestBlocks         [][]byte
	peerCreator           func(peerAddress string, peerHandler PeerHandlerI) (PeerI, error)
}

func NewPeerManagerMock() *PeerManagerMock {
	return &PeerManagerMock{
		Peers: make(map[string]PeerI),
	}
}

func (p *PeerManagerMock) RequestTransaction(txID []byte) PeerI {
	p.RequestTransactions = append(p.RequestTransactions, txID)
	return nil
}

func (p *PeerManagerMock) AnnounceTransaction(txID []byte, _ []PeerI) []PeerI {
	p.AnnouncedTransactions = append(p.AnnouncedTransactions, txID)
	return nil
}

func (p *PeerManagerMock) AnnounceBlock(blockHash []byte, _ []PeerI) []PeerI {
	p.AnnouncedBlocks = append(p.AnnouncedBlocks, blockHash)
	return nil
}

func (p *PeerManagerMock) RequestBlock(blockHash []byte) PeerI {
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
