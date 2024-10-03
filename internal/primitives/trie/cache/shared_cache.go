package cache

import (
	"sync"

	"github.com/ChainSafe/gossamer/internal/primitives/runtime"
)

// type SharedNodeCacheMap<H> =
// 	LruMap<H, NodeOwned<H>, SharedNodeCacheLimiter, schnellru::RandomState>;
// type SharedNodeCacheMap = map[]

// / The shared node cache.
// /
// / Internally this stores all cached nodes in a [`LruMap`]. It ensures that when updating the
// / cache, that the cache stays within its allowed bounds.
// pub(super) struct SharedNodeCache<H>
// where
//
//	H: AsRef<[u8]>,
//
// {
type SharedNodeCache[H runtime.Hash] struct {
	// 	/// The cached nodes, ordered by least recently used.
	// 	pub(super) lru: SharedNodeCacheMap<H>,
}

// / The shared trie cache.
// /
// / It should be instantiated once per node. It will hold the trie nodes and values of all
// / operations to the state. To not use all available memory it will ensure to stay in the
// / bounds given via the [`CacheSize`] at startup.
// /
// / The instance of this object can be shared between multiple threads.
// pub struct SharedTrieCache<H: Hasher> {
type SharedTrieCache[H runtime.Hash] struct {
	// inner: Arc<RwLock<SharedTrieCacheInner<H>>>,
	inner SharedTrieCacheInner[H]
	mtx   sync.RWMutex
}

// / The inner of [`SharedTrieCache`].
// pub(super) struct SharedTrieCacheInner<H: Hasher> {
type SharedTrieCacheInner[H runtime.Hash] struct {
	// node_cache: SharedNodeCache<H::Out>,
	nodeCache SharedNodeCache[H]
	// value_cache: SharedValueCache<H::Out>,
}
