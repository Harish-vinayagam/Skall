package discovery

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	defaultServiceName  = "_skall._tcp"
	defaultDomain       = "local."
	defaultPollInterval = 5 * time.Second
)

// resolverIface is the subset of zeroconf.Resolver used by MDNSDiscovery.
// It is satisfied by *zeroconf.Resolver and can be replaced in tests.
type resolverIface interface {
	Browse(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error
}

// ResolverFactory creates a new mDNS resolver. Tests inject a fake factory to
// avoid real network activity. The default factory calls zeroconf.NewResolver.
type ResolverFactory func() (resolverIface, error)

func defaultResolverFactory() (resolverIface, error) {
	return zeroconf.NewResolver(nil)
}

// MDNSConfig controls the mDNS browse/announce parameters.
type MDNSConfig struct {
	ServiceName     string
	Domain          string
	Interval        time.Duration
	ResolverFactory ResolverFactory // nil → uses zeroconf
}

// DefaultMDNSConfig returns a production-ready MDNSConfig.
func DefaultMDNSConfig() MDNSConfig {
	return MDNSConfig{
		ServiceName: defaultServiceName,
		Domain:      defaultDomain,
		Interval:    defaultPollInterval,
	}
}

// MDNSDiscovery discovers peers via mDNS (Bonjour/Zeroconf).
// It satisfies the Discovery interface.
type MDNSDiscovery struct {
	local  PeerInfo
	config MDNSConfig

	mu       sync.Mutex
	started  bool
	closed   bool
	events   chan Event
	shutdown chan struct{}
	resolver resolverIface
	entries  map[string]PeerInfo
}

// NewMDNSDiscovery creates a new MDNSDiscovery for the given local peer.
// config.ResolverFactory may be nil (uses zeroconf in production).
func NewMDNSDiscovery(local PeerInfo, config MDNSConfig) *MDNSDiscovery {
	if strings.TrimSpace(config.ServiceName) == "" {
		config.ServiceName = defaultServiceName
	}
	if strings.TrimSpace(config.Domain) == "" {
		config.Domain = defaultDomain
	}
	if config.Interval <= 0 {
		config.Interval = defaultPollInterval
	}
	if config.ResolverFactory == nil {
		config.ResolverFactory = defaultResolverFactory
	}
	return &MDNSDiscovery{
		local:    local,
		config:   config,
		events:   make(chan Event, 32),
		shutdown: make(chan struct{}),
		entries:  make(map[string]PeerInfo),
	}
}

// Events returns the channel on which discovery events are published.
func (d *MDNSDiscovery) Events() <-chan Event {
	return d.events
}

// Start initialises the resolver and begins periodic mDNS scanning.
// It returns an error if the resolver cannot be created or Start has already
// been called. Start is non-blocking after the first scan completes.
func (d *MDNSDiscovery) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return errors.New("discovery already started")
	}
	if d.closed {
		d.mu.Unlock()
		return errors.New("discovery is closed")
	}
	factory := d.config.ResolverFactory
	d.mu.Unlock()

	resolver, err := factory()
	if err != nil {
		return fmt.Errorf("create mDNS resolver: %w", err)
	}

	d.mu.Lock()
	d.resolver = resolver
	d.started = true
	d.mu.Unlock()

	if err := d.scan(ctx); err != nil {
		return fmt.Errorf("initial mDNS scan: %w", err)
	}

	go d.loop(ctx)
	return nil
}

func (d *MDNSDiscovery) loop(ctx context.Context) {
	d.mu.Lock()
	interval := d.config.Interval
	d.mu.Unlock()

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.shutdown:
			return
		case <-timer.C:
			_ = d.scan(ctx) // best-effort; transient errors are not fatal
			timer.Reset(interval)
		}
	}
}

