package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/Harish-vinayagam/Skall/internal/identity"
	"github.com/Harish-vinayagam/Skall/internal/protocol"
)

const currentSchemaVersion = 1

type MessageDirection string

type MessageStatus string

const (
	DirectionInbound  MessageDirection = "inbound"
	DirectionOutbound MessageDirection = "outbound"

	StatusPending   MessageStatus = "pending"
	StatusSent      MessageStatus = "sent"
	StatusReceived  MessageStatus = "received"
	StatusDelivered MessageStatus = "delivered"
	StatusFailed    MessageStatus = "failed"
)

var (
	ErrDuplicateMessage  = errors.New("duplicate message id")
	ErrCorruptedDatabase = errors.New("database appears to be corrupted")
)

type Store struct {
	path      string
	db        *sql.DB
	closeOnce sync.Once
}

type Peer struct {
	PeerID      string
	DisplayName string
	LastSeen    time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Group struct {
	GroupID   string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type GroupMembership struct {
	GroupID   string
	PeerID    string
	JoinedAt  time.Time
	Active    bool
	LastSeen  time.Time
	UpdatedAt time.Time
}

type StoredMessage struct {
	Message   protocol.Message
	Direction MessageDirection
	Status    MessageStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type migration struct {
	version int
	name    string
	upSQL   []string
}

var migrations = []migration{
	{
		version: 1,
		name:    "init_core_tables",
		upSQL: []string{
			`CREATE TABLE IF NOT EXISTS identities (
				peer_id TEXT PRIMARY KEY,
				username TEXT NOT NULL,
				display_name TEXT NOT NULL,
				created_at TEXT NOT NULL,
				public_key BLOB NOT NULL,
				last_seen TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);`,
			`CREATE TABLE IF NOT EXISTS peers (
				peer_id TEXT PRIMARY KEY,
				display_name TEXT NOT NULL,
				last_seen TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);`,
			`CREATE TABLE IF NOT EXISTS groups (
				group_id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);`,
			`CREATE TABLE IF NOT EXISTS group_memberships (
				group_id TEXT NOT NULL,
				peer_id TEXT NOT NULL,
				joined_at TEXT NOT NULL,
				active INTEGER NOT NULL DEFAULT 1,
				last_seen TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				PRIMARY KEY (group_id, peer_id),
				FOREIGN KEY (group_id) REFERENCES groups(group_id) ON DELETE CASCADE
			);`,
			`CREATE TABLE IF NOT EXISTS messages (
				message_id TEXT PRIMARY KEY,
				version INTEGER NOT NULL,
				type TEXT NOT NULL,
				sender_id TEXT NOT NULL,
				recipient_id TEXT,
				group_id TEXT,
				timestamp TEXT NOT NULL,
				body TEXT NOT NULL,
				direction TEXT NOT NULL,
				status TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_ts ON messages(timestamp);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_direct ON messages(sender_id, recipient_id, timestamp);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_group ON messages(group_id, timestamp);`,
		},
	},
}

func DefaultPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("SKALL_DB_PATH")); override != "" {
		return override, nil
	}
	if dataDir := strings.TrimSpace(os.Getenv("SKALL_DATA_DIR")); dataDir != "" {
		return filepath.Join(dataDir, "skall.db"), nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}

	return filepath.Join(configDir, "skall", "skall.db"), nil
}

func OpenDefault() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("database path is required")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	dsn := path + "?_foreign_keys=on&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	store := &Store{path: path, db: db}
	if err := store.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		_ = os.Chmod(path, 0o600)
	}
	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		if s.db != nil {
			closeErr = s.db.Close()
		}
	})
	return closeErr
}

func (s *Store) initialize() error {
	if _, err := s.db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		return classifyDBError("enable foreign keys", err)
	}
	if _, err := s.db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		return classifyDBError("enable WAL", err)
	}
	if _, err := s.db.Exec(`PRAGMA synchronous = NORMAL;`); err != nil {
		return classifyDBError("set synchronous mode", err)
	}

	if err := s.quickCheck(); err != nil {
		return err
	}
	if err := s.applyMigrations(); err != nil {
		return err
	}
	return nil
}

