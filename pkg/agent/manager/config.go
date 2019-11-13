package manager

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/andres-erbsen/clock"
	"github.com/sirupsen/logrus"
	attestor "github.com/spiffe/spire/pkg/agent/attestor/node"
	"github.com/spiffe/spire/pkg/agent/catalog"
	"github.com/spiffe/spire/pkg/agent/manager/cache"
	"github.com/spiffe/spire/pkg/agent/svid"
	"github.com/spiffe/spire/pkg/common/telemetry"
)

// Config holds a cache manager configuration
type Config struct {
	Attestor         attestor.Attestor
	Catalog          catalog.Catalog
	TrustDomain      url.URL
	Log              logrus.FieldLogger
	Metrics          telemetry.Metrics
	ServerAddr       string
	SVIDCachePath    string
	BundleCachePath  string
	SyncInterval     time.Duration
	RotationInterval time.Duration

	// Clk is the clock the manager will use to get time
	Clk clock.Clock
}

// New creates a cache manager based on c's configuration
func New(ctx context.Context, c *Config) (*manager, error) {
	as, err := c.Attestor.Attest(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to attest: %v", err)
	}
	spiffeID, err := getSpiffeIDFromSVID(as.SVID[0])
	if err != nil {
		return nil, fmt.Errorf("cannot get spiffe id from SVID: %v", err)
	}

	if c.SyncInterval == 0 {
		c.SyncInterval = 5 * time.Second
	}

	if c.RotationInterval == 0 {
		c.RotationInterval = svid.DefaultRotatorInterval
	}

	if c.Clk == nil {
		c.Clk = clock.New()
	}

	cache := cache.New(c.Log.WithField(telemetry.SubsystemName, telemetry.CacheManager), c.TrustDomain.String(), as.Bundle, c.Metrics)

	rotCfg := &svid.RotatorConfig{
		Catalog:      c.Catalog,
		Log:          c.Log,
		Metrics:      c.Metrics,
		Attestor:     c.Attestor,
		SVID:         as.SVID,
		SVIDKey:      as.Key,
		SpiffeID:     spiffeID,
		BundleStream: cache.SubscribeToBundleChanges(),
		ServerAddr:   c.ServerAddr,
		TrustDomain:  c.TrustDomain,
		Interval:     c.RotationInterval,
		Clk:          c.Clk,
	}
	svidRotator, client := svid.NewRotator(rotCfg)

	m := &manager{
		cache:           cache,
		c:               c,
		mtx:             new(sync.RWMutex),
		svid:            svidRotator,
		spiffeID:        spiffeID,
		svidCachePath:   c.SVIDCachePath,
		bundleCachePath: c.BundleCachePath,
		client:          client,
		clk:             c.Clk,
	}

	return m, nil
}
