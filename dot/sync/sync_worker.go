package sync

import (
	"context"
	"sync"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/libp2p/go-libp2p/core/peer"
)

// syncWorker represents a available peer that could be a source
// for requesting blocks, once a peer is disconnected or is ignored
// we can just disable its worker.
type syncWorker struct {
	// context shared between all workers
	ctx context.Context
	l   sync.RWMutex

	doneCh chan struct{}
	stopCh chan struct{}

	who        peer.ID
	network    Network
	bestHash   common.Hash
	bestNumber uint
}

func newSyncWorker(ctx context.Context, who peer.ID,
	bestHash common.Hash, bestNumber uint, network Network) *syncWorker {
	return &syncWorker{
		ctx:        ctx,
		who:        who,
		bestHash:   bestHash,
		bestNumber: bestNumber,
		network:    network,
		doneCh:     make(chan struct{}),
		stopCh:     make(chan struct{}),
	}
}

func (s *syncWorker) update(bestHash common.Hash, bestNumber uint) {
	s.l.Lock()
	defer s.l.Unlock()

	s.bestHash = bestHash
	s.bestNumber = bestNumber
}

func (s *syncWorker) Start(tasks chan *syncTask, wg *sync.WaitGroup) {
	wg.Add(1)

	go func() {
		defer func() {
			wg.Done()
			close(s.doneCh)
			logger.Infof("[SHUTDOWN] worker %s", s.who)
		}()

		logger.Infof("worker %s started, waiting for tasks...", s.who)

		for {
			select {
			case <-s.stopCh:
				return

			case task := <-tasks:
				request := task.request
				logger.Infof("[EXECUTING] worker %s: block request: %s", s.who, request)

				response, err := s.network.DoBlockRequest(s.who, request)
				if err != nil {
					logger.Infof("[FINISHED] worker %s: err: %s", s.who, err)
				} else if response != nil {
					logger.Infof("[FINISHED] worker %s: block data amount: %d", s.who, len(response.BlockData))
				}

				task.resultCh <- &syncTaskResult{
					who:      s.who,
					request:  request,
					response: response,
					err:      err,
				}
			}
		}
	}()
}

func (s *syncWorker) Stop() {
	close(s.stopCh)
	<-s.doneCh
}