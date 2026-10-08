package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	libhost "github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	libprotocol "github.com/libp2p/go-libp2p/core/protocol"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

const (
	// DHTProtocolID is the libp2p protocol identifier for SKALL Kademlia DHT streams.
	DHTProtocolID = libprotocol.ID("/skall/kad/1.0.0")

	// DefaultDHTNamespace is the rendezvous namespace used for discovering SKALL peers over DHT.
	DefaultDHTNamespace = "skall.rendezvous.v2"

	// DHT Modes
	DHTModeAuto     = "auto"
	DHTModeClient   = "client"
	DHTModeServer   = "server"
	DHTModeDisabled = "disabled"

	// DefaultKBucketSize is the standard Kademlia bucket size (k=20).
	DefaultKBucketSize = 20
)

// DHTConfig configures the Kademlia DHT subsystem.
type DHTConfig struct {
	Mode           string         // "auto", "client", "server", "disabled"
	ProtocolPrefix libprotocol.ID // Protocol identifier
	Namespace      string         // Rendezvous namespace
	PollInterval   time.Duration  // Interval between peer discovery cycles
	RecordTTL      time.Duration  // How long peer advertisements remain valid
}

// DefaultDHTConfig returns a production-ready DHT configuration.
func DefaultDHTConfig() DHTConfig {
	return DHTConfig{
		Mode:           DHTModeAuto,
		ProtocolPrefix: DHTProtocolID,
		Namespace:      DefaultDHTNamespace,
		PollInterval:   30 * time.Second,
		RecordTTL:      15 * time.Minute,
	}
}

// DHTMessageType represents the type of a Kademlia RPC frame.
type DHTMessageType string

const (
	DHTMsgPing              DHTMessageType = "PING"
	DHTMsgPong              DHTMessageType = "PONG"
	DHTMsgFindNode          DHTMessageType = "FIND_NODE"
	DHTMsgFindNodeResp      DHTMessageType = "FIND_NODE_RESP"
	DHTMsgAdvertise         DHTMessageType = "ADVERTISE"
	DHTMsgFindProviders     DHTMessageType = "FIND_PROVIDERS"
	DHTMsgFindProvidersResp DHTMessageType = "FIND_PROVIDERS_RESP"
)

// DHTMessage is the wire message format exchanged over /skall/kad/1.0.0 streams.
type DHTMessage struct {
	Type      DHTMessageType   `json:"type"`
	Sender    peer.AddrInfo    `json:"sender"`
	Target    string           `json:"target,omitempty"`    // Target Key or PeerID
	Namespace string           `json:"namespace,omitempty"` // Rendezvous string
	Peers     []peer.AddrInfo  `json:"peers,omitempty"`     // Returned peers
	Timestamp time.Time        `json:"timestamp"`
	TTLSec    int64            `json:"ttl_sec,omitempty"`
}

type providerRecord struct {
	info      peer.AddrInfo
	expiresAt time.Time
}

// DHTManager implements Kademlia DHT routing, peer advertisement, and discovery for SKALL.
type DHTManager struct {
	host libhost.Host
	cfg  DHTConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Routing table (known DHT peers)
	routingMu    sync.RWMutex
	routingTable map[peer.ID]peer.AddrInfo

	// Rendezvous provider records (namespace -> peerID -> record)
	providersMu sync.RWMutex
	providers   map[string]map[peer.ID]providerRecord

	closed int32
}

