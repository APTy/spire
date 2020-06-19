package endpoints

import (
	"sync"
	"time"

	"github.com/spiffe/spire/proto/spire/common"
)

const defaultBundleCacheTTL = time.Second

type bundleCache struct {
	sync.RWMutex

	ttl       time.Duration
	timeNow   func() time.Time
	bundle    *common.Bundle
	expiresAt time.Time
}

func newBundleCache() *bundleCache {
	return &bundleCache{
		ttl:     defaultBundleCacheTTL,
		timeNow: time.Now,
	}
}

// return a bundle cache that never caches since the entries always expire in the past
func newNoopBundleCache() *bundleCache {
	return &bundleCache{
		ttl:     -1 * time.Second,
		timeNow: func() time.Time { return time.Time{} },
	}
}

func (c *bundleCache) Get() *common.Bundle {
	c.RLock()
	defer c.RUnlock()

	if c.timeNow().After(c.expiresAt) {
		return nil
	}
	return c.bundle
}

func (c *bundleCache) Set(bundle *common.Bundle) {
	c.Lock()
	defer c.Unlock()

	c.expiresAt = c.timeNow().Add(c.ttl)
	c.bundle = bundle
}
