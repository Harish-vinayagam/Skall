package network

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

type Server struct {
	address string

	mu       sync.RWMutex
	listener net.Listener
	clients  map[*clientConn]struct{}
	registry *PeerRegistry
	router   *MessageRouter

	shutdownOnce sync.Once
	wg           sync.WaitGroup
	closed       bool
}

func NewServer(address string) *Server {
	registry := NewPeerRegistry()
	return &Server{
		address:  address,
		clients:  make(map[*clientConn]struct{}),
		registry: registry,
		router:   NewMessageRouter(registry),
		listener: nil,
	}
}

func (s *Server) Listen() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	return nil
}

func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.listener == nil {
		return s.address
	}

	return s.listener.Addr().String()
}

func (s *Server) ActiveConnections() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.clients)
}

func (s *Server) Serve() error {
	s.mu.RLock()
	listener := s.listener
	s.mu.RUnlock()

	if listener == nil {
		return errors.New("network server is not listening")
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}

			s.mu.RLock()
			closed := s.closed
			s.mu.RUnlock()
			if closed {
				return nil
			}

			return err
		}

		client := newClientConn(s, conn)
		s.addClient(client)
		fmt.Println("Client connected:", conn.RemoteAddr())

		s.wg.Add(2)
		go client.readLoop()
		go client.writeLoop()
	}
}

func (s *Server) Shutdown() error {
	var closeErr error

	s.shutdownOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		listener := s.listener
		clients := make([]*clientConn, 0, len(s.clients))
		for client := range s.clients {
			clients = append(clients, client)
		}
		s.mu.Unlock()

		if listener != nil {
			if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				closeErr = err
			}
		}

		for _, client := range clients {
			client.close()
		}
	})

	s.wg.Wait()
	return closeErr
}

func (s *Server) addClient(client *clientConn) {
	s.mu.Lock()
	s.clients[client] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) removeClient(client *clientConn) {
	s.mu.Lock()
	if _, exists := s.clients[client]; exists {
		delete(s.clients, client)
	}
	s.mu.Unlock()

	if peerID, ok := s.registry.Unregister(client); ok {
		s.notifyPeerDisconnected(peerID)
	}

	client.close()
}

func (s *Server) handleIncoming(client *clientConn, message protocol.Message) {
	switch message.Type {
	case protocol.TypeJoin:
		s.handleJoin(client, message)
	case protocol.TypeChat:
		s.handleDirectChat(client, message)
	case protocol.TypeSystem:
		s.handleSystem(client, message)
	default:
		s.sendSystem(client, "unsupported message type")
	}
}

func (s *Server) handleJoin(client *clientConn, message protocol.Message) {
	if err := s.registry.Register(message.SenderID, client); err != nil {
		s.sendSystem(client, "failed to identify peer: "+err.Error())
		return
	}

	s.sendSystem(client, "identified as "+message.SenderID)
	s.notifyPeerConnected(message.SenderID)
}

func (s *Server) handleDirectChat(client *clientConn, message protocol.Message) {
	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router == nil {
		s.sendSystem(client, "routing unavailable")
		return
	}

	err := router.RouteDirect(client, message)
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, ErrMissingRecipient):
		s.sendSystem(client, "delivery failed: recipient is required")
	case errors.Is(err, ErrUnknownPeer):
		s.sendSystem(client, "delivery failed: peer "+message.RecipientID+" is offline or unknown")
	case errors.Is(err, ErrPeerDisconnected):
		s.sendSystem(client, "delivery failed: peer "+message.RecipientID+" disconnected")
	case errors.Is(err, ErrSenderNotIdentified):
		s.sendSystem(client, "identify first before sending messages")
	case errors.Is(err, ErrSenderMismatch):
		s.sendSystem(client, "delivery failed: sender identity mismatch")
	default:
		s.sendSystem(client, "delivery failed")
	}
}

func (s *Server) handleSystem(client *clientConn, message protocol.Message) {
	if strings.TrimSpace(message.Body) != "/peers" {
		s.sendSystem(client, "unknown command")
		return
	}

	senderPeerID, ok := s.registry.PeerID(client)
	if !ok {
		s.sendSystem(client, "identify first before requesting peers")
		return
	}

	peers := s.registry.List(senderPeerID)
	if len(peers) == 0 {
		s.sendSystem(client, "Connected peers: (none)")
		return
	}

	s.sendSystem(client, "Connected peers: "+strings.Join(peers, ", "))
}

func (s *Server) sendSystem(client *clientConn, body string) {
	systemMessage := protocol.NewSystemMessage("server", "", body)
	if !client.enqueue(systemMessage) {
		s.removeClient(client)
	}
}

func (s *Server) notifyPeerConnected(peerID string) {
	notice := protocol.NewJoinMessage("server", "peer connected: "+peerID)
	s.broadcastSystem(notice, peerID)
}

func (s *Server) notifyPeerDisconnected(peerID string) {
	notice := protocol.NewLeaveMessage("server", "peer disconnected: "+peerID)
	s.broadcastSystem(notice, peerID)
}

func (s *Server) broadcastSystem(message protocol.Message, excludePeerID string) {
	peers := s.registry.List(excludePeerID)
	for _, peerID := range peers {
		recipient, ok := s.registry.Lookup(peerID)
		if !ok {
			continue
		}
		if !recipient.enqueue(message) {
			if conn, ok := recipient.(*clientConn); ok {
				s.removeClient(conn)
				continue
			}
			s.registry.Unregister(recipient)
		}
	}
}

func StartServer(address string, localIdentity identity.Identity) error {
	server := NewServer(address)
	if err := server.Listen(); err != nil {
		return err
	}

	fmt.Println("SKALL server listening on", server.Addr())
	fmt.Println("Local identity:", localIdentity.DisplayName, "(", localIdentity.PeerID, ")")

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve()
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	select {
	case err := <-errCh:
		return err
	case sig := <-signalCh:
		fmt.Println("Shutting down on", sig)
		if err := server.Shutdown(); err != nil {
			return err
		}
		return <-errCh
	}
}
