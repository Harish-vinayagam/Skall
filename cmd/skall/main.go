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

	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/network"
	"github.com/Harish-vinayagam/Skall/internal/network/connection"
	"github.com/Harish-vinayagam/Skall/internal/network/discovery"
	"github.com/Harish-vinayagam/Skall/internal/network/p2p"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
	"github.com/Harish-vinayagam/Skall/internal/storage"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
)

func main() {
	localIdentity, err := loadLocalIdentity()
	if err != nil {
		log.Fatal(err)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		if err := network.StartServer("localhost:4000", localIdentity); err != nil {
			log.Fatal(err)
		}
		return
	}

	switch args[0] {
	case "identity":
		fmt.Println(localIdentity.Summary())
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
		// p2p [listen-multiaddr]
		// Starts a libp2p node with mDNS discovery and a stdin message loop.
		listenAddr := "/ip4/0.0.0.0/tcp/0"
		if len(args) > 1 {
			listenAddr = args[1]
		}
		if err := runP2P(localIdentity, listenAddr); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("Usage:")
		fmt.Println("  skall identity")
		fmt.Println("  skall server [address]")
		fmt.Println("  skall client [address]")
		fmt.Println("  skall discover [address]")
		fmt.Println("  skall p2p [listen-multiaddr]")
	}
}

// runP2P starts a libp2p-backed SKALL node. It:
//   - builds a libp2p host from the existing identity (no new keys)
//   - starts mDNS discovery for automatic LAN peer finding
//   - registers an inbound message handler that prints received messages
//   - reads stdin and broadcasts messages to all connected peers
//   - shuts down cleanly on SIGINT/SIGTERM
func runP2P(localID identity.Identity, listenAddr string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	node, err := p2p.BuildNode(ctx, localID, []string{listenAddr})
	if err != nil {
		return fmt.Errorf("p2p: build node: %w", err)
	}
	defer node.Close()

	// Derive the libp2p peer.ID for display purposes.
	lp2pID, err := localID.LibP2PPeerID()
	if err != nil {
		return fmt.Errorf("p2p: derive peer id: %w", err)
	}

	fmt.Printf("SKALL p2p node started\n")
	fmt.Printf("  SKALL peer ID  : %s\n", localID.PeerID)
	fmt.Printf("  libp2p peer ID : %s\n", lp2pID)
	fmt.Printf("  Display name   : %s\n", localID.DisplayName)
	fmt.Printf("  Listen addrs   : %v\n", node.Addrs())
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
