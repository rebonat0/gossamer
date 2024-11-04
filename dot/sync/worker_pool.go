// Copyright 2024 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package sync

import (
	"context"
	"errors"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

const workerPoolCapacity = 100

var (
	ErrPeerIgnored = errors.New("peer ignored")
)

type TaskID string
type Result any

type Task interface {
	ID() TaskID
	String() string
	Do(p peer.ID) (Result, error)
}

type TaskResult struct {
	Task   Task
	Result Result
	Who    peer.ID
}

type WorkerPool interface {
	SubmitBatch(tasks []Task) error
	Results() chan TaskResult
	AddPeer(p peer.ID)
	IgnorePeer(p peer.ID)
	IdlePeers() int
	Shutdown()
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool() WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	return &workerPool{
		peers:        make(chan peer.ID, workerPoolCapacity),
		ignoredPeers: make(map[peer.ID]struct{}),
		ctx:          ctx,
		cancel:       cancel,
	}
}

type workerPool struct {
	mtx sync.RWMutex
	wg  sync.WaitGroup

	peers        chan peer.ID // TODO figure out how to avoid duplicates
	ignoredPeers map[peer.ID]struct{}
	resChan      chan TaskResult
	ctx          context.Context
	cancel       context.CancelFunc
}

// SubmitBatch accepts a list of tasks and starts processing it concurrently, limited by the number of available peers.
// The caller is responsible for consuming all results of the batch using [Results], before submitting another batch.
func (w *workerPool) SubmitBatch(tasks []Task) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}

	w.resChan = make(chan TaskResult, len(tasks))

	go func() {
		for _, t := range tasks {
			w.wg.Add(1)
			go w.executeTask(t)
		}

		w.wg.Wait()
		close(w.resChan)
		w.resChan = nil
	}()

	return nil
}

func (w *workerPool) executeTask(t Task) {
	defer w.wg.Done()

	var who peer.ID
	select {
	case <-w.ctx.Done():
		return
	case who = <-w.peers:
	}

	logger.Infof("[EXECUTING] task=%s peer=%s", t.String(), who) // TODO: change to debug
	result, err := t.Do(who)
	if err != nil {
		logger.Infof("[ERR] retrying... task=%s peer=%s, err=%s", t.ID(), who, err.Error()) // TODO: change to debug
		w.wg.Add(1)
		go w.executeTask(t)
	} else {
		logger.Infof("[FINISHED] task=%s peer=%s", t.ID(), who) // TODO: change to debug
		w.AddPeer(who)
		w.resChan <- TaskResult{Task: t, Who: who, Result: result}
	}
}

// Results returns a channel that can be used to receive the results of the current batch. The channel is closed
// when the batch is completed. If no batch is currently being processed, the method returns nil.
func (w *workerPool) Results() chan TaskResult {
	return w.resChan
}

// AddPeer adds a peer to the worker pool unless it has been ignored previously.
func (w *workerPool) AddPeer(who peer.ID) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if _, ok := w.ignoredPeers[who]; ok {
		return
	}

	if len(w.peers) == cap(w.peers) {
		<-w.peers
	}
	w.peers <- who
}

// IgnorePeer prevents a peer from being added to the worker pool in the future, but does not remove it. After a peer
// that is currently in the pool is ignored, it won't be added to the pool again after the next task has been executed
// using it.
func (w *workerPool) IgnorePeer(who peer.ID) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	w.ignoredPeers[who] = struct{}{}
}

// IdlePeers returns the number of idle peers in the worker pool.
func (w *workerPool) IdlePeers() int {
	w.mtx.RLock()
	defer w.mtx.RUnlock()

	return len(w.peers)
}

// Shutdown stops the worker pool and waits for all tasks to abort.
func (w *workerPool) Shutdown() {
	w.cancel()
	w.wg.Wait()
}
