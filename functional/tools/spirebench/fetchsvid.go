package main

import (
	"context"
	"log"

	"github.com/spiffe/spire/api/workload/v2"
)

func fetchSVID(ctx context.Context, agentAddr string) (*workload.X509SVID, error) {
	w := newBasicWatcher()
	c, err := workload.NewX509SVIDClient(w, workload.WithAddr(agentAddr))
	if err != nil {
		return nil, err
	}
	if err := c.Start(ctx); err != nil {
		return nil, err
	}
	svid := w.WaitForIdentity()
	if err := c.Stop(ctx); err != nil {
		return nil, err
	}
	return svid, nil
}

type basicWatcher struct {
	x509svid *workload.X509SVID
	sync     chan struct{}
}

func newBasicWatcher() *basicWatcher {
	return &basicWatcher{
		sync: make(chan struct{}),
	}
}

func (w *basicWatcher) UpdateX509SVIDs(x509svids *workload.X509SVIDs) {
	w.x509svid = x509svids.Default()
	close(w.sync)
}

func (w *basicWatcher) OnError(err error) {
	log.Printf("Error during identity watch: %v", err)
}

func (w *basicWatcher) WaitForIdentity() *workload.X509SVID {
	<-w.sync
	return w.x509svid
}