func (s *Store) quickCheck() error {
	var result string
	if err := s.db.QueryRow(`PRAGMA quick_check(1);`).Scan(&result); err != nil {
		return classifyDBError("quick check database", err)
	}
	if !strings.EqualFold(strings.TrimSpace(result), "ok") {
		return fmt.Errorf("%w: %s", ErrCorruptedDatabase, result)
	}
	return nil
}

func (s *Store) applyMigrations() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	);`); err != nil {
		return classifyDBError("create migration table", err)
	}

	var currentVersion int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations;`).Scan(&currentVersion); err != nil {
		return classifyDBError("read schema version", err)
	}

	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return classifyDBError("begin migration", err)
		}

		rolledBack := false
		rollback := func(cause error) error {
			if !rolledBack {
				_ = tx.Rollback()
				rolledBack = true
			}
			return cause
		}

		for _, statement := range m.upSQL {
			if _, err := tx.Exec(statement); err != nil {
				return rollback(classifyDBError(fmt.Sprintf("apply migration %d", m.version), err))
			}
		}

		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?);`, m.version, m.name, nowUTC()); err != nil {
			return rollback(classifyDBError(fmt.Sprintf("record migration %d", m.version), err))
		}

		if err := tx.Commit(); err != nil {
			return classifyDBError(fmt.Sprintf("commit migration %d", m.version), err)
		}
	}

	var finalVersion int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations;`).Scan(&finalVersion); err != nil {
		return classifyDBError("read final schema version", err)
	}
	if finalVersion < currentSchemaVersion {
		return fmt.Errorf("schema migration incomplete: have %d want %d", finalVersion, currentSchemaVersion)
	}
	return nil
}

func (s *Store) UpsertIdentityMetadata(local identity.Identity) error {
	if err := local.Validate(); err != nil {
		return fmt.Errorf("validate identity: %w", err)
	}

	now := nowUTC()
	createdAt := local.CreatedAt.UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`INSERT INTO identities(peer_id, username, display_name, created_at, public_key, last_seen, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(peer_id) DO UPDATE SET
			username=excluded.username,
			display_name=excluded.display_name,
			public_key=excluded.public_key,
			last_seen=excluded.last_seen,
			updated_at=excluded.updated_at;`,
		local.PeerID,
		local.Username,
		local.DisplayName,
		createdAt,
		[]byte(local.PublicKey),
		now,
		now,
	); err != nil {
		return classifyDBError("upsert identity metadata", err)
	}

	return nil
}

func (s *Store) UpsertPeer(peerID, displayName string, seenAt time.Time) error {
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return errors.New("peer id is required")
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = peerID
	}
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}

	now := nowUTC()
	if _, err := s.db.Exec(`INSERT INTO peers(peer_id, display_name, last_seen, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(peer_id) DO UPDATE SET
			display_name=excluded.display_name,
			last_seen=excluded.last_seen,
			updated_at=excluded.updated_at;`,
		peerID,
		displayName,
		seenAt.UTC().Format(time.RFC3339Nano),
		now,
		now,
	); err != nil {
		return classifyDBError("upsert peer", err)
	}
	return nil
}

func (s *Store) ListPeers() ([]Peer, error) {
	rows, err := s.db.Query(`SELECT peer_id, display_name, last_seen, created_at, updated_at FROM peers ORDER BY peer_id ASC;`)
	if err != nil {
		return nil, classifyDBError("query peers", err)
	}
	defer rows.Close()

	peers := make([]Peer, 0)
	for rows.Next() {
		var peer Peer
		var lastSeenRaw, createdRaw, updatedRaw string
		if err := rows.Scan(&peer.PeerID, &peer.DisplayName, &lastSeenRaw, &createdRaw, &updatedRaw); err != nil {
			return nil, classifyDBError("scan peer", err)
		}
		peer.LastSeen = mustParseTime(lastSeenRaw)
		peer.CreatedAt = mustParseTime(createdRaw)
		peer.UpdatedAt = mustParseTime(updatedRaw)
		peers = append(peers, peer)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDBError("iterate peers", err)
	}
	return peers, nil
}

func (s *Store) UpsertGroup(groupID, name string, createdAt time.Time) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return errors.New("group id is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("group name is required")
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	now := nowUTC()
	if _, err := s.db.Exec(`INSERT INTO groups(group_id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(group_id) DO UPDATE SET
			name=excluded.name,
			updated_at=excluded.updated_at;`,
		groupID,
		name,
		createdAt.UTC().Format(time.RFC3339Nano),
		now,
	); err != nil {
		return classifyDBError("upsert group", err)
	}
	return nil
}

