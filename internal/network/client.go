package network

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
	"github.com/Harish-vinayagam/Skall/internal/storage"
)

type clientConn struct {
	server      *Server
	conn        net.Conn
	remote      string
	peerID      string
	displayName string

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

		// Handle join messages to register peer identity
		if message.Type == protocol.TypeJoin {
			c.peerID = message.SenderID
			c.displayName = message.Body
			c.server.registerPeer(c, c.peerID)
		}

		fmt.Printf("%s: %s\n", c.remote, message.Body)
		c.server.broadcast(c, message)
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
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

func StartClient(address string, localIdentity identity.Identity) error {
	db, err := storage.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if err := db.UpsertIdentityMetadata(localIdentity); err != nil {
		return err
	}

	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("Connected to", address)
	writer := protocol.NewFrameWriter(conn)
	senderID := localIdentity.PeerID
	persistOutbound := func(payload protocol.Message) error {
		if err := db.InsertMessage(payload, storage.DirectionOutbound, storage.StatusPending); err != nil && !errors.Is(err, storage.ErrDuplicateMessage) {
			fmt.Println("Warning: failed to persist outgoing message:", err)
		}
		if err := writer.WriteMessage(payload); err != nil {
			if updateErr := db.UpdateMessageStatus(payload.ID, storage.StatusFailed); updateErr != nil && !errors.Is(updateErr, io.EOF) {
				fmt.Println("Warning: failed to mark outgoing message as failed:", updateErr)
			}
			return err
		}
		if updateErr := db.UpdateMessageStatus(payload.ID, storage.StatusSent); updateErr != nil {
			fmt.Println("Warning: failed to mark outgoing message as sent:", updateErr)
		}
		return nil
	}

	// announce ourselves right away
	now := time.Now().UTC()
	join := protocol.Message{
		Version:   protocol.Version,
		ID:        strconv.FormatInt(now.UnixNano(), 10),
		Type:      protocol.TypeJoin,
		SenderID:  senderID,
		Timestamp: now,
		Body:      localIdentity.DisplayName,
	}
	if err := persistOutbound(join); err != nil {
		return err
	}

	// local peer list
	peers := make(map[string]string)
	var peersMu sync.Mutex

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
			switch message.Type {
			case protocol.TypeJoin:
				peersMu.Lock()
				peers[message.SenderID] = message.Body
				peersMu.Unlock()
				if err := db.UpsertPeer(message.SenderID, message.Body, message.Timestamp); err != nil {
					fmt.Println("Warning: failed to persist peer join:", err)
				}
				if err := db.InsertMessage(message, storage.DirectionInbound, storage.StatusReceived); err != nil && !errors.Is(err, storage.ErrDuplicateMessage) {
					fmt.Println("Warning: failed to persist join message:", err)
				}
				fmt.Printf("Peer joined: %s (%s)\n", message.Body, message.SenderID)
			case protocol.TypeLeave:
				peersMu.Lock()
				delete(peers, message.SenderID)
				peersMu.Unlock()
				if err := db.UpsertPeer(message.SenderID, message.SenderID, message.Timestamp); err != nil {
					fmt.Println("Warning: failed to persist peer leave:", err)
				}
				if err := db.InsertMessage(message, storage.DirectionInbound, storage.StatusReceived); err != nil && !errors.Is(err, storage.ErrDuplicateMessage) {
					fmt.Println("Warning: failed to persist leave message:", err)
				}
				fmt.Printf("Peer left: %s (%s)\n", message.SenderID, message.Body)
			case protocol.TypeSystem:
				status := storage.StatusReceived
				if strings.Contains(strings.ToLower(message.Body), "delivery failed") {
					status = storage.StatusFailed
				}
				if err := db.InsertMessage(message, storage.DirectionInbound, status); err != nil && !errors.Is(err, storage.ErrDuplicateMessage) {
					fmt.Println("Warning: failed to persist system message:", err)
				}
				fmt.Printf("System: %s\n", message.Body)
			default:
				peersMu.Lock()
				name := peers[message.SenderID]
				peersMu.Unlock()
				if name == "" {
					name = message.SenderID
				}
				if err := db.UpsertPeer(message.SenderID, name, message.Timestamp); err != nil {
					fmt.Println("Warning: failed to persist peer metadata:", err)
				}
				if err := db.InsertMessage(message, storage.DirectionInbound, storage.StatusReceived); err != nil && !errors.Is(err, storage.ErrDuplicateMessage) {
					fmt.Println("Warning: failed to persist inbound message:", err)
				}
				fmt.Printf("%s: %s\n", name, message.Body)
			}
		}
	}()

	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		line := strings.TrimSpace(input.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			parts := strings.Fields(line)
			switch parts[0] {
			case "/peers":
				peersMu.Lock()
				if len(peers) == 0 {
					fmt.Println("No peers connected")
				} else {
					for id, name := range peers {
						fmt.Printf("%s (%s)\n", name, id)
					}
				}
				peersMu.Unlock()
			case "/msg":
				if len(parts) < 3 {
					fmt.Println("Usage: /msg <peer-id> <message>")
					continue
				}
				peer := parts[1]
				body := strings.TrimSpace(line[len(parts[0])+1+len(peer):])
				if body == "" {
					fmt.Println("empty message")
					continue
				}
				now := time.Now().UTC()
				payload := protocol.Message{
					Version:     protocol.Version,
					ID:          strconv.FormatInt(now.UnixNano(), 10),
					Type:        protocol.TypeChat,
					SenderID:    senderID,
					RecipientID: peer,
					Timestamp:   now,
					Body:        body,
				}
				if err := db.UpsertPeer(peer, peer, now); err != nil {
					fmt.Println("Warning: failed to persist target peer:", err)
				}
				if err := persistOutbound(payload); err != nil {
					return err
				}
			default:
				fmt.Println("unknown command")
			}
			continue
		}

		// default: send to everyone (broadcast)
		now := time.Now().UTC()
		payload := protocol.Message{
			Version:     protocol.Version,
			ID:          strconv.FormatInt(now.UnixNano(), 10),
			Type:        protocol.TypeChat,
			SenderID:    senderID,
			RecipientID: "",
			Timestamp:   now,
			Body:        line,
		}
		if err := persistOutbound(payload); err != nil {
			return err
		}
	}

	return input.Err()
}
