package network

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type Server struct {
	address string

	mu       sync.RWMutex
	listener net.Listener
	clients  map[*clientConn]struct{}

	shutdownOnce sync.Once
	wg           sync.WaitGroup
	closed       bool
}

func NewServer(address string) *Server {
	return &Server{
		address:  address,
		clients:  make(map[*clientConn]struct{}),
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

	client.close()
}

func (s *Server) broadcast(sender *clientConn, message string) {
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

func StartServer(address string) error {
	server := NewServer(address)
	if err := server.Listen(); err != nil {
		return err
	}

	fmt.Println("SKALL server listening on", server.Addr())

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
