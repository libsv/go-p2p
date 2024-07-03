// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package p2p

import (
	"github.com/libsv/go-p2p/wire"
	"sync"
)

// Ensure, that PeerHandlerIMock does implement PeerHandlerI.
// If this is not the case, regenerate this file with moq.
var _ PeerHandlerI = &PeerHandlerIMock{}

// PeerHandlerIMock is a mock implementation of PeerHandlerI.
//
//	func TestSomethingThatUsesPeerHandlerI(t *testing.T) {
//
//		// make and configure a mocked PeerHandlerI
//		mockedPeerHandlerI := &PeerHandlerIMock{
//			HandleBlockFunc: func(msg wire.Message, peer PeerI) error {
//				panic("mock out the HandleBlock method")
//			},
//			HandleBlockAnnouncementFunc: func(msg *wire.InvVect, peer PeerI) error {
//				panic("mock out the HandleBlockAnnouncement method")
//			},
//			HandleTransactionFunc: func(msg *wire.MsgTx, peer PeerI) error {
//				panic("mock out the HandleTransaction method")
//			},
//			HandleTransactionAnnouncementFunc: func(msg *wire.InvVect, peer PeerI) error {
//				panic("mock out the HandleTransactionAnnouncement method")
//			},
//			HandleTransactionRejectionFunc: func(rejMsg *wire.MsgReject, peer PeerI) error {
//				panic("mock out the HandleTransactionRejection method")
//			},
//			HandleTransactionSentFunc: func(msg *wire.MsgTx, peer PeerI) error {
//				panic("mock out the HandleTransactionSent method")
//			},
//			HandleTransactionsGetFunc: func(msg []*wire.InvVect, peer PeerI) ([][]byte, error) {
//				panic("mock out the HandleTransactionsGet method")
//			},
//		}
//
//		// use mockedPeerHandlerI in code that requires PeerHandlerI
//		// and then make assertions.
//
//	}
type PeerHandlerIMock struct {
	// HandleBlockFunc mocks the HandleBlock method.
	HandleBlockFunc func(msg wire.Message, peer PeerI) error

	// HandleBlockAnnouncementFunc mocks the HandleBlockAnnouncement method.
	HandleBlockAnnouncementFunc func(msg *wire.InvVect, peer PeerI) error

	// HandleTransactionFunc mocks the HandleTransaction method.
	HandleTransactionFunc func(msg *wire.MsgTx, peer PeerI) error

	// HandleTransactionAnnouncementFunc mocks the HandleTransactionAnnouncement method.
	HandleTransactionAnnouncementFunc func(msg *wire.InvVect, peer PeerI) error

	// HandleTransactionRejectionFunc mocks the HandleTransactionRejection method.
	HandleTransactionRejectionFunc func(rejMsg *wire.MsgReject, peer PeerI) error

	// HandleTransactionSentFunc mocks the HandleTransactionSent method.
	HandleTransactionSentFunc func(msg *wire.MsgTx, peer PeerI) error

	// HandleTransactionsGetFunc mocks the HandleTransactionsGet method.
	HandleTransactionsGetFunc func(msg []*wire.InvVect, peer PeerI) ([][]byte, error)

	// calls tracks calls to the methods.
	calls struct {
		// HandleBlock holds details about calls to the HandleBlock method.
		HandleBlock []struct {
			// Msg is the msg argument value.
			Msg wire.Message
			// Peer is the peer argument value.
			Peer PeerI
		}
		// HandleBlockAnnouncement holds details about calls to the HandleBlockAnnouncement method.
		HandleBlockAnnouncement []struct {
			// Msg is the msg argument value.
			Msg *wire.InvVect
			// Peer is the peer argument value.
			Peer PeerI
		}
		// HandleTransaction holds details about calls to the HandleTransaction method.
		HandleTransaction []struct {
			// Msg is the msg argument value.
			Msg *wire.MsgTx
			// Peer is the peer argument value.
			Peer PeerI
		}
		// HandleTransactionAnnouncement holds details about calls to the HandleTransactionAnnouncement method.
		HandleTransactionAnnouncement []struct {
			// Msg is the msg argument value.
			Msg *wire.InvVect
			// Peer is the peer argument value.
			Peer PeerI
		}
		// HandleTransactionRejection holds details about calls to the HandleTransactionRejection method.
		HandleTransactionRejection []struct {
			// RejMsg is the rejMsg argument value.
			RejMsg *wire.MsgReject
			// Peer is the peer argument value.
			Peer PeerI
		}
		// HandleTransactionSent holds details about calls to the HandleTransactionSent method.
		HandleTransactionSent []struct {
			// Msg is the msg argument value.
			Msg *wire.MsgTx
			// Peer is the peer argument value.
			Peer PeerI
		}
		// HandleTransactionsGet holds details about calls to the HandleTransactionsGet method.
		HandleTransactionsGet []struct {
			// Msg is the msg argument value.
			Msg []*wire.InvVect
			// Peer is the peer argument value.
			Peer PeerI
		}
	}
	lockHandleBlock                   sync.RWMutex
	lockHandleBlockAnnouncement       sync.RWMutex
	lockHandleTransaction             sync.RWMutex
	lockHandleTransactionAnnouncement sync.RWMutex
	lockHandleTransactionRejection    sync.RWMutex
	lockHandleTransactionSent         sync.RWMutex
	lockHandleTransactionsGet         sync.RWMutex
}