func (d *MDNSDiscovery) scan(ctx context.Context) error {
	d.mu.Lock()
	resolver := d.resolver
	serviceName := d.config.ServiceName
	domain := d.config.Domain
	d.mu.Unlock()

	if resolver == nil {
		return errors.New("resolver not initialized")
	}

	results := make(chan *zeroconf.ServiceEntry, 16)
	scanCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := resolver.Browse(scanCtx, serviceName, domain, results); err != nil {
		return fmt.Errorf("browse mDNS: %w", err)
	}

	seen := make(map[string]PeerInfo)
	for {
		select {
		case entry, ok := <-results:
			if !ok {
				return d.applyDiff(seenValues(seen))
			}
			if entry == nil {
				continue
			}
			info, ok := entryToPeerInfo(*entry)
			if !ok {
				continue
			}
			seen[info.Key()] = info
		case <-scanCtx.Done():
			return d.applyDiff(seenValues(seen))
		}
	}
}

func seenValues(m map[string]PeerInfo) []PeerInfo {
	out := make([]PeerInfo, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func (d *MDNSDiscovery) applyDiff(seen []PeerInfo) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	seenSet := make(map[string]PeerInfo, len(seen))
	for _, info := range seen {
		if !info.IsValid() {
			continue
		}
		if info.PeerID == d.local.PeerID {
			continue
		}
		seenSet[info.Key()] = info
	}

	for key, info := range seenSet {
		if _, exists := d.entries[key]; exists {
			continue
		}
		d.entries[key] = info
		select {
		case d.events <- Event{Type: EventPeerAdded, Peer: info}:
		default:
		}
	}

	for key, info := range d.entries {
		if _, stillThere := seenSet[key]; stillThere {
			continue
		}
		delete(d.entries, key)
		select {
		case d.events <- Event{Type: EventPeerRemoved, Peer: info}:
		default:
		}
	}
	return nil
}

func entryToPeerInfo(entry zeroconf.ServiceEntry) (PeerInfo, bool) {
	host := strings.TrimSpace(entry.HostName)
	if host == "" {
		if len(entry.AddrIPv4) > 0 {
			host = entry.AddrIPv4[0].String()
		} else if len(entry.AddrIPv6) > 0 {
			host = entry.AddrIPv6[0].String()
		}
	}
	port := entry.Port
	if port == 0 {
		return PeerInfo{}, false
	}
	peerID := ""
	displayName := ""
	for _, field := range entry.Text {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch strings.TrimSpace(parts[0]) {
		case "peer_id":
			peerID = strings.TrimSpace(parts[1])
		case "display_name":
			displayName = strings.TrimSpace(parts[1])
		}
	}
	if peerID == "" {
		return PeerInfo{}, false
	}
	return PeerInfo{
		PeerID:      peerID,
		DisplayName: displayName,
		Host:        host,
		Port:        port,
	}, true
}

// Close shuts down the discovery loop. It is safe to call multiple times.
func (d *MDNSDiscovery) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	close(d.shutdown)
	d.mu.Unlock()
	return nil
}

// Announce registers this peer on the local network via mDNS so that other
// SKALL instances can discover it. The registration is withdrawn when ctx is
// cancelled.
func (d *MDNSDiscovery) Announce(ctx context.Context, port int) error {
	if port <= 0 {
		return errors.New("invalid port for announcement")
	}
	if strings.TrimSpace(d.local.PeerID) == "" {
		return errors.New("local peer id is required for announcement")
	}
	txt := []string{
		"peer_id=" + d.local.PeerID,
		"display_name=" + strings.TrimSpace(d.local.DisplayName),
	}
	server, err := zeroconf.Register(d.local.DisplayName, d.config.ServiceName, d.config.Domain, port, txt, nil)
	if err != nil {
		return fmt.Errorf("register mDNS service: %w", err)
	}
	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()
	return nil
}

// ParsePort parses and validates a TCP port string.
func ParsePort(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, errors.New("port is required")
	}
	port, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("parse port: %w", err)
	}
	if port <= 0 || port > 65535 {
		return 0, errors.New("port out of range")
	}
	return port, nil
}
