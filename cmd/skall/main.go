package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/Harish-vinayagam/Skall/internal/chat"
	"github.com/Harish-vinayagam/Skall/internal/groups"
	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/network"
	"github.com/Harish-vinayagam/Skall/internal/network/connection"
	"github.com/Harish-vinayagam/Skall/internal/network/discovery"
	"github.com/Harish-vinayagam/Skall/internal/network/p2p"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
	"github.com/Harish-vinayagam/Skall/internal/storage"
	"github.com/Harish-vinayagam/Skall/internal/ui"
)

func main() {
	localIdentity, err := loadLocalIdentity()
	if err != nil {
		log.Fatal(err)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		// Default: launch the interactive TUI (no manual peers).
		if err := runTUI(localIdentity, "", nil); err != nil {
			log.Fatal(err)
		}
		return
	}


	switch args[0] {
	case "identity":
		fmt.Println(localIdentity.Summary())
		if lp2pID, err := localIdentity.LibP2PPeerID(); err == nil {
			fmt.Printf("libp2p Peer ID: %s\n", lp2pID)
			fmt.Printf("Sample Multiaddr (port 9001): /ip4/127.0.0.1/tcp/9001/p2p/%s\n", lp2pID)
		}
	case "server":
		address := "localhost:4000"
		if len(args) > 1 {
			address = args[1]
		}
		if err := network.StartServer(address, localIdentity); err != nil {
			log.Fatal(err)
		}
	case "client":
		address := "localhost:4000"
		if len(args) > 1 {
			address = args[1]
		}
		if err := network.StartClient(address, localIdentity); err != nil {
			log.Fatal(err)
		}
	case "discover":
		// discover [address]
		address := "0.0.0.0:4000"
		if len(args) > 1 {
			address = args[1]
		}
		if err := runDiscover(localIdentity, address); err != nil {
			log.Fatal(err)
		}
	case "p2p":
		// p2p [--listen <port|addr>] [--peer <multiaddr>...]
		listenAddr := "/ip4/0.0.0.0/tcp/0"
		var peerAddrs []string
		for i := 1; i < len(args); i++ {
			if (args[i] == "--listen" || args[i] == "-l") && i+1 < len(args) {
				listenAddr = parseListenAddr(args[i+1])
				i++
			} else if (args[i] == "--peer" || args[i] == "-p") && i+1 < len(args) {
				peerAddrs = append(peerAddrs, args[i+1])
				i++
			} else if !strings.HasPrefix(args[i], "-") {
				listenAddr = parseListenAddr(args[i])
			}
		}
		if err := runP2P(localIdentity, listenAddr, peerAddrs); err != nil {
			log.Fatal(err)
		}
	case "tui":
		// tui [--listen <port|addr>] [--peer <multiaddr>...]
		listenAddr := "/ip4/0.0.0.0/tcp/0"
		var peerAddrs []string
		for i := 1; i < len(args); i++ {
			if (args[i] == "--listen" || args[i] == "-l") && i+1 < len(args) {
				listenAddr = parseListenAddr(args[i+1])
				i++
			} else if (args[i] == "--peer" || args[i] == "-p") && i+1 < len(args) {
				peerAddrs = append(peerAddrs, args[i+1])
				i++
			}
		}
		if err := runTUI(localIdentity, listenAddr, peerAddrs); err != nil {
			log.Fatal(err)
		}
	default:
		// Check if flags are passed directly without "tui" subcommand
		if strings.HasPrefix(args[0], "-") {
			listenAddr := "/ip4/0.0.0.0/tcp/0"
			var peerAddrs []string
			for i := 0; i < len(args); i++ {
				if (args[i] == "--listen" || args[i] == "-l") && i+1 < len(args) {
					listenAddr = parseListenAddr(args[i+1])
					i++
				} else if (args[i] == "--peer" || args[i] == "-p") && i+1 < len(args) {
					peerAddrs = append(peerAddrs, args[i+1])
					i++
				}
			}
			if err := runTUI(localIdentity, listenAddr, peerAddrs); err != nil {
				log.Fatal(err)
			}
			return
		}
		fmt.Println("Usage:")
		fmt.Println("  skall                                (launch TUI — default)")
		fmt.Println("  skall tui [--listen PORT] [--peer MULTIADDR]  (launch TUI with options)")
		fmt.Println("  skall p2p [--listen PORT] [--peer MULTIADDR]  (CLI p2p broadcast)")
		fmt.Println("  skall identity                       (display local identity & multiaddr)")
		fmt.Println("  skall server [address]")
		fmt.Println("  skall client [address]")
		fmt.Println("  skall discover [address]")
	}
}