// HandleBlock calls HandleBlockFunc.
func (mock *PeerHandlerIMock) HandleBlock(msg wire.Message, peer PeerI) error {
	if mock.HandleBlockFunc == nil {
		panic("PeerHandlerIMock.HandleBlockFunc: method is nil but PeerHandlerI.HandleBlock was just called")
	}
	callInfo := struct {
		Msg  wire.Message
		Peer PeerI
	}{
		Msg:  msg,
		Peer: peer,
	}
	mock.lockHandleBlock.Lock()
	mock.calls.HandleBlock = append(mock.calls.HandleBlock, callInfo)
	mock.lockHandleBlock.Unlock()
	return mock.HandleBlockFunc(msg, peer)
}

// HandleBlockCalls gets all the calls that were made to HandleBlock.
// Check the length with:
//
//	len(mockedPeerHandlerI.HandleBlockCalls())
func (mock *PeerHandlerIMock) HandleBlockCalls() []struct {
	Msg  wire.Message
	Peer PeerI
} {
	var calls []struct {
		Msg  wire.Message
		Peer PeerI
	}
	mock.lockHandleBlock.RLock()
	calls = mock.calls.HandleBlock
	mock.lockHandleBlock.RUnlock()
	return calls
}

// HandleBlockAnnouncement calls HandleBlockAnnouncementFunc.
func (mock *PeerHandlerIMock) HandleBlockAnnouncement(msg *wire.InvVect, peer PeerI) error {
	if mock.HandleBlockAnnouncementFunc == nil {
		panic("PeerHandlerIMock.HandleBlockAnnouncementFunc: method is nil but PeerHandlerI.HandleBlockAnnouncement was just called")
	}
	callInfo := struct {
		Msg  *wire.InvVect
		Peer PeerI
	}{
		Msg:  msg,
		Peer: peer,
	}
	mock.lockHandleBlockAnnouncement.Lock()
	mock.calls.HandleBlockAnnouncement = append(mock.calls.HandleBlockAnnouncement, callInfo)
	mock.lockHandleBlockAnnouncement.Unlock()
	return mock.HandleBlockAnnouncementFunc(msg, peer)
}

