package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/Harish-vinayagam/Skall/internal/chat"
	"github.com/Harish-vinayagam/Skall/internal/config"
	"github.com/Harish-vinayagam/Skall/internal/groups"
	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/network"
	"github.com/Harish-vinayagam/Skall/internal/network/connection"
	"github.com/Harish-vinayagam/Skall/internal/network/discovery"
	"github.com/Harish-vinayagam/Skall/internal/network/p2p"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
	"github.com/Harish-vinayagam/Skall/internal/storage"
	"github.com/Harish-vinayagam/Skall/internal/ui"
	"github.com/Harish-vinayagam/Skall/internal/version"
)

func main() {
	cfg, err := config.LoadOrCreate()
	if err != nil {
		log.Printf("skall: warning: load config: %v", err)
		cfg = config.DefaultConfig()
	}

	localIdentity, err := loadLocalIdentity(cfg)
	if err != nil {
		log.Fatal(err)
	}

	args := os.Args[1:]
	if err := runCLI(args, localIdentity, cfg, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// runCLI dispatches command-line arguments to their respective handlers.
func runCLI(args []string, localID identity.Identity, cfg config.Config, out io.Writer) error {
	if len(args) == 0 {
		return runTUI(localID, cfg, "", nil)
	}

	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintln(out, version.String())
		return nil

	case "help", "--help", "-h":
		printHelp(out)
		return nil

	case "identity":
		return handleIdentityCommand(args[1:], localID, cfg, out)

	case "peers":
		return handlePeersCommand(cfg, out)

	case "connect":
		if len(args) < 2 {
			return errors.New("usage: skall connect <multiaddr|host:port>")
		}
		return handleConnectCommand(args[1], localID, cfg, out)

	case "chat":
		target := ""
		if len(args) > 1 {
			target = args[1]
		}
		var peerAddrs []string
		if strings.HasPrefix(target, "/") {
			peerAddrs = append(peerAddrs, target)
		}
		return runTUI(localID, cfg, "", peerAddrs)

	case "groups":
		return handleGroupsCommand(args[1:], cfg, out)

	case "config":
		return handleConfigCommand(args[1:], cfg, out)

	case "server":
		address := "localhost:4000"
		if len(args) > 1 {
			address = args[1]
		}
		return network.StartServer(address, localID)

	case "client":
		address := "localhost:4000"
		if len(args) > 1 {
			address = args[1]
		}
		return network.StartClient(address, localID)

	case "discover":
		address := "0.0.0.0:4000"
		if len(args) > 1 {
			address = args[1]
		}
		return runDiscover(localID, address)

	case "p2p":
		listenAddr := ""
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
		return runP2P(localID, listenAddr, peerAddrs)

	case "tui":
		listenAddr := ""
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
		return runTUI(localID, cfg, listenAddr, peerAddrs)

	default:
		if strings.HasPrefix(args[0], "-") {
			listenAddr := ""
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
			return runTUI(localID, cfg, listenAddr, peerAddrs)
		}
		return fmt.Errorf("unknown command %q\nRun 'skall --help' for usage", args[0])
	}
}

func printHelp(out io.Writer) {
	fmt.Fprintf(out, `SKALL — Secure, Decentralized P2P Chat (%s)

Usage:
  skall [flags]                             Launch interactive TUI (default)
  skall <command> [arguments...]

Commands:
  identity                                  Show local cryptographic identity & peer IDs
  identity set-name <display-name>          Update your display name
  peers                                     List known and connected peers
  connect <multiaddr|host:port>             Test and establish peer connection
  chat [peer-id|multiaddr]                  Start chat session targeting a peer
  groups [list]                             List all known groups
  groups create <id> [name]                 Create a new chat group
  config show                               Display active configuration
  config path                               Display path to config file
  config init                               Initialize default config file on disk
  version, --version, -v                    Print version information
  help, --help, -h                          Show this help message

Options:
  -l, --listen <port|multiaddr>             Specify listen address (e.g. 9001 or /ip4/0.0.0.0/tcp/9001)
  -p, --peer <multiaddr>                    Connect to peer on startup (repeatable)

Diagnostics / Advanced:
  p2p [--listen PORT] [--peer MULTIADDR]    CLI-only peer broadcast session
  server [address]                          Legacy TCP server
  client [address]                          Legacy TCP client
  discover [address]                        Legacy mDNS discovery daemon
`, version.Short())
}

func handleIdentityCommand(args []string, localID identity.Identity, cfg config.Config, out io.Writer) error {
	if len(args) >= 2 && args[0] == "set-name" {
		newName := strings.TrimSpace(strings.Join(args[1:], " "))
		if newName == "" {
			return errors.New("display name cannot be empty")
		}

		store, err := openIdentityStore(cfg)
		if err != nil {
			return fmt.Errorf("open identity store: %w", err)
		}

		localID.DisplayName = newName
		if err := store.Save(localID); err != nil {
			return fmt.Errorf("save identity: %w", err)
		}

		db, err := openStore(cfg)
		if err == nil {
			_ = db.UpsertIdentityMetadata(localID)
			_ = db.Close()
		}

		fmt.Fprintf(out, "Display name updated to: %s\n", newName)
		return nil
	}

	fmt.Fprintln(out, localID.Summary())
	if lp2pID, err := localID.LibP2PPeerID(); err == nil {
		fmt.Fprintf(out, "libp2p Peer ID: %s\n", lp2pID)
		fmt.Fprintf(out, "Sample Multiaddr (port 9001): /ip4/127.0.0.1/tcp/9001/p2p/%s\n", lp2pID)
	}
	return nil
}

func handlePeersCommand(cfg config.Config, out io.Writer) error {
	db, err := openStore(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	peers, err := db.ListPeers()
	if err != nil {
		return fmt.Errorf("query peers: %w", err)
	}

	if len(peers) == 0 {
		fmt.Fprintln(out, "No known peers found in local store.")
		return nil
	}

	fmt.Fprintf(out, "%-16s  %-20s  %-24s\n", "PEER ID", "DISPLAY NAME", "LAST SEEN")
	fmt.Fprintf(out, "%-16s  %-20s  %-24s\n", "----------------", "--------------------", "------------------------")
	for _, p := range peers {
		shortID := p.PeerID
		if len(shortID) > 16 {
			shortID = shortID[:16]
		}
		lastSeen := p.LastSeen.Format("2006-01-02 15:04:05")
		if p.LastSeen.IsZero() {
			lastSeen = "never"
		}
		fmt.Fprintf(out, "%-16s  %-20s  %-24s\n", shortID, p.DisplayName, lastSeen)
	}
	return nil
}

func handleConnectCommand(target string, localID identity.Identity, cfg config.Config, out io.Writer) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return errors.New("target address is required")
	}

	// Try parsing as multiaddr
	maddr, err := ma.NewMultiaddr(target)
	if err == nil {
		pi, err := libp2ppeer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			return fmt.Errorf("parse multiaddr: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		node, err := p2p.BuildNodeNoMDNS(ctx, localID, []string{"/ip4/0.0.0.0/tcp/0"})
		if err != nil {
			return fmt.Errorf("build p2p node: %w", err)
		}
		defer node.Close()

		if err := node.Connect(ctx, *pi); err != nil {
			return fmt.Errorf("connect to %s: %w", target, err)
		}

		// Save to SQLite
		if db, err := openStore(cfg); err == nil {
			_ = db.UpsertPeer(pi.ID.String(), pi.ID.String(), time.Now().UTC())
			_ = db.Close()
		}

		fmt.Fprintf(out, "Successfully connected to peer: %s\n", pi.ID)
		return nil
	}

	// Fall back to standard TCP dial validation
	conn, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", target, err)
	}
	_ = conn.Close()

	fmt.Fprintf(out, "TCP endpoint %s is reachable.\n", target)
	return nil
}