func parseListenAddr(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/ip4/0.0.0.0/tcp/0"
	}
	if strings.HasPrefix(raw, "/") {
		return raw
	}
	if _, err := strconv.Atoi(raw); err == nil {
		return fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", raw)
	}
	return raw
}


// runP2P starts a libp2p-backed SKALL node. It:
//   - builds a libp2p host from the existing identity (no new keys)
//   - starts mDNS discovery for automatic LAN peer finding
//   - registers an inbound message handler that prints received messages
//   - reads stdin and broadcasts messages to all connected peers
//   - shuts down cleanly on SIGINT/SIGTERM
func runP2P(localID identity.Identity, listenAddr string, peerAddrs []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var listenAddrs []string
	if listenAddr != "" {
		listenAddrs = []string{listenAddr}
	}

	node, err := p2p.BuildNode(ctx, localID, listenAddrs)
	if err != nil {
		return fmt.Errorf("p2p: build node: %w", err)
	}
	defer node.Close()

	// Derive the libp2p peer.ID for display purposes.
	lp2pID, err := localID.LibP2PPeerID()
	if err != nil {
		return fmt.Errorf("p2p: derive peer id: %w", err)
	}

	// Connect to any manual peers provided
	for _, peerAddr := range peerAddrs {
		maddr, err := ma.NewMultiaddr(peerAddr)
		if err != nil {
			log.Printf("skall: invalid peer addr %q: %v", peerAddr, err)
			continue
		}
		pi, err := libp2ppeer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.Printf("skall: parse peer addr %q: %v", peerAddr, err)
			continue
		}
		if err := node.Connect(ctx, *pi); err != nil {
			log.Printf("skall: connect to %s: %v", peerAddr, err)
		} else {
			log.Printf("skall: connected to %s", peerAddr)
		}
	}

	fmt.Printf("SKALL p2p node started\n")
	fmt.Printf("  SKALL peer ID  : %s\n", localID.PeerID)
	fmt.Printf("  libp2p peer ID : %s\n", lp2pID)
	fmt.Printf("  Display name   : %s\n", localID.DisplayName)
	fmt.Printf("  Listen addrs   :\n")
	for _, addr := range node.Addrs() {
		fmt.Printf("    %s/p2p/%s\n", addr, lp2pID)
	}
	fmt.Println("mDNS discovery active. Peers on the same LAN will connect automatically.")
	fmt.Println("Type a message and press Enter to broadcast. Press Ctrl+C to quit.")

	// Print incoming messages to stdout.
	node.SetMessageHandler(func(msg protocol.Message, from libp2ppeer.ID) {
		fmt.Printf("\r[%s] %s: %s\n", msg.Timestamp.Format("15:04:05"), from.ShortString(), msg.Body)
	})

	// Read stdin and send to all connected peers.
	scanner := bufio.NewScanner(os.Stdin)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			msg := protocol.NewChatMessage(localID.PeerID, "", "", line)
			for _, pid := range node.ConnectedPeers() {
				if err := node.SendMessage(ctx, pid, msg); err != nil {
					fmt.Printf("send to %s: %v\n", pid.ShortString(), err)
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		fmt.Println("\nShutting down p2p node...")
	case <-readDone:
	}
	return nil
}

