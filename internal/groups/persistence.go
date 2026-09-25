package groups

import (
	"fmt"
	"log"

	"github.com/Harish-vinayagam/Skall/internal/storage"
)

// LoadFromStore hydrates mgr with every group and its active memberships that
// are persisted in the SQLite store. It is intended to be called once at
// application startup, immediately after the Store and Manager are created.
//
// LoadFromStore is safe to call on an empty database (it becomes a no-op).
// If a group already exists in mgr it is skipped rather than overwritten, so
// the function is idempotent when called multiple times.
//
// Only memberships with active=true are restored; former members whose active
// flag was set to false by LeaveGroup / RemoveMember are deliberately excluded.
func LoadFromStore(store *storage.Store, mgr *Manager) error {
	groups, err := store.ListGroups()
	if err != nil {
		return fmt.Errorf("groups: load from store: list groups: %w", err)
	}

	for _, sg := range groups {
		// Skip groups that are already in the manager (e.g. if called twice).
		if _, exists := mgr.GetGroup(sg.GroupID); exists {
			continue
		}

		// Restore the group record into the manager.
		// We bypass CreateGroup so we can set the original CreatedAt timestamp
		// instead of time.Now().
		mgr.mu.Lock()
		mgr.groups[sg.GroupID] = &Group{
			ID:        sg.GroupID,
			Name:      sg.Name,
			Members:   make(map[string]GroupMember),
			CreatedAt: sg.CreatedAt,
			UpdatedAt: sg.UpdatedAt,
		}
		mgr.seen[sg.GroupID] = make(map[string]struct{})
		mgr.mu.Unlock()

		// Restore active memberships.
		members, err := store.ListGroupMembers(sg.GroupID)
		if err != nil {
			// Log but continue — a single bad group should not abort the whole restore.
			log.Printf("groups: load from store: list members for %q: %v", sg.GroupID, err)
			continue
		}

		for _, m := range members {
			if !m.Active {
				continue // inactive members are former members; do not restore
			}
			// Use the internal helper so we can set the original JoinedAt/LastSeen.
			mgr.mu.Lock()
			group := mgr.groups[sg.GroupID]
			if group != nil {
				if _, alreadyMember := group.Members[m.PeerID]; !alreadyMember {
					group.Members[m.PeerID] = GroupMember{
						PeerID:   m.PeerID,
						JoinedAt: m.JoinedAt,
						Active:   true,
						LastSeen: m.LastSeen,
					}
				}
			}
			mgr.mu.Unlock()
		}
	}

	return nil
}
