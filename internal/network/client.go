package network

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
)

type clientConn struct {
	server *Server
	conn   net.Conn
	remote string

	writeCh chan string
	closed  chan struct{}
	once    sync.Once
	closedN int32
}

func newClientConn(server *Server, conn net.Conn) *clientConn {
	return &clientConn{
		server:  server,
		conn:    conn,
		remote:  conn.RemoteAddr().String(),
		writeCh: make(chan string, 16),
		closed:  make(chan struct{}),
	}
}

func (c *clientConn) readLoop() {
	defer c.server.wg.Done()
	defer c.server.removeClient(c)

	scanner := bufio.NewScanner(c.conn)
	for scanner.Scan() {
		message := scanner.Text()
		fmt.Printf("%s: %s\n", c.remote, message)
		c.server.broadcast(c, message)
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Client read error:", c.remote, err)
	}
}

func (c *clientConn) writeLoop() {
	defer c.server.wg.Done()
	defer c.server.removeClient(c)

	for {
		select {
		case message, ok := <-c.writeCh:
			if !ok {
				return
			}

			if _, err := fmt.Fprintln(c.conn, message); err != nil {
				fmt.Println("Client write error:", c.remote, err)
				return
			}
		case <-c.closed:
			return
		}
	}
}

func (c *clientConn) enqueue(message string) bool {
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

func StartClient(address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("Connected to", address)

	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Println("Server:", scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			fmt.Println("Client receive error:", err)
		}
	}()

	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		message := input.Text()
		if _, err := fmt.Fprintln(conn, message); err != nil {
			return err
		}
	}

	return input.Err()
}
