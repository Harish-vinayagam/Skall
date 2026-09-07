package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/network"
	"github.com/Harish-vinayagam/Skall/internal/network/connection"
	"github.com/Harish-vinayagam/Skall/internal/network/discovery"
	"github.com/Harish-vinayagam/Skall/internal/storage"
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
		// discover [port]
		// Starts mDNS announce + browse and automatically connects to
		// discovered peers. Also starts the TCP server so that remote peers
		// can connect back to us.
		address := "0.0.0.0:4000"
		if len(args) > 1 {
			address = args[1]
		}
		if err := runDiscover(localIdentity, address); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("Usage:")
		fmt.Println("  skall identity")
		fmt.Println("  skall server [address]")
		fmt.Println("  skall client [address]")
		fmt.Println("  skall discover [address]")
	}
}

// runDiscover wires together the TCP server, mDNS announce, mDNS browse, and
// the Connector so that SKALL instances on the same LAN discover each other
// automatically.
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
		return fmt.Errorf("parse listen address: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
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

	// Announce ourselves so other SKALL instances can find us.
	if err := mdns.Announce(ctx, port); err != nil {
		return fmt.Errorf("mDNS announce: %w", err)
	}
	fmt.Printf("Announced as %s (%s) on port %d\n", localID.DisplayName, localID.PeerID[:8], port)

	// Start browsing for other instances.
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
