package p2p

import (
	"sort"
	"sync"
	"time"

	"github.com/libsv/go-p2p/wire"
	"github.com/ordishs/go-utils"
	"github.com/ordishs/gocore"
)

type PeerManager struct {
	mu         sync.RWMutex
	peers      map[string]PeerI
	network    wire.BitcoinNet
	batchDelay time.Duration
	logger     utils.Logger
}

var ebs int

func init() {
	ebs, _ = gocore.Config().GetInt("excessive_block_size", 4000000000)
	wire.SetLimits(uint64(ebs))
}

// NewPeerManager creates a new PeerManager
// messageCh is a channel that will be used to send messages from peers to the parent process
// this is used to pass INV messages from the bitcoin network peers to the parent process
// at the moment this is only used for Inv tx message for "seen", "sent" and "rejected" transactions
func NewPeerManager(logger utils.Logger, network wire.BitcoinNet, options ...PeerManagerOptions) PeerManagerI {
	logger.Infof("Excessive block size set to %d", ebs)

	pm := &PeerManager{
		peers:   make(map[string]PeerI),
		network: network,
		logger:  logger,
	}

	for _, option := range options {
		option(pm)
	}

	return pm
}

func (pm *PeerManager) AddPeer(peer PeerI) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer.Network() != pm.network {
		return ErrPeerNetworkMismatch
	}

	pm.peers[peer.String()] = peer

	return nil
}

func (pm *PeerManager) RemovePeer(peerURL string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.peers, peerURL)

	return nil
}

func (pm *PeerManager) GetPeers() []PeerI {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peers := make([]PeerI, 0, len(pm.peers))
	for _, peer := range pm.peers {
		peers = append(peers, peer)
	}

	return peers
}

// AnnounceTransaction will send an INV message to the provided peers or to selected peers if peers is nil
// it will return the peers that the transaction was actually announced to
func (pm *PeerManager) AnnounceTransaction(txID []byte, peers []PeerI) []PeerI {
	if len(peers) == 0 {
		peers = pm.GetAnnouncedPeers()
	}

	for _, peer := range peers {
		peer.AnnounceTransaction(txID)
	}

	return peers
}

func (pm *PeerManager) GetTransaction(txID []byte) {
	// send to the first found peer that is connected
	var sendToPeer PeerI
	for _, peer := range pm.GetAnnouncedPeers() {
		if peer.Connected() {
			sendToPeer = peer
			break
		}
	}

	sendToPeer.GetTransaction(txID)
}

func (pm *PeerManager) GetAnnouncedPeers() []PeerI {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Get a list of peers that are connected
	connectedPeers := make([]PeerI, 0, len(pm.peers))
	for _, peer := range pm.peers {
		if peer.Connected() {
			connectedPeers = append(connectedPeers, peer)
		}
	}

	// sort peers by address
	sort.SliceStable(connectedPeers, func(i, j int) bool {
		return connectedPeers[i].String() < connectedPeers[j].String()
	})

	// send to a subset of peers to be able to listen on the rest
	sendToPeers := make([]PeerI, 0, len(connectedPeers))
	for _, peer := range connectedPeers {

		if len(connectedPeers) > 1 && len(sendToPeers) >= (len(connectedPeers)+1)/2 {
			break
		}
		sendToPeers = append(sendToPeers, peer)
	}

	return sendToPeers
}
