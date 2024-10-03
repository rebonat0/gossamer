package sync

import (
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

type WarpSyncStrategy struct {
}

func (s *WarpSyncStrategy) Process(results []*SyncTaskResult) (
	isFinished bool, reputations []Change, bans []peer.ID, err error) {
	panic("implement me")
}

func (s *WarpSyncStrategy) OnBlockAnnounceHandshake(from peer.ID, msg *network.BlockAnnounceHandshake) error {
	panic("implement me")
}

func (s *WarpSyncStrategy) OnBlockAnnounce(from peer.ID, msg *network.BlockAnnounceMessage) (
	repChange *Change, err error) {
	panic("implement me")
}

func (s *WarpSyncStrategy) NextActions() ([]*SyncTask, error) {
	s.startedAt = time.Now()
	panic("implement me")
}

func (f *WarpSyncStrategy) IsSynced() bool {
	panic("implement me")
}

func (f *WarpSyncStrategy) ShowMetrics() {
	panic("implement me")
}

var _ Strategy = (*WarpSyncStrategy)(nil)