// HandleBlockAnnouncementCalls gets all the calls that were made to HandleBlockAnnouncement.
// Check the length with:
//
//	len(mockedPeerHandlerI.HandleBlockAnnouncementCalls())
func (mock *PeerHandlerIMock) HandleBlockAnnouncementCalls() []struct {
	Msg  *wire.InvVect
	Peer PeerI
} {
	var calls []struct {
		Msg  *wire.InvVect
		Peer PeerI
	}
	mock.lockHandleBlockAnnouncement.RLock()
	calls = mock.calls.HandleBlockAnnouncement
	mock.lockHandleBlockAnnouncement.RUnlock()
	return calls
}

// HandleTransaction calls HandleTransactionFunc.
func (mock *PeerHandlerIMock) HandleTransaction(msg *wire.MsgTx, peer PeerI) error {
	if mock.HandleTransactionFunc == nil {
		panic("PeerHandlerIMock.HandleTransactionFunc: method is nil but PeerHandlerI.HandleTransaction was just called")
	}
	callInfo := struct {
		Msg  *wire.MsgTx
		Peer PeerI
	}{
		Msg:  msg,
		Peer: peer,
	}
	mock.lockHandleTransaction.Lock()
	mock.calls.HandleTransaction = append(mock.calls.HandleTransaction, callInfo)
	mock.lockHandleTransaction.Unlock()
	return mock.HandleTransactionFunc(msg, peer)
}

// HandleTransactionCalls gets all the calls that were made to HandleTransaction.
// Check the length with:
//
//	len(mockedPeerHandlerI.HandleTransactionCalls())
func (mock *PeerHandlerIMock) HandleTransactionCalls() []struct {
	Msg  *wire.MsgTx
	Peer PeerI
} {
	var calls []struct {
		Msg  *wire.MsgTx
		Peer PeerI
	}
	mock.lockHandleTransaction.RLock()
	calls = mock.calls.HandleTransaction
	mock.lockHandleTransaction.RUnlock()
	return calls
}

// HandleTransactionAnnouncement calls HandleTransactionAnnouncementFunc.
func (mock *PeerHandlerIMock) HandleTransactionAnnouncement(msg *wire.InvVect, peer PeerI) error {
	if mock.HandleTransactionAnnouncementFunc == nil {
		panic("PeerHandlerIMock.HandleTransactionAnnouncementFunc: method is nil but PeerHandlerI.HandleTransactionAnnouncement was just called")
	}
	callInfo := struct {
		Msg  *wire.InvVect
		Peer PeerI
	}{
		Msg:  msg,
		Peer: peer,
	}
	mock.lockHandleTransactionAnnouncement.Lock()
	mock.calls.HandleTransactionAnnouncement = append(mock.calls.HandleTransactionAnnouncement, callInfo)
	mock.lockHandleTransactionAnnouncement.Unlock()
	return mock.HandleTransactionAnnouncementFunc(msg, peer)
}

// HandleTransactionAnnouncementCalls gets all the calls that were made to HandleTransactionAnnouncement.
// Check the length with:
//
//	len(mockedPeerHandlerI.HandleTransactionAnnouncementCalls())
func (mock *PeerHandlerIMock) HandleTransactionAnnouncementCalls() []struct {
	Msg  *wire.InvVect
	Peer PeerI
} {
	var calls []struct {
		Msg  *wire.InvVect
		Peer PeerI
	}
	mock.lockHandleTransactionAnnouncement.RLock()
	calls = mock.calls.HandleTransactionAnnouncement
	mock.lockHandleTransactionAnnouncement.RUnlock()
	return calls
}

// HandleTransactionRejection calls HandleTransactionRejectionFunc.
func (mock *PeerHandlerIMock) HandleTransactionRejection(rejMsg *wire.MsgReject, peer PeerI) error {
	if mock.HandleTransactionRejectionFunc == nil {
		panic("PeerHandlerIMock.HandleTransactionRejectionFunc: method is nil but PeerHandlerI.HandleTransactionRejection was just called")
	}
	callInfo := struct {
		RejMsg *wire.MsgReject
		Peer   PeerI
	}{
		RejMsg: rejMsg,
		Peer:   peer,
	}
	mock.lockHandleTransactionRejection.Lock()
	mock.calls.HandleTransactionRejection = append(mock.calls.HandleTransactionRejection, callInfo)
	mock.lockHandleTransactionRejection.Unlock()
	return mock.HandleTransactionRejectionFunc(rejMsg, peer)
}

