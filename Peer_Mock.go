package p2p

import (
	"sync"

	"github.com/libsv/go-p2p/wire"
)

type PeerMock struct {
	mu                  sync.Mutex
	address             string
	peerHandler         PeerHandlerI
	network             wire.BitcoinNet
	writeChan           chan wire.Message
	messages            []wire.Message
	announcements       [][]byte
	requestTransactions [][]byte
	announceBlocks      [][]byte
	requestBlocks       [][]byte
}

func NewPeerMock(address string, peerHandler PeerHandlerI, network wire.BitcoinNet) (*PeerMock, error) {
	writeChan := make(chan wire.Message)

	p := &PeerMock{
		peerHandler: peerHandler,
		address:     address,
		network:     network,
		writeChan:   writeChan,
	}

	go func() {
		for msg := range writeChan {
			p.message(msg)
		}
	}()

	return p, nil
}

func (p *PeerMock) Network() wire.BitcoinNet {
	return p.network
}

func (p *PeerMock) Connected() bool {
	return true
}

func (p *PeerMock) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.messages)
}

func (p *PeerMock) AnnounceTransaction(txHash []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.announcements = append(p.announcements, txHash)
}

func (p *PeerMock) GetAnnouncements() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.announcements
}

func (p *PeerMock) RequestTransaction(txHash []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.requestTransactions = append(p.requestTransactions, txHash)
}

func (p *PeerMock) GetRequestTransactions() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.requestTransactions
}

func (p *PeerMock) AnnounceBlock(blockHash []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.announceBlocks = append(p.announceBlocks, blockHash)
}

func (p *PeerMock) GetAnnounceBlocks() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.announceBlocks
}

func (p *PeerMock) RequestBlock(blockHash []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.requestBlocks = append(p.requestBlocks, blockHash)
}

func (p *PeerMock) GetRequestBlocks() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.requestBlocks
}

func (p *PeerMock) message(msg wire.Message) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.messages = append(p.messages, msg)
}

// func (p *PeerMock) getMessages() []wire.Message {
// 	p.mu.Lock()
// 	defer p.mu.Unlock()

// 	return p.messages
// }

func (p *PeerMock) WriteMsg(msg wire.Message) error {
	p.writeChan <- msg
	return nil
}

func (p *PeerMock) String() string {
	return p.address
}
