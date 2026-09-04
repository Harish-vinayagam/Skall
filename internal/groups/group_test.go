package groups

import (
	"testing"
	"time"

	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

func TestGroupCreation(t *testing.T) {
	manager := NewManager()
	group, err := manager.CreateGroup("g-1", "Friends")
	if err != nil {
		t.Fatalf("CreateGroup() unexpected error: %v", err)
	}
	if group.ID != "g-1" {
		t.Fatalf("group ID mismatch: got %q want %q", group.ID, "g-1")
	}
	if group.Name != "Friends" {
		t.Fatalf("group name mismatch: got %q want %q", group.Name, "Friends")
	}
	if len(group.Members) != 0 {
		t.Fatalf("new group should start empty, got %d members", len(group.Members))
	}
}

func TestMembershipChanges(t *testing.T) {
	manager := NewManager()
	if _, err := manager.CreateGroup("g-2", "Team"); err != nil {
		t.Fatalf("CreateGroup() unexpected error: %v", err)
	}
	if err := manager.AddMember("g-2", "alice"); err != nil {
		t.Fatalf("AddMember() unexpected error: %v", err)
	}
	if err := manager.AddMember("g-2", "bob"); err != nil {
		t.Fatalf("AddMember() unexpected error: %v", err)
	}
	members, err := manager.ViewMembers("g-2")
	if err != nil {
		t.Fatalf("ViewMembers() unexpected error: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d: %#v", len(members), members)
	}
	if err := manager.RemoveMember("g-2", "alice"); err != nil {
		t.Fatalf("RemoveMember() unexpected error: %v", err)
	}
	if manager.IsMember("g-2", "alice") {
		t.Fatal("removed member still appears active")
	}
	if err := manager.LeaveGroup("g-2", "bob"); err != nil {
		t.Fatalf("LeaveGroup() unexpected error: %v", err)
	}
	if manager.IsMember("g-2", "bob") {
		t.Fatal("left member still appears active")
	}
}

func TestGroupMessageRouting(t *testing.T) {
	manager := NewManager()
	if _, err := manager.CreateGroup("g-3", "Ops"); err != nil {
		t.Fatalf("CreateGroup() unexpected error: %v", err)
	}
	for _, peerID := range []string{"alice", "bob", "charlie"} {
		if err := manager.AddMember("g-3", peerID); err != nil {
			t.Fatalf("AddMember(%s) unexpected error: %v", peerID, err)
		}
	}

	msg, err := manager.SendGroupMessage("g-3", "alice", "hello team")
	if err != nil {
		t.Fatalf("SendGroupMessage() unexpected error: %v", err)
	}
	if msg.GroupID != "g-3" {
		t.Fatalf("wrong group id in message: got %q want %q", msg.GroupID, "g-3")
	}

	delivered := make([]string, 0)
	connected := map[string]bool{"alice": true, "bob": true, "charlie": false}
	res, err := manager.RouteGroupMessage(msg, connected, func(m protocol.Message, peerID string) error {
		delivered = append(delivered, peerID)
		return nil
	})
	if err != nil {
		t.Fatalf("RouteGroupMessage() unexpected error: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 delivered peer, got %d: %#v", len(res), res)
	}
	if len(delivered) != 1 || delivered[0] != "bob" {
		t.Fatalf("unexpected connected peers delivered: %#v", delivered)
	}
	if len(res) != len(delivered) || res[0] != delivered[0] {
		t.Fatalf("route result did not match delivered set: res=%#v delivered=%#v", res, delivered)
	}
}

func TestRemovedMembersCannotReceive(t *testing.T) {
	manager := NewManager()
	if _, err := manager.CreateGroup("g-4", "Support"); err != nil {
		t.Fatalf("CreateGroup() unexpected error: %v", err)
	}
	for _, peerID := range []string{"alice", "bob"} {
		if err := manager.AddMember("g-4", peerID); err != nil {
			t.Fatalf("AddMember(%s) unexpected error: %v", peerID, err)
		}
	}
	if err := manager.RemoveMember("g-4", "bob"); err != nil {
		t.Fatalf("RemoveMember() unexpected error: %v", err)
	}

	msg, err := manager.SendGroupMessage("g-4", "alice", "only alice")
	if err != nil {
		t.Fatalf("SendGroupMessage() unexpected error: %v", err)
	}
	delivered := make([]string, 0)
	_, err = manager.RouteGroupMessage(msg, map[string]bool{"alice": true, "bob": true}, func(m protocol.Message, peerID string) error {
		delivered = append(delivered, peerID)
		return nil
	})
	if err != nil {
		t.Fatalf("RouteGroupMessage() unexpected error: %v", err)
	}
	if len(delivered) != 0 {
		t.Fatalf("removed member should not receive message: %#v", delivered)
	}
}

func TestUnknownGroup(t *testing.T) {
	manager := NewManager()
	if _, err := manager.SendGroupMessage("missing", "alice", "nope"); err == nil {
		t.Fatal("expected unknown group error")
	}
	if err := manager.AddMember("missing", "alice"); err == nil {
		t.Fatal("expected unknown group error when adding member")
	}
}

func TestDuplicateMessages(t *testing.T) {
	manager := NewManager()
	if _, err := manager.CreateGroup("g-5", "Audit"); err != nil {
		t.Fatalf("CreateGroup() unexpected error: %v", err)
	}
	if err := manager.AddMember("g-5", "alice"); err != nil {
		t.Fatalf("AddMember() unexpected error: %v", err)
	}
	if err := manager.AddMember("g-5", "bob"); err != nil {
		t.Fatalf("AddMember() unexpected error: %v", err)
	}

	msg, err := manager.SendGroupMessage("g-5", "alice", "hello")
	if err != nil {
		t.Fatalf("SendGroupMessage() unexpected error: %v", err)
	}
	if err := manager.MarkMessageSeen("g-5", msg.ID); err != nil {
		t.Fatalf("MarkMessageSeen() unexpected error: %v", err)
	}
	if manager.IsDuplicate("g-5", msg.ID) == false {
		t.Fatal("expected message ID to be flagged as duplicate after marking seen")
	}

	delivered := make([]string, 0)
	_, err = manager.RouteGroupMessage(msg, map[string]bool{"alice": true, "bob": true}, func(m protocol.Message, peerID string) error {
		delivered = append(delivered, peerID)
		return nil
	})
	if err == nil {
		t.Fatal("expected duplicate group message to be rejected")
	}
	if len(delivered) != 0 {
		t.Fatalf("duplicate message should not be routed: %#v", delivered)
	}
}

func TestDisconnectedPeerHandling(t *testing.T) {
	manager := NewManager()
	if _, err := manager.CreateGroup("g-6", "Net"); err != nil {
		t.Fatalf("CreateGroup() unexpected error: %v", err)
	}
	for _, peerID := range []string{"alice", "bob"} {
		if err := manager.AddMember("g-6", peerID); err != nil {
			t.Fatalf("AddMember(%s) unexpected error: %v", peerID, err)
		}
	}

	msg, err := manager.SendGroupMessage("g-6", "alice", "offline")
	if err != nil {
		t.Fatalf("SendGroupMessage() unexpected error: %v", err)
	}
	connected := map[string]bool{"alice": true, "bob": false}
	delivered := make([]string, 0)
	res, err := manager.RouteGroupMessage(msg, connected, func(m protocol.Message, peerID string) error {
		if connected[peerID] {
			delivered = append(delivered, peerID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RouteGroupMessage() unexpected error: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("sender should not be included in routed recipients: %#v", res)
	}
	if len(delivered) != 0 {
		t.Fatalf("disconnected peers should be skipped and sender excluded: %#v", delivered)
	}
	if _, ok := manager.groups["g-6"]; !ok {
		t.Fatal("group missing from manager after routing")
	}
	_ = time.Second
}