// runDiscover wires together the TCP server, mDNS announce, mDNS browse, and
// the Connector so that SKALL instances on the same LAN discover each other
// automatically (legacy TCP path).
func runDiscover(localID identity.Identity, address string) error {
	// --- TCP server ---
	server := network.NewServer(address)
	if err := server.Listen(); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	fmt.Println("SKALL server listening on", server.Addr())

	// Derive the TCP port from the listener address so we know what to announce.
	_, portStr, err := net.SplitHostPort(server.Addr())
	if err != nil {
		return fmt.Errorf("parse listen address %q: %w", server.Addr(), err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port == 0 {
		return fmt.Errorf("invalid port %q", portStr)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Accept incoming connections in the background.
	go func() {
		if err := server.Serve(); err != nil {
			log.Println("server error:", err)
		}
	}()

	// --- mDNS discovery ---
	local := discovery.PeerInfo{
		PeerID:      localID.PeerID,
		DisplayName: localID.DisplayName,
		Host:        "localhost",
		Port:        port,
	}
	mdns := discovery.NewMDNSDiscovery(local, discovery.DefaultMDNSConfig())

	if err := mdns.Announce(ctx, port); err != nil {
		return fmt.Errorf("mDNS announce: %w", err)
	}
	fmt.Printf("Announced as %s (%s) on port %d\n", localID.DisplayName, localID.PeerID[:8], port)

	if err := mdns.Start(ctx); err != nil {
		return fmt.Errorf("mDNS start: %w", err)
	}
	defer mdns.Close()

	// --- Connection manager ---
	mgr := connection.NewManager(nil)

	// --- Connector: bridges discovery events → connection manager ---
	conn := discovery.NewConnector(mgr)
	go func() {
		if err := conn.Run(ctx, mdns.Events()); err != nil && ctx.Err() == nil {
			log.Println("connector error:", err)
		}
	}()

	fmt.Println("Peer discovery active. Press Ctrl+C to stop.")
	<-ctx.Done()
	fmt.Println("Shutting down...")
	return server.Shutdown()
}

func loadLocalIdentity() (identity.Identity, error) {
	store, err := identity.DefaultStore()
	if err != nil {
		return identity.Identity{}, err
	}

	localIdentity, err := store.LoadOrCreate()
	if err != nil {
		return identity.Identity{}, err
	}

	db, err := storage.OpenDefault()
	if err != nil {
		return identity.Identity{}, err
	}
	defer func() { _ = db.Close() }()

	if err := db.UpsertIdentityMetadata(localIdentity); err != nil {
		return identity.Identity{}, err
	}

	return localIdentity, nil
}

// runTUI starts the full Bubble Tea terminal UI.
// listenAddr is an optional multiaddr or port to listen on.
// peerAddrs is an optional list of multiaddrs to connect to immediately
// (useful when mDNS is blocked by a VPN or firewall).
func runTUI(localID identity.Identity, listenAddr string, peerAddrs []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var listenAddrs []string
	if listenAddr != "" && listenAddr != "/ip4/0.0.0.0/tcp/0" {
		listenAddrs = []string{listenAddr}
	}

	// Build p2p node (mDNS-enabled for LAN peer discovery)
	node, err := p2p.BuildNode(ctx, localID, listenAddrs)
	if err != nil {
		return fmt.Errorf("tui: build p2p node: %w", err)
	}

	// Print our listen addresses so the other user can copy them for --peer.
	lp2pID, _ := localID.LibP2PPeerID()
	for _, addr := range node.Addrs() {
		log.Printf("skall: listening on %s/p2p/%s", addr, lp2pID)
	}

	// Manually connect to any peers provided via --peer flag.
	// This bypasses mDNS and works even when multicast is blocked (e.g. VPN).
	for _, peerAddr := range peerAddrs {
		maddr, err := ma.NewMultiaddr(peerAddr)
		if err != nil {
			log.Printf("skall: invalid peer addr %q: %v", peerAddr, err)
			continue
		}
		pi, err := libp2ppeer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.Printf("skall: parse peer addr %q: %v", peerAddr, err)
			continue
		}
		if err := node.Connect(ctx, *pi); err != nil {
			log.Printf("skall: connect to %s: %v", peerAddr, err)
		} else {
			log.Printf("skall: connected to %s", peerAddr)
		}
	}

	// Open SQLite store
	db, err := storage.OpenDefault()
	if err != nil {
		_ = node.Close()
		return fmt.Errorf("tui: open store: %w", err)
	}

	// Group manager (in-memory for now)
	grpMgr := groups.NewManager()

	// Chat service — bridges UI ↔ network+storage
	svc := chat.NewP2PService(ctx, localID, node, db, grpMgr)

	// Build and run the TUI
	app := ui.New(svc)
	prog := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := prog.Run(); err != nil {
		_ = svc.Close()
		_ = db.Close()
		return fmt.Errorf("tui: program error: %w", err)
	}

	_ = svc.Close()
	_ = db.Close()
	return nil
}

