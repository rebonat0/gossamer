package cache

import "github.com/ChainSafe/gossamer/internal/primitives/runtime"

/// The local trie cache.
///
/// This cache should be used per state instance created by the backend. One state instance is
/// referring to the state of one block. It will cache all the accesses that are done to the state
/// which could not be fullfilled by the [`SharedTrieCache`]. These locally cached items are merged
/// back to the shared trie cache when this instance is dropped.
///
/// When using [`Self::as_trie_db_cache`] or [`Self::as_trie_db_mut_cache`], it will lock Mutexes.
/// So, it is important that these methods are not called multiple times, because they otherwise
/// deadlock.
// pub struct LocalTrieCache<H: Hasher> {
type LocalTrieCache[H runtime.Hash] struct {
	// 	/// The shared trie cache that created this instance.
	// 	shared: SharedTrieCache<H>,

	// 	/// The local cache for the trie nodes.
	// 	node_cache: Mutex<NodeCacheMap<H::Out>>,

	// 	/// The local cache for the values.
	// 	value_cache: Mutex<ValueCacheMap<H::Out>>,

	// 	/// Keeps track of all values accessed in the shared cache.
	// 	///
	// 	/// This will be used to ensure that these nodes are brought to the front of the lru when this
	// 	/// local instance is merged back to the shared cache. This can actually lead to collision when
	// 	/// two [`ValueCacheKey`]s with different storage roots and keys map to the same hash. However,
	// 	/// as we only use this set to update the lru position it is fine, even if we bring the wrong
	// 	/// value to the top. The important part is that we always get the correct value from the value
	// 	/// cache for a given key.
	// 	shared_value_cache_access: Mutex<ValueAccessSet>,

	// stats: TrieHitStats,
}