func (s *Store) SetGroupMembership(groupID, peerID string, joinedAt time.Time, active bool, lastSeen time.Time) error {
	groupID = strings.TrimSpace(groupID)
	peerID = strings.TrimSpace(peerID)
	if groupID == "" {
		return errors.New("group id is required")
	}
	if peerID == "" {
		return errors.New("peer id is required")
	}
	if joinedAt.IsZero() {
		joinedAt = time.Now().UTC()
	}
	if lastSeen.IsZero() {
		lastSeen = joinedAt
	}
	activeFlag := 0
	if active {
		activeFlag = 1
	}
	if err := s.UpsertPeer(peerID, peerID, lastSeen); err != nil {
		return err
	}

	now := nowUTC()
	if _, err := s.db.Exec(`INSERT INTO group_memberships(group_id, peer_id, joined_at, active, last_seen, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(group_id, peer_id) DO UPDATE SET
			active=excluded.active,
			last_seen=excluded.last_seen,
			updated_at=excluded.updated_at;`,
		groupID,
		peerID,
		joinedAt.UTC().Format(time.RFC3339Nano),
		activeFlag,
		lastSeen.UTC().Format(time.RFC3339Nano),
		now,
	); err != nil {
		return classifyDBError("upsert group membership", err)
	}
	return nil
}

func (s *Store) ListGroups() ([]Group, error) {
	rows, err := s.db.Query(`SELECT group_id, name, created_at, updated_at FROM groups ORDER BY group_id ASC;`)
	if err != nil {
		return nil, classifyDBError("query groups", err)
	}
	defer rows.Close()

	groups := make([]Group, 0)
	for rows.Next() {
		var group Group
		var createdRaw, updatedRaw string
		if err := rows.Scan(&group.GroupID, &group.Name, &createdRaw, &updatedRaw); err != nil {
			return nil, classifyDBError("scan group", err)
		}
		group.CreatedAt = mustParseTime(createdRaw)
		group.UpdatedAt = mustParseTime(updatedRaw)
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDBError("iterate groups", err)
	}
	return groups, nil
}

func (s *Store) ListGroupMembers(groupID string) ([]GroupMembership, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, errors.New("group id is required")
	}

	rows, err := s.db.Query(`SELECT group_id, peer_id, joined_at, active, last_seen, updated_at
		FROM group_memberships
		WHERE group_id = ?
		ORDER BY peer_id ASC;`, groupID)
	if err != nil {
		return nil, classifyDBError("query group members", err)
	}
	defer rows.Close()

	members := make([]GroupMembership, 0)
	for rows.Next() {
		var member GroupMembership
		var joinedRaw, lastSeenRaw, updatedRaw string
		var activeInt int
		if err := rows.Scan(&member.GroupID, &member.PeerID, &joinedRaw, &activeInt, &lastSeenRaw, &updatedRaw); err != nil {
			return nil, classifyDBError("scan group member", err)
		}
		member.JoinedAt = mustParseTime(joinedRaw)
		member.Active = activeInt == 1
		member.LastSeen = mustParseTime(lastSeenRaw)
		member.UpdatedAt = mustParseTime(updatedRaw)
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDBError("iterate group members", err)
	}
	return members, nil
}

