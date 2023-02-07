package main

import (
	"fmt"

	"github.com/libsv/go-p2p"
	"github.com/libsv/go-p2p/wire"
	"github.com/ordishs/go-utils"
)

// SimplePeerHandler is a simple implementation of the PeerHandler interface.
// This is how you can customize the behaviour of the peer.
type SimplePeerHandler struct {
	logger utils.Logger
}

func (s *SimplePeerHandler) HandleTransactionGet(msg *wire.InvVect, peer p2p.PeerI) ([]byte, error) {
	s.logger.Infof("Peer %s requested transaction %s", peer.String(), msg.Hash.String())
	// You should implement a store and return the transaction bytes here.
	return nil, fmt.Errorf("transaction not found")
}

func (s *SimplePeerHandler) HandleTransactionSent(msg *wire.MsgTx, peer p2p.PeerI) error {
	s.logger.Infof("Sent transaction %s to peer %s", msg.TxHash().String(), peer.String())
	// This is called when a transaction is sent to a peer. You could save this to your store here.
	return nil
}

func (s *SimplePeerHandler) HandleTransactionAnnouncement(msg *wire.InvVect, peer p2p.PeerI) error {
	s.logger.Infof("Peer %s announced transaction %s", peer.String(), msg.Hash.String())
	// This is called when a transaction is announced by a peer. Handle this as you wish.
	return nil
}

func (s *SimplePeerHandler) HandleTransactionRejection(rejMsg *wire.MsgReject, peer p2p.PeerI) error {
	s.logger.Infof("Peer %s rejected transaction %s", peer.String(), rejMsg.Hash.String())
	// This is called when a transaction is rejected by a peer. Handle this as you wish.
	return nil
}

func (s *SimplePeerHandler) HandleTransaction(msg *wire.MsgTx, peer p2p.PeerI) error {
	s.logger.Infof("Received transaction %s from peer %s", msg.TxHash().String(), peer.String())
	// This is called when a transaction is received from a peer. Handle this as you wish.
	return nil
}

func (s *SimplePeerHandler) HandleBlockAnnouncement(msg *wire.InvVect, peer p2p.PeerI) error {
	s.logger.Infof("Peer %s announced block %s", peer.String(), msg.Hash.String())
	// This is called when a block is announced by a peer. Handle this as you wish.
	return nil
}

func (s *SimplePeerHandler) HandleBlock(msg *p2p.BlockMessage, peer p2p.PeerI) error {
	s.logger.Infof("Received block %s from peer %s", msg.Header.BlockHash().String(), peer.String())
	// This is called when a block is received from a peer. Handle this as you wish.
	// note: the block message is a custom BlockMessage and not a wire.MsgBlock
	return nil
}