func handleGroupsCommand(args []string, cfg config.Config, out io.Writer) error {
	db, err := openStore(cfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if len(args) >= 2 && args[0] == "create" {
		groupID := args[1]
		groupName := groupID
		if len(args) > 2 {
			groupName = strings.Join(args[2:], " ")
		}

		if err := db.UpsertGroup(groupID, groupName, time.Now().UTC()); err != nil {
			return fmt.Errorf("create group: %w", err)
		}
		fmt.Fprintf(out, "Created group %q (%s)\n", groupName, groupID)
		return nil
	}

	groups, err := db.ListGroups()
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}

	if len(groups) == 0 {
		fmt.Fprintln(out, "No groups found. Create one with: skall groups create <id> [name]")
		return nil
	}

	fmt.Fprintf(out, "%-16s  %-24s  %-24s\n", "GROUP ID", "NAME", "CREATED AT")
	fmt.Fprintf(out, "%-16s  %-24s  %-24s\n", "----------------", "------------------------", "------------------------")
	for _, g := range groups {
		createdAt := g.CreatedAt.Format("2006-01-02 15:04:05")
		fmt.Fprintf(out, "%-16s  %-24s  %-24s\n", g.GroupID, g.Name, createdAt)
	}
	return nil
}

func handleConfigCommand(args []string, cfg config.Config, out io.Writer) error {
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}

	switch sub {
	case "path":
		path, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		fmt.Fprintln(out, path)
		return nil

	case "init":
		path, err := config.DefaultConfigPath()
		if err != nil {
			return err
		}
		defaultCfg := config.DefaultConfig()
		if err := config.Save(path, defaultCfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Fprintf(out, "Initialized configuration at %s\n", path)
		return nil

	case "show":
		fallthrough
	default:
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(data))
		return nil
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

func openStore(cfg config.Config) (*storage.Store, error) {
	if override := strings.TrimSpace(os.Getenv("SKALL_DB_PATH")); override != "" {
		return storage.Open(override)
	}
	if dataDir := strings.TrimSpace(os.Getenv("SKALL_DATA_DIR")); dataDir != "" {
		return storage.Open(filepath.Join(dataDir, "skall.db"))
	}
	if cfg.DataDir != "" {
		return storage.Open(filepath.Join(cfg.DataDir, "skall.db"))
	}
	return storage.OpenDefault()
}

func openIdentityStore(cfg config.Config) (*identity.Store, error) {
	if override := strings.TrimSpace(os.Getenv("SKALL_IDENTITY_PATH")); override != "" {
		return identity.NewStore(override), nil
	}
	if dataDir := strings.TrimSpace(os.Getenv("SKALL_DATA_DIR")); dataDir != "" {
		return identity.NewStore(filepath.Join(dataDir, "identity.json")), nil
	}
	if cfg.DataDir != "" {
		return identity.NewStore(filepath.Join(cfg.DataDir, "identity.json")), nil
	}
	return identity.DefaultStore()
}

func loadLocalIdentity(cfg config.Config) (identity.Identity, error) {
	store, err := openIdentityStore(cfg)
	if err != nil {
		return identity.Identity{}, err
	}

	localIdentity, err := store.LoadOrCreate()
	if err != nil {
		return identity.Identity{}, err
	}

	// Apply configuration overrides if defined
	if cfg.DisplayName != "" && cfg.DisplayName != localIdentity.DisplayName {
		localIdentity.DisplayName = cfg.DisplayName
		_ = store.Save(localIdentity)
	}
	if cfg.Username != "" && cfg.Username != localIdentity.Username {
		localIdentity.Username = cfg.Username
		_ = store.Save(localIdentity)
	}

	db, err := openStore(cfg)
	if err != nil {
		return identity.Identity{}, err
	}
	defer func() { _ = db.Close() }()

	if err := db.UpsertIdentityMetadata(localIdentity); err != nil {
		return identity.Identity{}, err
	}

	return localIdentity, nil
}

// runP2P starts a libp2p-backed SKALL node in CLI broadcast mode.
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

	lp2pID, err := localID.LibP2PPeerID()
	if err != nil {
		return fmt.Errorf("p2p: derive peer id: %w", err)
	}

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

	node.SetMessageHandler(func(msg protocol.Message, from libp2ppeer.ID) {
		fmt.Printf("\r[%s] %s: %s\n", msg.Timestamp.Format("15:04:05"), from.ShortString(), msg.Body)
	})

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

// runDiscover wires together TCP server, mDNS announce, browse, and connector.
func runDiscover(localID identity.Identity, address string) error {
	server := network.NewServer(address)
	if err := server.Listen(); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	fmt.Println("SKALL server listening on", server.Addr())

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

	go func() {
		if err := server.Serve(); err != nil {
			log.Println("server error:", err)
		}
	}()

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

	mgr := connection.NewManager(nil)
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

// runTUI starts the full Bubble Tea terminal UI.
func runTUI(localID identity.Identity, cfg config.Config, listenAddr string, peerAddrs []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var listenAddrs []string
	if listenAddr != "" && listenAddr != "/ip4/0.0.0.0/tcp/0" {
		listenAddrs = []string{listenAddr}
	} else if len(cfg.Network.ListenAddrs) > 0 && cfg.Network.ListenAddrs[0] != "/ip4/0.0.0.0/tcp/0" {
		listenAddrs = cfg.Network.ListenAddrs
	}

	// Merge bootstrap peers from config
	for _, bp := range cfg.Network.BootstrapPeers {
		if bp != "" {
			peerAddrs = append(peerAddrs, bp)
		}
	}

	node, err := p2p.BuildNode(ctx, localID, listenAddrs)
	if err != nil {
		return fmt.Errorf("tui: build p2p node: %w", err)
	}

	lp2pID, _ := localID.LibP2PPeerID()
	for _, addr := range node.Addrs() {
		log.Printf("skall: listening on %s/p2p/%s", addr, lp2pID)
	}

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

	db, err := openStore(cfg)
	if err != nil {
		_ = node.Close()
		return fmt.Errorf("tui: open store: %w", err)
	}

	grpMgr := groups.NewManager()

	// Restore persisted groups and their active memberships from SQLite so that
	// groups survive application restarts.
	if err := groups.LoadFromStore(db, grpMgr); err != nil {
		log.Printf("skall: restore groups from store: %v", err)
	}

	svc := chat.NewP2PService(ctx, localID, node, db, grpMgr)

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
