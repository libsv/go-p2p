package p2p

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/libsv/go-p2p/chaincfg/chainhash"
)

// BatchProcessor for chain hashes runs a specified function on a batch of chain hashes in specified intervals or if the batch has reached a specified size
type BatchProcessor struct {
	fn          func([]*chainhash.Hash)
	batchSize   int
	runInterval time.Duration
	batch       []*chainhash.Hash
	hashChannel chan *chainhash.Hash
	wg          *sync.WaitGroup
	cancelAll   context.CancelFunc
	ctx         context.Context
}

func NewBatchProcessor(batchSize int, runInterval time.Duration, fn func(batch []*chainhash.Hash), bufferSize int) (*BatchProcessor, error) {
	if batchSize == 0 {
		return nil, errors.New("batch size must be greater than zero")
	}

	b := &BatchProcessor{
		fn:          fn,
		batchSize:   batchSize,
		runInterval: runInterval,
		hashChannel: make(chan *chainhash.Hash, bufferSize),
		wg:          &sync.WaitGroup{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.ctx = ctx
	b.cancelAll = cancel

	b.start()

	return b, nil
}

func (b *BatchProcessor) Put(item *chainhash.Hash) {
	b.hashChannel <- item
}

func (b *BatchProcessor) Shutdown() {
	if b.cancelAll != nil {
		b.cancelAll()
		b.wg.Wait()
	}
}

func (b *BatchProcessor) runFunction() {
	copyBatch := make([]*chainhash.Hash, len(b.batch))

	copy(copyBatch, b.batch)

	b.batch = b.batch[:0] // Clear the batch slice without reallocating the underlying memory

	go b.fn(copyBatch)
}

func (b *BatchProcessor) start() {
	runTicker := time.NewTicker(b.runInterval)
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case <-b.ctx.Done():
				runTicker.Stop()
				return
			case item := <-b.hashChannel:
				b.batch = append(b.batch, item)

				if len(b.batch) >= b.batchSize {
					b.runFunction()
				}

			case <-runTicker.C:
				if len(b.batch) > 0 {
					b.runFunction()
				}
			}
		}
	}()
}
