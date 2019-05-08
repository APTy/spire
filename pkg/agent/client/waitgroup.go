package client

import (
	"context"
	"errors"
	"sync"
	"time"
)

// WaitGroup manages concurrent access to the client.
type WaitGroup interface {
	Inc() error
	Done()
	Wait(context.Context) error
}

type waitGroup struct {
	wg         sync.WaitGroup
	mu         sync.RWMutex
	isDraining bool
}

func (n *waitGroup) Inc() error {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.isDraining {
		return errors.New("failed to fetch: client is draining requests")
	}

	n.wg.Add(1)
	return nil
}

func (n *waitGroup) Done() {
	n.wg.Done()
}

func (n *waitGroup) Wait(ctx context.Context) error {
	n.mu.Lock()
	n.isDraining = true
	n.mu.Unlock()

	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("client timed out waiting for requests to drain")
	case <-ctx.Done():
		return ctx.Err()
	}
}