// NewDHTManager initializes a Kademlia DHT manager on h.
func NewDHTManager(ctx context.Context, h libhost.Host, cfg DHTConfig) (*DHTManager, error) {
	if strings.EqualFold(cfg.Mode, DHTModeDisabled) {
		return nil, nil
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = DHTModeAuto
	}
	if mode != DHTModeAuto && mode != DHTModeClient && mode != DHTModeServer && mode != DHTModeDisabled {
		return nil, fmt.Errorf("unknown DHT mode %q; valid modes are auto, client, server, disabled", cfg.Mode)
	}
	cfg.Mode = mode

	if cfg.ProtocolPrefix == "" {
		cfg.ProtocolPrefix = DHTProtocolID
	}
	if cfg.Namespace == "" {
		cfg.Namespace = DefaultDHTNamespace
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	if cfg.RecordTTL <= 0 {
		cfg.RecordTTL = 15 * time.Minute
	}

	mCtx, mCancel := context.WithCancel(context.Background())
	dm := &DHTManager{
		host:         h,
		cfg:          cfg,
		ctx:          mCtx,
		cancel:       mCancel,
		routingTable: make(map[peer.ID]peer.AddrInfo),
		providers:    make(map[string]map[peer.ID]providerRecord),
	}

	// Register stream handler on the libp2p host if not purely client-restricted
	h.SetStreamHandler(cfg.ProtocolPrefix, dm.handleStream)

	return dm, nil
}

// Config returns the active DHTConfig.
func (dm *DHTManager) Config() DHTConfig {
	if dm == nil {
		return DHTConfig{Mode: DHTModeDisabled}
	}
	return dm.cfg
}

// AddPeer adds a known peer to the Kademlia routing table.
func (dm *DHTManager) AddPeer(pi peer.AddrInfo) {
	if dm == nil || pi.ID == dm.host.ID() || len(pi.Addrs) == 0 {
		return
	}
	dm.routingMu.Lock()
	defer dm.routingMu.Unlock()
	dm.routingTable[pi.ID] = pi
}

// RemovePeer drops a peer from the routing table.
func (dm *DHTManager) RemovePeer(pid peer.ID) {
	if dm == nil {
		return
	}
	dm.routingMu.Lock()
	delete(dm.routingTable, pid)
	dm.routingMu.Unlock()

	dm.providersMu.Lock()
	for _, m := range dm.providers {
		delete(m, pid)
	}
	dm.providersMu.Unlock()
}

// RoutingTableSize returns the number of peers currently in the Kademlia routing table.
func (dm *DHTManager) RoutingTableSize() int {
	if dm == nil {
		return 0
	}
	dm.routingMu.RLock()
	defer dm.routingMu.RUnlock()
	return len(dm.routingTable)
}

// ClosestPeers returns up to count peers closest to targetID according to the Kademlia XOR metric.
func (dm *DHTManager) ClosestPeers(targetID peer.ID, count int) []peer.AddrInfo {
	if dm == nil || count <= 0 {
		return nil
	}
	dm.routingMu.RLock()
	defer dm.routingMu.RUnlock()

	type peerDist struct {
		info peer.AddrInfo
		dist *big.Int
	}

	dists := make([]peerDist, 0, len(dm.routingTable))
	for _, pi := range dm.routingTable {
		dists = append(dists, peerDist{
			info: pi,
			dist: XORKeyDistance(string(targetID), string(pi.ID)),
		})
	}

	sort.Slice(dists, func(i, j int) bool {
		return dists[i].dist.Cmp(dists[j].dist) < 0
	})

	if len(dists) > count {
		dists = dists[:count]
	}

	out := make([]peer.AddrInfo, len(dists))
	for i, pd := range dists {
		out[i] = pd.info
	}
	return out
}

// XORKeyDistance calculates the Kademlia distance between two key strings using SHA-256 and XOR.
func XORKeyDistance(a, b string) *big.Int {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	xor := make([]byte, 32)
	for i := 0; i < 32; i++ {
		xor[i] = ha[i] ^ hb[i]
	}
	return new(big.Int).SetBytes(xor)
}

// Advertise advertises our AddrInfo under namespace to connected peers.
func (dm *DHTManager) Advertise(ctx context.Context, namespace string, ttl time.Duration) error {
	if dm == nil {
		return nil
	}
	if namespace == "" {
		namespace = dm.cfg.Namespace
	}
	if ttl <= 0 {
		ttl = dm.cfg.RecordTTL
	}

	myInfo := peer.AddrInfo{
		ID:    dm.host.ID(),
		Addrs: dm.host.Addrs(),
	}

	msg := DHTMessage{
		Type:      DHTMsgAdvertise,
		Sender:    myInfo,
		Namespace: namespace,
		Timestamp: time.Now().UTC(),
		TTLSec:    int64(ttl.Seconds()),
	}

	peers := dm.host.Network().Peers()
	for _, pid := range peers {
		if pid == dm.host.ID() {
			continue
		}
		go func(target peer.ID) {
			_ = dm.sendDHTMessage(ctx, target, msg, nil)
		}(pid)
	}

	// Also record locally
	dm.storeProvider(namespace, myInfo, ttl)
	return nil
}

// FindPeers queries connected DHT peers for providers of namespace.
func (dm *DHTManager) FindPeers(ctx context.Context, namespace string, limit int) (<-chan peer.AddrInfo, error) {
	if dm == nil {
		ch := make(chan peer.AddrInfo)
		close(ch)
		return ch, nil
	}
	if namespace == "" {
		namespace = dm.cfg.Namespace
	}
	if limit <= 0 {
		limit = DefaultKBucketSize
	}

	out := make(chan peer.AddrInfo, limit)

	go func() {
		defer close(out)

		seen := make(map[peer.ID]bool)
		seen[dm.host.ID()] = true

		// 1. Check local provider cache first
		localProviders := dm.getLocalProviders(namespace)
		for _, pi := range localProviders {
			if !seen[pi.ID] {
				seen[pi.ID] = true
				select {
				case out <- pi:
					if len(seen)-1 >= limit {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}

		// 2. Query peers over network
		req := DHTMessage{
			Type:      DHTMsgFindProviders,
			Sender:    peer.AddrInfo{ID: dm.host.ID(), Addrs: dm.host.Addrs()},
			Namespace: namespace,
			Timestamp: time.Now().UTC(),
		}

		peers := dm.host.Network().Peers()
		var wg sync.WaitGroup
		var mu sync.Mutex

		for _, p := range peers {
			if p == dm.host.ID() {
				continue
			}
			wg.Add(1)
			go func(pid peer.ID) {
				defer wg.Done()
				var resp DHTMessage
				dialCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
				defer cancel()

				if err := dm.sendDHTMessage(dialCtx, pid, req, &resp); err == nil && resp.Type == DHTMsgFindProvidersResp {
					mu.Lock()
					defer mu.Unlock()
					for _, found := range resp.Peers {
						if !seen[found.ID] && len(found.Addrs) > 0 {
							seen[found.ID] = true
							select {
							case out <- found:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}(p)
		}
		wg.Wait()
	}()

	return out, nil
}

// StartDiscovery begins continuous advertising and discovery in the background.
func (dm *DHTManager) StartDiscovery(ctx context.Context, onPeerFound func(peer.AddrInfo)) {
	if dm == nil {
		return
	}

	dm.wg.Add(1)
	go func() {
		defer dm.wg.Done()
		dm.loop(ctx, onPeerFound)
	}()
}

func (dm *DHTManager) loop(parentCtx context.Context, onPeerFound func(peer.AddrInfo)) {
	ticker := time.NewTicker(dm.cfg.PollInterval)
	defer ticker.Stop()

	// Initial advertise and find
	_ = dm.Advertise(parentCtx, dm.cfg.Namespace, dm.cfg.RecordTTL)
	dm.discoverOnce(parentCtx, onPeerFound)

	for {
		select {
		case <-dm.ctx.Done():
			return
		case <-parentCtx.Done():
			return
		case <-ticker.C:
			_ = dm.Advertise(parentCtx, dm.cfg.Namespace, dm.cfg.RecordTTL)
			dm.discoverOnce(parentCtx, onPeerFound)
		}
	}
}

func (dm *DHTManager) discoverOnce(ctx context.Context, onPeerFound func(peer.AddrInfo)) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ch, err := dm.FindPeers(queryCtx, dm.cfg.Namespace, DefaultKBucketSize)
	if err != nil {
		return
	}

	for pi := range ch {
		if pi.ID == dm.host.ID() || len(pi.Addrs) == 0 {
			continue
		}
		dm.AddPeer(pi)
		if onPeerFound != nil {
			onPeerFound(pi)
		}
	}
}

func (dm *DHTManager) handleStream(s network.Stream) {
	defer func() { _ = s.Close() }()

	_ = s.SetDeadline(time.Now().Add(10 * time.Second))
	r := protocol.NewFrameReader(s)
	msgBytes, err := r.ReadMessage()
	if err != nil {
		return
	}

	var dhtReq DHTMessage
	if err := json.Unmarshal([]byte(msgBytes.Body), &dhtReq); err != nil {
		return
	}

	// Update routing table with sender
	if len(dhtReq.Sender.Addrs) > 0 {
		dm.AddPeer(dhtReq.Sender)
	}

	switch dhtReq.Type {
	case DHTMsgPing:
		resp := DHTMessage{
			Type:      DHTMsgPong,
			Sender:    peer.AddrInfo{ID: dm.host.ID(), Addrs: dm.host.Addrs()},
			Timestamp: time.Now().UTC(),
		}
		dm.writeResponse(s, resp)

	case DHTMsgAdvertise:
		ttl := time.Duration(dhtReq.TTLSec) * time.Second
		if ttl <= 0 {
			ttl = dm.cfg.RecordTTL
		}
		dm.storeProvider(dhtReq.Namespace, dhtReq.Sender, ttl)

	case DHTMsgFindProviders:
		providers := dm.getLocalProviders(dhtReq.Namespace)
		resp := DHTMessage{
			Type:      DHTMsgFindProvidersResp,
			Sender:    peer.AddrInfo{ID: dm.host.ID(), Addrs: dm.host.Addrs()},
			Namespace: dhtReq.Namespace,
			Peers:     providers,
			Timestamp: time.Now().UTC(),
		}
		dm.writeResponse(s, resp)

	case DHTMsgFindNode:
		closest := dm.ClosestPeers(peer.ID(dhtReq.Target), DefaultKBucketSize)
		resp := DHTMessage{
			Type:      DHTMsgFindNodeResp,
			Sender:    peer.AddrInfo{ID: dm.host.ID(), Addrs: dm.host.Addrs()},
			Target:    dhtReq.Target,
			Peers:     closest,
			Timestamp: time.Now().UTC(),
		}
		dm.writeResponse(s, resp)
	}
}

func (dm *DHTManager) writeResponse(s network.Stream, resp DHTMessage) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	w := protocol.NewFrameWriter(s)
	_ = w.WriteMessage(protocol.Message{
		Version:   protocol.Version,
		ID:        fmt.Sprintf("dht-%d", time.Now().UnixNano()),
		Type:      protocol.TypeSystem,
		SenderID:  dm.host.ID().String(),
		Timestamp: time.Now().UTC(),
		Body:      string(data),
	})
}

func (dm *DHTManager) sendDHTMessage(ctx context.Context, target peer.ID, req DHTMessage, resp *DHTMessage) error {
	s, err := dm.host.NewStream(ctx, target, dm.cfg.ProtocolPrefix)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	_ = s.SetDeadline(time.Now().Add(5 * time.Second))

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	w := protocol.NewFrameWriter(s)
	if err := w.WriteMessage(protocol.Message{
		Version:   protocol.Version,
		ID:        fmt.Sprintf("dht-%d", time.Now().UnixNano()),
		Type:      protocol.TypeSystem,
		SenderID:  dm.host.ID().String(),
		Timestamp: time.Now().UTC(),
		Body:      string(data),
	}); err != nil {
		return err
	}

	if resp == nil {
		return nil
	}

	_ = s.CloseWrite()
	r := protocol.NewFrameReader(s)
	resMsg, err := r.ReadMessage()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	return json.Unmarshal([]byte(resMsg.Body), resp)
}

func (dm *DHTManager) storeProvider(ns string, pi peer.AddrInfo, ttl time.Duration) {
	if ns == "" || pi.ID == "" || len(pi.Addrs) == 0 {
		return
	}
	dm.providersMu.Lock()
	defer dm.providersMu.Unlock()

	bucket, ok := dm.providers[ns]
	if !ok {
		bucket = make(map[peer.ID]providerRecord)
		dm.providers[ns] = bucket
	}
	bucket[pi.ID] = providerRecord{
		info:      pi,
		expiresAt: time.Now().Add(ttl),
	}
}

func (dm *DHTManager) getLocalProviders(ns string) []peer.AddrInfo {
	dm.providersMu.Lock()
	defer dm.providersMu.Unlock()

	bucket, ok := dm.providers[ns]
	if !ok {
		return nil
	}

	now := time.Now()
	var out []peer.AddrInfo
	for pid, rec := range bucket {
		if now.After(rec.expiresAt) {
			delete(bucket, pid)
		} else {
			out = append(out, rec.info)
		}
	}
	return out
}

// Close gracefully stops the DHT manager and background loops.
func (dm *DHTManager) Close() error {
	if dm == nil {
		return nil
	}
	if !atomic.CompareAndSwapInt32(&dm.closed, 0, 1) {
		return nil
	}
	dm.cancel()
	dm.host.RemoveStreamHandler(dm.cfg.ProtocolPrefix)
	dm.wg.Wait()
	return nil
}
