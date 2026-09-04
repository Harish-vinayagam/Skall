package network

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

type Server struct {
	address string

	mu       sync.RWMutex
	listener net.Listener
	clients  map[*clientConn]struct{}
	peers    map[string]*clientConn

	shutdownOnce sync.Once
	wg           sync.WaitGroup
	closed       bool
}

func NewServer(address string) *Server {
	return &Server{
		address:  address,
		clients:  make(map[*clientConn]struct{}),
		peers:    make(map[string]*clientConn),
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
	// remove from peer map if registered
	if client.peerID != "" {
		if p, ok := s.peers[client.peerID]; ok && p == client {
			delete(s.peers, client.peerID)
		}
	}
	s.mu.Unlock()

	client.close()
}

func (s *Server) broadcast(sender *clientConn, message protocol.Message) {
	// If a recipient is specified, route only to that peer
	if strings.TrimSpace(message.RecipientID) != "" {
		s.mu.RLock()
		recipient, exists := s.peers[message.RecipientID]
		s.mu.RUnlock()

		if !exists || recipient == nil {
			// inform sender of failure
			sys := protocol.Message{
				Version:   protocol.Version,
				ID:        strconv.FormatInt(time.Now().UTC().UnixNano(), 10),
				Type:      protocol.TypeSystem,
				SenderID:  "server",
				RecipientID: message.SenderID,
				Timestamp: time.Now().UTC(),
				Body:      fmt.Sprintf("delivery failed: peer %s unknown or offline", message.RecipientID),
			}
			_ = sender.enqueue(sys)
			return
		}

		if !recipient.enqueue(message) {
			s.removeClient(recipient)
			sys := protocol.Message{
				Version:   protocol.Version,
				ID:        strconv.FormatInt(time.Now().UTC().UnixNano(), 10),
				Type:      protocol.TypeSystem,
				SenderID:  "server",
				RecipientID: message.SenderID,
				Timestamp: time.Now().UTC(),
				Body:      fmt.Sprintf("delivery failed: peer %s disconnected", message.RecipientID),
			}
			_ = sender.enqueue(sys)
		}
		return
	}

	// broadcast to all except sender
	s.mu.RLock()
	recipients := make([]*clientConn, 0, len(s.clients))
	for client := range s.clients {
		if client != sender {
			recipients = append(recipients, client)
		}
	}
	s.mu.RUnlock()

	for _, client := range recipients {
		if !client.enqueue(message) {
			s.removeClient(client)
		}
	}
}

func (s *Server) registerPeer(client *clientConn, peerID string) {
	s.mu.Lock()
	s.peers[peerID] = client
	s.mu.Unlock()
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
