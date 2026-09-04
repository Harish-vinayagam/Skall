package chat

import (
	"fmt"
	"strings"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

type Sender interface {
	Send(protocol.Message) error
}

type Service struct {
	localPeerID string
	sender      Sender
}

func NewService(localPeerID string, sender Sender) *Service {
	return &Service{
		localPeerID: localPeerID,
		sender:      sender,
	}
}

func (s *Service) Start() error {
	return s.sender.Send(protocol.NewJoinMessage(s.localPeerID, "join"))
}

func (s *Service) HandleInput(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	if line == "/peers" {
		return s.sender.Send(protocol.NewSystemMessage(s.localPeerID, "server", "/peers"))
	}

	if strings.HasPrefix(line, "/msg ") {
		args := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(line, "/msg ")), " ", 2)
		if len(args) != 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
			return fmt.Errorf("usage: /msg <peer> <message>")
		}
		return s.sender.Send(protocol.NewChatMessage(s.localPeerID, strings.TrimSpace(args[0]), "", strings.TrimSpace(args[1])))
	}

	if strings.HasPrefix(line, "/") {
		return fmt.Errorf("unknown command: %s", line)
	}

	return fmt.Errorf("use /msg <peer> <message> to send direct messages")
}

func FormatIncoming(message protocol.Message) string {
	switch message.Type {
	case protocol.TypeChat:
		return fmt.Sprintf("[%s] %s", message.SenderID, message.Body)
	case protocol.TypeLeave:
		return fmt.Sprintf("[system] %s", message.Body)
	case protocol.TypeJoin:
		return fmt.Sprintf("[system] %s", message.Body)
	case protocol.TypeSystem:
		return fmt.Sprintf("[system] %s", message.Body)
	default:
		return fmt.Sprintf("[%s] %s", message.SenderID, message.Body)
	}
}