func (s *Store) InsertMessage(message protocol.Message, direction MessageDirection, status MessageStatus) error {
	if err := message.Validate(); err != nil {
		return fmt.Errorf("validate message: %w", err)
	}
	if direction == "" {
		direction = DirectionInbound
	}
	if status == "" {
		status = StatusPending
	}

	now := nowUTC()
	_, err := s.db.Exec(`INSERT INTO messages(message_id, version, type, sender_id, recipient_id, group_id, timestamp, body, direction, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		message.ID,
		message.Version,
		string(message.Type),
		message.SenderID,
		nullIfEmpty(message.RecipientID),
		nullIfEmpty(message.GroupID),
		message.Timestamp.UTC().Format(time.RFC3339Nano),
		message.Body,
		string(direction),
		string(status),
		now,
		now,
	)
	if err != nil {
		if isDuplicateErr(err) {
			return ErrDuplicateMessage
		}
		return classifyDBError("insert message", err)
	}
	return nil
}

func (s *Store) UpdateMessageStatus(messageID string, status MessageStatus) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return errors.New("message id is required")
	}
	if status == "" {
		return errors.New("message status is required")
	}

	res, err := s.db.Exec(`UPDATE messages SET status = ?, updated_at = ? WHERE message_id = ?;`, string(status), nowUTC(), messageID)
	if err != nil {
		return classifyDBError("update message status", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return classifyDBError("check status update rows", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetMessage(messageID string) (StoredMessage, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return StoredMessage{}, errors.New("message id is required")
	}

	row := s.db.QueryRow(`SELECT message_id, version, type, sender_id, recipient_id, group_id, timestamp, body, direction, status, created_at, updated_at
		FROM messages
		WHERE message_id = ?;`, messageID)
	return scanStoredMessage(row)
}

func (s *Store) ListDirectConversation(localPeerID, remotePeerID string, limit int) ([]StoredMessage, error) {
	localPeerID = strings.TrimSpace(localPeerID)
	remotePeerID = strings.TrimSpace(remotePeerID)
	if localPeerID == "" || remotePeerID == "" {
		return nil, errors.New("both local and remote peer ids are required")
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`SELECT message_id, version, type, sender_id, recipient_id, group_id, timestamp, body, direction, status, created_at, updated_at
		FROM messages
		WHERE group_id IS NULL
		  AND ((sender_id = ? AND recipient_id = ?) OR (sender_id = ? AND recipient_id = ?))
		ORDER BY timestamp ASC, message_id ASC
		LIMIT ?;`, localPeerID, remotePeerID, remotePeerID, localPeerID, limit)
	if err != nil {
		return nil, classifyDBError("query direct conversation", err)
	}
	defer rows.Close()

	messages := make([]StoredMessage, 0)
	for rows.Next() {
		msg, err := scanStoredMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDBError("iterate direct conversation", err)
	}
	return messages, nil
}

func (s *Store) ListGroupConversation(groupID string, limit int) ([]StoredMessage, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, errors.New("group id is required")
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`SELECT message_id, version, type, sender_id, recipient_id, group_id, timestamp, body, direction, status, created_at, updated_at
		FROM messages
		WHERE group_id = ?
		ORDER BY timestamp ASC, message_id ASC
		LIMIT ?;`, groupID, limit)
	if err != nil {
		return nil, classifyDBError("query group conversation", err)
	}
	defer rows.Close()

	messages := make([]StoredMessage, 0)
	for rows.Next() {
		msg, err := scanStoredMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDBError("iterate group conversation", err)
	}
	return messages, nil
}

func scanStoredMessage(scanner interface{ Scan(dest ...any) error }) (StoredMessage, error) {
	var (
		messageID    string
		version      int
		msgType      string
		senderID     string
		recipientID  sql.NullString
		groupID      sql.NullString
		timestampRaw string
		body         string
		directionRaw string
		statusRaw    string
		createdRaw   string
		updatedRaw   string
	)

	if err := scanner.Scan(
		&messageID,
		&version,
		&msgType,
		&senderID,
		&recipientID,
		&groupID,
		&timestampRaw,
		&body,
		&directionRaw,
		&statusRaw,
		&createdRaw,
		&updatedRaw,
	); err != nil {
		return StoredMessage{}, classifyDBError("scan message", err)
	}

	stored := StoredMessage{
		Message: protocol.Message{
			Version:     version,
			ID:          messageID,
			Type:        protocol.Type(msgType),
			SenderID:    senderID,
			RecipientID: recipientID.String,
			GroupID:     groupID.String,
			Timestamp:   mustParseTime(timestampRaw),
			Body:        body,
		},
		Direction: MessageDirection(directionRaw),
		Status:    MessageStatus(statusRaw),
		CreatedAt: mustParseTime(createdRaw),
		UpdatedAt: mustParseTime(updatedRaw),
	}
	return stored, nil
}

func classifyDBError(op string, err error) error {
	if err == nil {
		return nil
	}
	if isCorruptionErr(err) {
		return fmt.Errorf("%w: %s: %v", ErrCorruptedDatabase, op, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

func isCorruptionErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "file is not a database") ||
		strings.Contains(msg, "database disk image is malformed") ||
		strings.Contains(msg, "malformed")
}

func isDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "constraint failed")
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func mustParseTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func nullIfEmpty(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}
