package cacheshim

import (
	"context"
	"time"
)

// ResultMeta is returned from Get calls along with the value and can be used
// to expose information about the cache status for debugging or testing.
type ResultMeta struct {
	// Hit indicates whether or not the request was a cache hit
	Hit bool

	// Age identifies how "stale" the result is. It's semantics differ based on
	// whether or not the cache type performs background refresh or not as defined
	// in https://www.consul.io/api/index.html#agent-caching.
	//
	// For background refresh types, Age is 0 unless the background blocking query
	// is currently in a failed state and so not keeping up with the server's
	// values. If it is non-zero it represents the time since the first failure to
	// connect during background refresh, and is reset after a background request
	// does manage to reconnect and either return successfully, or block for at
	// least the yamux keepalive timeout of 30 seconds (which indicates the
	// connection is OK but blocked as expected).
	//
	// For simple cache types, Age is the time since the result being returned was
	// fetched from the servers.
	Age time.Duration

	// Index is the internal ModifyIndex for the cache entry. Not all types
	// support blocking and all that do will likely have this in their result type
	// already but this allows generic code to reason about whether cache values
	// have changed.
	Index uint64
}

type Request interface {
	// CacheInfo returns information used for caching this request.
	CacheInfo() RequestInfo
}

type RequestInfo struct {
	// Key is a unique cache key for this request. This key should
	// be globally unique to identify this request, since any conflicting
	// cache keys could result in invalid data being returned from the cache.
	// The Key does not need to include ACL or DC information, since the
	// cache already partitions by these values prior to using this key.
	Key string

	// Token is the ACL token associated with this request.
	//
	// Datacenter is the datacenter that the request is targeting.
	//
	// PeerName is the peer that the request is targeting.
	//
	// All of these values are used to partition the cache. The cache framework
	// today partitions data on these values to simplify behavior: by
	// partitioning ACL tokens, the cache doesn't need to be smart about
	// filtering results. By filtering datacenter/peer results, the cache can
	// service the multi-DC/multi-peer nature of Consul. This comes at the expense of
	// working set size, but in general the effect is minimal.
	Token      string
	Datacenter string
	PeerName   string

	// MinIndex is the minimum index being queried. This is used to
	// determine if we already have data satisfying the query or if we need
	// to block until new data is available. If no index is available, the
	// default value (zero) is acceptable.
	MinIndex uint64

	// Timeout is the timeout for waiting on a blocking query. When the
	// timeout is reached, the last known value is returned (or maybe nil
	// if there was no prior value). This "last known value" behavior matches
	// normal Consul blocking queries.
	Timeout time.Duration

	// MaxAge if set limits how stale a cache entry can be. If it is non-zero and
	// there is an entry in cache that is older than specified, it is treated as a
	// cache miss and re-fetched. It is ignored for cachetypes with Refresh =
	// true.
	MaxAge time.Duration

	// MustRevalidate forces a new lookup of the cache even if there is an
	// existing one that has not expired. It is implied by HTTP requests with
	// `Cache-Control: max-age=0` but we can't distinguish that case from the
	// unset case for MaxAge. Later we may support revalidating the index without
	// a full re-fetch but for now the only option is to refetch. It is ignored
	// for cachetypes with Refresh = true.
	MustRevalidate bool
}

type UpdateEvent struct {
	// CorrelationID is used by the Notify API to allow correlation of updates
	// with specific requests. We could return the full request object and
	// cachetype for consumers to match against the calls they made but in
	// practice it's cleaner for them to choose the minimal necessary unique
	// identifier given the set of things they are watching. They might even
	// choose to assign random IDs for example.
	CorrelationID string
	Result        interface{}
	Meta          ResultMeta
	Err           error
}

type Callback func(ctx context.Context, event UpdateEvent)

type Cache interface {
	Get(ctx context.Context, t string, r Request) (interface{}, ResultMeta, error)
	NotifyCallback(ctx context.Context, t string, r Request, correlationID string, cb Callback) error
	Notify(ctx context.Context, t string, r Request, correlationID string, ch chan<- UpdateEvent) error
}

