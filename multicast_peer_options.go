package p2p

import "github.com/libsv/go-p2p/wire"

type McastPeerOptions func(p *McastPeer)

func McastWithMaximumMessageSize(n int64) McastPeerOptions {
	return func(p *McastPeer) {
		p.maxMsgSize = n
	}
}

func McastWithNrOfWriteHandlers(n uint8) McastPeerOptions {
	return func(p *McastPeer) {
		p.nWriters = n
	}
}

func McastWithWriteMsgBufforSize(n int) McastPeerOptions {
	return func(p *McastPeer) {
		p.writeCh = make(chan wire.Message, n)
	}
}