// HandleTransactionRejectionCalls gets all the calls that were made to HandleTransactionRejection.
// Check the length with:
//
//	len(mockedPeerHandlerI.HandleTransactionRejectionCalls())
func (mock *PeerHandlerIMock) HandleTransactionRejectionCalls() []struct {
	RejMsg *wire.MsgReject
	Peer   PeerI
} {
	var calls []struct {
		RejMsg *wire.MsgReject
		Peer   PeerI
	}
	mock.lockHandleTransactionRejection.RLock()
	calls = mock.calls.HandleTransactionRejection
	mock.lockHandleTransactionRejection.RUnlock()
	return calls
}

// HandleTransactionSent calls HandleTransactionSentFunc.
func (mock *PeerHandlerIMock) HandleTransactionSent(msg *wire.MsgTx, peer PeerI) error {
	if mock.HandleTransactionSentFunc == nil {
		panic("PeerHandlerIMock.HandleTransactionSentFunc: method is nil but PeerHandlerI.HandleTransactionSent was just called")
	}
	callInfo := struct {
		Msg  *wire.MsgTx
		Peer PeerI
	}{
		Msg:  msg,
		Peer: peer,
	}
	mock.lockHandleTransactionSent.Lock()
	mock.calls.HandleTransactionSent = append(mock.calls.HandleTransactionSent, callInfo)
	mock.lockHandleTransactionSent.Unlock()
	return mock.HandleTransactionSentFunc(msg, peer)
}

// HandleTransactionSentCalls gets all the calls that were made to HandleTransactionSent.
// Check the length with:
//
//	len(mockedPeerHandlerI.HandleTransactionSentCalls())
func (mock *PeerHandlerIMock) HandleTransactionSentCalls() []struct {
	Msg  *wire.MsgTx
	Peer PeerI
} {
	var calls []struct {
		Msg  *wire.MsgTx
		Peer PeerI
	}
	mock.lockHandleTransactionSent.RLock()
	calls = mock.calls.HandleTransactionSent
	mock.lockHandleTransactionSent.RUnlock()
	return calls
}

// HandleTransactionsGet calls HandleTransactionsGetFunc.
func (mock *PeerHandlerIMock) HandleTransactionsGet(msg []*wire.InvVect, peer PeerI) ([][]byte, error) {
	if mock.HandleTransactionsGetFunc == nil {
		panic("PeerHandlerIMock.HandleTransactionsGetFunc: method is nil but PeerHandlerI.HandleTransactionsGet was just called")
	}
	callInfo := struct {
		Msg  []*wire.InvVect
		Peer PeerI
	}{
		Msg:  msg,
		Peer: peer,
	}
	mock.lockHandleTransactionsGet.Lock()
	mock.calls.HandleTransactionsGet = append(mock.calls.HandleTransactionsGet, callInfo)
	mock.lockHandleTransactionsGet.Unlock()
	return mock.HandleTransactionsGetFunc(msg, peer)
}

// HandleTransactionsGetCalls gets all the calls that were made to HandleTransactionsGet.
// Check the length with:
//
//	len(mockedPeerHandlerI.HandleTransactionsGetCalls())
func (mock *PeerHandlerIMock) HandleTransactionsGetCalls() []struct {
	Msg  []*wire.InvVect
	Peer PeerI
} {
	var calls []struct {
		Msg  []*wire.InvVect
		Peer PeerI
	}
	mock.lockHandleTransactionsGet.RLock()
	calls = mock.calls.HandleTransactionsGet
	mock.lockHandleTransactionsGet.RUnlock()
	return calls
}
