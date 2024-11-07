package better_p2p

import "time"

type PeerOptions func(p *Peer)

func WithMaximumMessageSize(maximumMessageSize int64) PeerOptions {
	return func(p *Peer) {
		p.maxMsgSize = maximumMessageSize
	}
}

func WithUserAgent(userAgentName string, userAgentVersion string) PeerOptions {
	return func(p *Peer) {
		p.userAgentName = &userAgentName
		p.userAgentVersion = &userAgentVersion
	}
}

func WithNrOfWriteHandlers(n uint8) PeerOptions {
	return func(p *Peer) {
		p.nWriters = n
	}
}

func WithPingInterval(interval time.Duration, connectionHealthThreshold time.Duration) PeerOptions {
	return func(p *Peer) {
		p.pingInterval = interval
		p.healthThreshold = connectionHealthThreshold
	}
}
