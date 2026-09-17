package groups

import (
	"errors"
	"fmt"
	"sync"
	"testing"

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
}

func TestCreateGroup_EmptyID(t *testing.T) {
	mgr := NewManager()
	if _, err := mgr.CreateGroup("", "Friends"); !errors.Is(err, ErrInvalidGroupID) {
		t.Fatalf("expected ErrInvalidGroupID, got %v", err)
	}
}

func TestCreateGroup_EmptyName(t *testing.T) {
	mgr := NewManager()
	if _, err := mgr.CreateGroup("g-1", "   "); !errors.Is(err, ErrInvalidGroupName) {
		t.Fatalf("expected ErrInvalidGroupName, got %v", err)
	}
}

func TestCreateGroup_Duplicate(t *testing.T) {
	mgr := NewManager()
	if _, err := mgr.CreateGroup("g-1", "Friends"); err != nil {
		t.Fatalf("CreateGroup error: %v", err)
	}
	if _, err := mgr.CreateGroup("g-1", "Friends 2"); !errors.Is(err, ErrGroupAlreadyExists) {
		t.Fatalf("expected ErrGroupAlreadyExists, got %v", err)
	}
}

func TestAddMember_DuplicatePeer(t *testing.T) {
	mgr := NewManager()
	if _, err := mgr.CreateGroup("g-1", "Friends"); err != nil {
		t.Fatalf("CreateGroup error: %v", err)
	}
	if err := mgr.AddMember("g-1", "alice"); err != nil {
		t.Fatalf("AddMember error: %v", err)
	}
	if err := mgr.AddMember("g-1", "alice"); !errors.Is(err, ErrMemberAlreadyAdded) {
		t.Fatalf("expected ErrMemberAlreadyAdded, got %v", err)
	}
}

func TestRemoveMember_NonexistentPeer(t *testing.T) {
	mgr := NewManager()
	if _, err := mgr.CreateGroup("g-1", "Friends"); err != nil {
		t.Fatalf("CreateGroup error: %v", err)
	}
	if err := mgr.RemoveMember("g-1", "nonexistent"); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("expected ErrMemberNotFound, got %v", err)
	}
}

func TestSendGroupMessage_EmptyBody(t *testing.T) {
	mgr := NewManager()
	if _, err := mgr.CreateGroup("g-1", "Friends"); err != nil {
		t.Fatalf("CreateGroup error: %v", err)
	}
	if err := mgr.AddMember("g-1", "alice"); err != nil {
		t.Fatalf("AddMember error: %v", err)
	}
	if _, err := mgr.SendGroupMessage("g-1", "alice", "   "); err == nil {
		t.Fatal("expected error for empty group message body")
	}
}

func TestIsDuplicate_UnknownGroup(t *testing.T) {
	mgr := NewManager()
	if mgr.IsDuplicate("unknown-group", "msg-1") {
		t.Fatal("expected IsDuplicate to return false for unknown group")
	}
}

func TestRouteGroupMessage_NilDeliverFn(t *testing.T) {
	mgr := NewManager()
	if _, err := mgr.CreateGroup("g-1", "Friends"); err != nil {
		t.Fatalf("CreateGroup error: %v", err)
	}
	if err := mgr.AddMember("g-1", "alice"); err != nil {
		t.Fatalf("AddMember error: %v", err)
	}
	if err := mgr.AddMember("g-1", "bob"); err != nil {
		t.Fatalf("AddMember error: %v", err)
	}

	msg, err := mgr.SendGroupMessage("g-1", "alice", "hi")
	if err != nil {
		t.Fatalf("SendGroupMessage error: %v", err)
	}

	delivered, err := mgr.RouteGroupMessage(msg, map[string]bool{"bob": true}, nil)
	if err != nil {
		t.Fatalf("RouteGroupMessage error with nil deliver: %v", err)
	}
	if len(delivered) != 0 {
		t.Fatalf("expected 0 delivered with nil deliver fn, got %d", len(delivered))
	}
}

func TestGetGroup_FoundAndNotFound(t *testing.T) {
	mgr := NewManager()
	_, found := mgr.GetGroup("nonexistent")
	if found {
		t.Fatal("expected GetGroup to return false for nonexistent group")
	}

	_, err := mgr.CreateGroup("g-1", "Alpha")
	if err != nil {
		t.Fatalf("CreateGroup error: %v", err)
	}
	grp, found := mgr.GetGroup("g-1")
	if !found {
		t.Fatal("expected GetGroup to find g-1")
	}
	if grp.Name != "Alpha" {
		t.Fatalf("expected group name Alpha, got %s", grp.Name)
	}
}

func TestListGroups_EmptyAndPopulated(t *testing.T) {
	mgr := NewManager()
	if len(mgr.ListGroups()) != 0 {
		t.Fatal("expected empty group list")
	}
	_, _ = mgr.CreateGroup("g-2", "Two")
	_, _ = mgr.CreateGroup("g-1", "One")
	list := mgr.ListGroups()
	if len(list) != 2 || list[0] != "g-1" || list[1] != "g-2" {
		t.Fatalf("expected sorted [g-1, g-2], got %v", list)
	}
}

func TestConcurrentGroupOperations(t *testing.T) {
	mgr := NewManager()
	_, err := mgr.CreateGroup("g-concurrent", "Concurrent Test")
	if err != nil {
		t.Fatalf("CreateGroup error: %v", err)
	}

	const count = 20
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			peerID := fmt.Sprintf("peer-%d", idx)
			_ = mgr.AddMember("g-concurrent", peerID)
			_ = mgr.IsMember("g-concurrent", peerID)
			_ = mgr.ListGroups()
			_, _ = mgr.ViewMembers("g-concurrent")

			msg := protocol.NewChatMessage(peerID, "", "g-concurrent", fmt.Sprintf("msg from %d", idx))
			_, _ = mgr.RouteGroupMessage(msg, map[string]bool{peerID: true}, func(m protocol.Message, p string) error {
				return nil
			})
			_ = mgr.MarkMessageSeen("g-concurrent", fmt.Sprintf("seen-%d", idx))
			_ = mgr.IsDuplicate("g-concurrent", fmt.Sprintf("seen-%d", idx))

			if idx%3 == 0 {
				_ = mgr.RemoveMember("g-concurrent", peerID)
			}
		}(i)
	}
	wg.Wait()
}