type RegisterOptions struct {
	// LastGetTTL is the time that the values returned by this type remain
	// in the cache after the last get operation. If a value isn't accessed
	// within this duration, the value is purged from the cache and
	// background refreshing will cease.
	LastGetTTL time.Duration

	// Refresh configures whether the data is actively refreshed or if
	// the data is only refreshed on an explicit Get. The default (false)
	// is to only request data on explicit Get.
	Refresh bool

	// SupportsBlocking should be set to true if the type supports blocking queries.
	// Types that do not support blocking queries will not be able to use
	// background refresh nor will the cache attempt blocking fetches if the
	// client requests them with MinIndex.
	SupportsBlocking bool

	// RefreshTimer is the time to sleep between attempts to refresh data.
	// If this is zero, then data is refreshed immediately when a fetch
	// is returned.
	//
	// Using different values for RefreshTimer and QueryTimeout, various
	// "refresh" mechanisms can be implemented:
	//
	//   * With a high timer duration and a low timeout, a timer-based
	//     refresh can be set that minimizes load on the Consul servers.
	//
	//   * With a low timer and high timeout duration, a blocking-query-based
	//     refresh can be set so that changes in server data are recognized
	//     within the cache very quickly.
	//
	RefreshTimer time.Duration

	// QueryTimeout is the default value for the maximum query time for a fetch
	// operation. It is set as FetchOptions.Timeout so that cache.Type
	// implementations can use it as the MaxQueryTime.
	QueryTimeout time.Duration
}

type Type interface {
	// Fetch fetches a single unique item.
	//
	// The FetchOptions contain the index and timeouts for blocking queries. The
	// MinIndex value on the Request itself should NOT be used as the blocking
	// index since a request may be reused multiple times as part of Refresh
	// behavior.
	//
	// The return value is a FetchResult which contains information about the
	// fetch. If an error is given, the FetchResult is ignored. The cache does not
	// support backends that return partial values. Optional State can be added to
	// the FetchResult which will be stored with the cache entry and provided to
	// the next Fetch call but will not be returned to clients. This allows types
	// to add additional bookkeeping data per cache entry that will still be aged
	// out along with the entry's TTL.
	//
	// On timeout, FetchResult can behave one of two ways. First, it can return
	// the last known value. This is the default behavior of blocking RPC calls in
	// Consul so this allows cache types to be implemented with no extra logic.
	// Second, FetchResult can return an unset value and index. In this case, the
	// cache will reuse the last value automatically. If an unset Value is
	// returned, the State field will still be updated which allows maintaining
	// metadata even when there is no result.
	Fetch(FetchOptions, Request) (FetchResult, error)

	// RegisterOptions are used when the type is registered to configure the
	// behaviour of cache entries for this type.
	RegisterOptions() RegisterOptions
}

type FetchOptions struct {
	// MinIndex is the minimum index to be used for blocking queries.
	// If blocking queries aren't supported for data being returned,
	// this value can be ignored.
	MinIndex uint64

	// Timeout is the maximum time for the query. This must be implemented
	// in the Fetch itself.
	Timeout time.Duration

	// LastResult is the result from the last successful Fetch and represents the
	// value currently stored in the cache at the time Fetch is invoked. It will
	// be nil on first call where there is no current cache value. There may have
	// been other Fetch attempts that resulted in an error in the mean time. These
	// are not explicitly represented currently. We could add that if needed this
	// was just simpler for now.
	//
	// The FetchResult read-only! It is constructed per Fetch call so modifying
	// the struct directly (e.g. changing it's Index of Value field) will have no
	// effect, however the Value and State fields may be pointers to the actual
	// values stored in the cache entry. It is thread-unsafe to modify the Value
	// or State via pointers since readers may be concurrently inspecting those
	// values under the entry lock (although we guarantee only one Fetch call per
	// entry) and modifying them even if the index doesn't change or the Fetch
	// eventually errors will likely break logical invariants in the cache too!
	LastResult *FetchResult
}

// FetchResult is the result of a Type Fetch operation and contains the
// data along with metadata gathered from that operation.
type FetchResult struct {
	// Value is the result of the fetch.
	Value interface{}

	// State is opaque data stored in the cache but not returned to clients. It
	// can be used by Types to maintain any bookkeeping they need between fetches
	// (using FetchOptions.LastResult) in a way that gets automatically cleaned up
	// by TTL expiry etc.
	State interface{}

	// Index is the corresponding index value for this data.
	Index uint64

	// NotModified indicates that the Value has not changed since LastResult, and
	// the LastResult value should be used instead of Value.
	NotModified bool
}
