package lru

import (
	"fmt"
	"time"

	"github.com/erigontech/erigon-lib/metrics"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

// Cache is a wrapper around hashicorp lru but with metric for Get
type Cache[K comparable, V any] struct {
	*lru.Cache[K, V]

	metricName string
}

func NewWithEvict[K comparable, V any](metricName string, size int, fn func(K, V)) (*Cache[K, V], error) {
	v, err := lru.NewWithEvict(size, fn)
	if err != nil {
		return nil, err
	}
	return &Cache[K, V]{Cache: v, metricName: metricName}, nil
}

func New[K comparable, V any](metricName string, size int) (*Cache[K, V], error) {
	v, err := lru.NewWithEvict[K, V](size, nil)
	if err != nil {
		return nil, err
	}
	return &Cache[K, V]{Cache: v, metricName: metricName}, nil
}

func (c *Cache[K, V]) Get(k K) (V, bool) {
	v, ok := c.Cache.Get(k)
	if ok {
		metrics.GetOrCreateCounter(fmt.Sprintf(`golang_lru_cache_hit{%s="%s"}`, "cache", c.metricName)).Inc()
	} else {
		metrics.GetOrCreateCounter(fmt.Sprintf(`golang_lru_cache_miss{%s="%s"}`, "cache", c.metricName)).Inc()
	}
	return v, ok
}

type CacheWithTTL[K comparable, V any] struct {
	*expirable.LRU[K, V]
	metric string
}

func NewWithTTL[K comparable, V any](metricName string, size int, ttl time.Duration) *CacheWithTTL[K, V] {
	cache := expirable.NewLRU[K, V](size, nil, ttl)
	return &CacheWithTTL[K, V]{LRU: cache, metric: metricName}
}

func (c *CacheWithTTL[K, V]) Get(k K) (V, bool) {
	v, ok := c.LRU.Get(k)
	if ok {
		metrics.GetOrCreateCounter(fmt.Sprintf(`golang_ttl_lru_cache_hit{%s="%s"}`, "cache", c.metric)).Inc()
	} else {
		metrics.GetOrCreateCounter(fmt.Sprintf(`golang_ttl_lru_cache_miss{%s="%s"}`, "cache", c.metric)).Inc()
	}
	return v, ok
}
