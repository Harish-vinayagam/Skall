package network

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"github.com/Harish-vinayagam/Skall/internal/chat"
	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

type clientConn struct {
	server *Server
	conn   net.Conn
	remote string

	writeCh chan protocol.Message
	closed  chan struct{}
	once    sync.Once
	closedN int32
}

func newClientConn(server *Server, conn net.Conn) *clientConn {
	return &clientConn{
		server:  server,
		conn:    conn,
		remote:  conn.RemoteAddr().String(),
		writeCh: make(chan protocol.Message, 16),
		closed:  make(chan struct{}),
	}
}

func (c *clientConn) readLoop() {
	defer c.server.wg.Done()
	defer c.server.removeClient(c)

	reader := protocol.NewFrameReader(c.conn)
	for {
		message, err := reader.ReadMessage()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			fmt.Println("Client read error:", c.remote, err)
			return
		}

		if err := message.Validate(); err != nil {
			fmt.Println("Rejected message from", c.remote, ":", err)
			continue
		}

		c.server.handleIncoming(c, message)
	}
}

func (c *clientConn) writeLoop() {
	defer c.server.wg.Done()
	defer c.server.removeClient(c)

	writer := protocol.NewFrameWriter(c.conn)
	for {
		select {
		case message, ok := <-c.writeCh:
			if !ok {
				return
			}

			if err := writer.WriteMessage(message); err != nil {
				fmt.Println("Client write error:", c.remote, err)
				return
			}
		case <-c.closed:
			return
		}
	}
}

func (c *clientConn) enqueue(message protocol.Message) bool {
	if atomic.LoadInt32(&c.closedN) == 1 {
		return false
	}

	select {
	case c.writeCh <- message:
		return true
	default:
		return false
	}
}

func (c *clientConn) close() {
	c.once.Do(func() {
		atomic.StoreInt32(&c.closedN, 1)
		close(c.closed)
		close(c.writeCh)
		_ = c.conn.Close()
	})
}

func StartClient(address string, localIdentity identity.Identity) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("Connected to", address)
	fmt.Println("Commands: /peers, /msg <peer> <message>")
	writer := newSafeFrameSender(conn)
	chatService := chat.NewService(localIdentity.PeerID, writer)
	if err := chatService.Start(); err != nil {
		return err
	}

	go func() {
		reader := protocol.NewFrameReader(conn)
		for {
			message, err := reader.ReadMessage()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
					return
				}
				fmt.Println("Client receive error:", err)
				return
			}

			fmt.Println(chat.FormatIncoming(message))
		}
	}()

	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		text := input.Text()
		if err := chatService.HandleInput(text); err != nil {
			fmt.Println("[system]", err)
		}
	}

	return input.Err()
}

type safeFrameSender struct {
	mu     sync.Mutex
	writer *protocol.FrameWriter
}

func newSafeFrameSender(conn net.Conn) *safeFrameSender {
	return &safeFrameSender{
		writer: protocol.NewFrameWriter(conn),
	}
}

func (s *safeFrameSender) Send(message protocol.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writer.WriteMessage(message)
}
