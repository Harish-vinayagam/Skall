package p2p

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// BootstrapResult captures the outcome of an initial connection attempt to a bootstrap peer.
type BootstrapResult struct {
	AddrInfo peer.AddrInfo
	Err      error
	Duration time.Duration
}

// BootstrapManager manages connections to configured bootstrap peers.
// It handles unreachable peers gracefully, avoids crashes, and continuously maintains
// bootstrap connectivity using exponential backoff retry.
type BootstrapManager struct {
	host       Host
	peers      []peer.AddrInfo
	backoffCfg BackoffConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	connectedCount int64
	closed         int32
}

// NewBootstrapManager creates a manager for the given bootstrap peer multiaddresses.
// Any invalid address strings produce an error; valid addresses have self-connections filtered.
func NewBootstrapManager(h Host, addrs []string, cfg BackoffConfig) (*BootstrapManager, error) {
	if h == nil {
		return nil, fmt.Errorf("bootstrap manager: host cannot be nil")
	}

	var parsedPeers []peer.AddrInfo
	for _, raw := range addrs {
		if raw == "" {
			continue
		}
		pi, err := ParsePeerAddr(raw, "")
		if err != nil {
			return nil, fmt.Errorf("bootstrap manager: invalid bootstrap peer %q: %w", raw, err)
		}
		// Skip self
		if pi.ID == h.ID() {
			continue
		}
		parsedPeers = append(parsedPeers, pi)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &BootstrapManager{
		host:       h,
		peers:      parsedPeers,
		backoffCfg: cfg,
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Peers returns the parsed bootstrap peer AddrInfos.
func (bm *BootstrapManager) Peers() []peer.AddrInfo {
	out := make([]peer.AddrInfo, len(bm.peers))
	copy(out, bm.peers)
	return out
}

// ConnectAll attempts to connect to all configured bootstrap peers in parallel.
// Unreachable peers return an error in their BootstrapResult but do not cause a crash.
func (bm *BootstrapManager) ConnectAll(ctx context.Context) []BootstrapResult {
	if len(bm.peers) == 0 {
		return nil
	}

	results := make([]BootstrapResult, len(bm.peers))
	var wg sync.WaitGroup

	for i, pi := range bm.peers {
		wg.Add(1)
		go func(idx int, target peer.AddrInfo) {
			defer wg.Done()
			start := time.Now()
			// Use a bounded timeout per peer attempt
			dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			err := bm.host.Connect(dialCtx, target)
			results[idx] = BootstrapResult{
				AddrInfo: target,
				Err:      err,
				Duration: time.Since(start),
			}
			if err == nil {
				atomic.AddInt64(&bm.connectedCount, 1)
			}
		}(i, pi)
	}

	wg.Wait()
	return results
}

// Start launches a background maintenance loop that monitors bootstrap peer connectivity
// and reconnects with exponential backoff if connectivity is lost.
func (bm *BootstrapManager) Start(ctx context.Context) {
	if len(bm.peers) == 0 {
		return
	}

	bm.wg.Add(1)
	go func() {
		defer bm.wg.Done()
		bm.maintainLoop(ctx)
	}()
}

func (bm *BootstrapManager) maintainLoop(ctx context.Context) {
	backoff := NewBackoff(bm.backoffCfg)
	checkTicker := time.NewTicker(10 * time.Second)
	defer checkTicker.Stop()

	for {
		select {
		case <-bm.ctx.Done():
			return
		case <-ctx.Done():
			return
		case <-checkTicker.C:
			// Check if we are connected to any bootstrap peer
			connectedPeers := bm.host.ConnectedPeers()
			connectedSet := make(map[peer.ID]bool, len(connectedPeers))
			for _, pid := range connectedPeers {
				connectedSet[pid] = true
			}

			hasBootstrap := false
			var disconnected []peer.AddrInfo
			for _, bp := range bm.peers {
				if connectedSet[bp.ID] {
					hasBootstrap = true
				} else {
					disconnected = append(disconnected, bp)
				}
			}

			if hasBootstrap {
				// We have at least one bootstrap peer connected; reset backoff
				backoff.Reset()
				continue
			}

			// We have lost connection to all bootstrap peers; attempt reconnect with backoff
			if len(disconnected) > 0 {
				log.Printf("p2p: bootstrap connectivity lost; retrying in %v (attempt %d)...",
					backoff.currentDelay, backoff.Attempts()+1)

				if err := backoff.Sleep(bm.ctx); err != nil {
					return
				}

				reconnected := false
				for _, bp := range disconnected {
					dialCtx, dialCancel := context.WithTimeout(bm.ctx, 5*time.Second)
					if err := bm.host.Connect(dialCtx, bp); err == nil {
						log.Printf("p2p: reconnected to bootstrap peer %s", bp.ID)
						reconnected = true
					}
					dialCancel()
				}

				if reconnected {
					backoff.Reset()
				}
			}
		}
	}
}

// Close gracefully stops the bootstrap manager and background goroutines.
func (bm *BootstrapManager) Close() error {
	if !atomic.CompareAndSwapInt32(&bm.closed, 0, 1) {
		return nil
	}
	bm.cancel()
	bm.wg.Wait()
	return nil
}
