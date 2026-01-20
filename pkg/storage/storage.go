package storage

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// Relationship represents a follower/following entry from the export.
type Relationship struct {
	Username  string
	Href      string
	Timestamp int64
}

// Open opens (or creates) the SQLite database and ensures schema is present.
func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func ensureSchema(db *sql.DB) error {
	stmts := []string{
		`PRAGMA journal_mode = WAL;`,
		`CREATE TABLE IF NOT EXISTS following (
			username TEXT PRIMARY KEY,
			href TEXT,
			timestamp INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS followers (
			username TEXT PRIMARY KEY,
			href TEXT,
			timestamp INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS unfollowed (
			username TEXT PRIMARY KEY,
			unfollowed_at INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS session_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action_type TEXT NOT NULL,
			username TEXT,
			action_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS not_following (
			username TEXT PRIMARY KEY,
			detected_at INTEGER
		);`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("schema setup: %w", err)
		}
	}

	return nil
}

// UpsertFollowing replaces or inserts following entries.
func UpsertFollowing(db *sql.DB, rows []Relationship) error {
	return upsertRelationships(db, "following", rows)
}

// UpsertFollowers replaces or inserts follower entries.
func UpsertFollowers(db *sql.DB, rows []Relationship) error {
	return upsertRelationships(db, "followers", rows)
}

func upsertRelationships(db *sql.DB, table string, rows []Relationship) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(fmt.Sprintf("INSERT OR REPLACE INTO %s (username, href, timestamp) VALUES (?, ?, ?)", table))
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		if _, err := stmt.Exec(r.Username, r.Href, r.Timestamp); err != nil {
			return fmt.Errorf("exec %s: %w", table, err)
		}
	}

	return tx.Commit()
}

// UnfollowCandidates returns users you follow who do not follow back and haven't been unfollowed yet.
func UnfollowCandidates(db *sql.DB) ([]Relationship, error) {
	query := `
		SELECT f.username, f.href, f.timestamp
		FROM following f
		LEFT JOIN followers fr ON f.username = fr.username
		LEFT JOIN unfollowed u ON f.username = u.username
		LEFT JOIN not_following nf ON f.username = nf.username
		WHERE fr.username IS NULL AND u.username IS NULL AND nf.username IS NULL
		ORDER BY f.timestamp DESC;
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query candidates: %w", err)
	}
	defer rows.Close()

	var out []Relationship
	for rows.Next() {
		var r Relationship
		if err := rows.Scan(&r.Username, &r.Href, &r.Timestamp); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, r)
	}

	return out, rows.Err()
}

// MarkUnfollowed records that a user has been unfollowed
func MarkUnfollowed(db *sql.DB, username string) error {
	_, err := db.Exec("INSERT OR REPLACE INTO unfollowed (username, unfollowed_at) VALUES (?, strftime('%s', 'now'))", username)
	if err != nil {
		return fmt.Errorf("mark unfollowed: %w", err)
	}
	return nil
}

// UnfollowedCount returns the total number of users we've unfollowed
func UnfollowedCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM unfollowed").Scan(&count)
	return count, err
}

// RecordAction logs an action for rate limiting purposes
func RecordAction(db *sql.DB, actionType string, username string) error {
	_, err := db.Exec(
		"INSERT INTO session_actions (action_type, username, action_at) VALUES (?, ?, strftime('%s', 'now'))",
		actionType, username,
	)
	return err
}

// ActionsInLastHour returns the count of actions in the past hour
func ActionsInLastHour(db *sql.DB, actionType string) (int, error) {
	oneHourAgo := "strftime('%s', 'now') - 3600"
	var count int
	err := db.QueryRow(
		fmt.Sprintf("SELECT COUNT(*) FROM session_actions WHERE action_type = ? AND action_at > (%s)", oneHourAgo),
		actionType,
	).Scan(&count)
	return count, err
}

// OldestActionInLastHour returns the timestamp of the oldest action in the past hour
// Returns 0 if no actions found
func OldestActionInLastHour(db *sql.DB, actionType string) (int64, error) {
	oneHourAgo := "strftime('%s', 'now') - 3600"
	var oldest int64
	err := db.QueryRow(
		fmt.Sprintf("SELECT COALESCE(MIN(action_at), 0) FROM session_actions WHERE action_type = ? AND action_at > (%s)", oneHourAgo),
		actionType,
	).Scan(&oldest)
	return oldest, err
}

// NewestActionInLastHour returns the timestamp of the most recent action in the past hour
// Returns 0 if no actions found
func NewestActionInLastHour(db *sql.DB, actionType string) (int64, error) {
	oneHourAgo := "strftime('%s', 'now') - 3600"
	var newest int64
	err := db.QueryRow(
		fmt.Sprintf("SELECT COALESCE(MAX(action_at), 0) FROM session_actions WHERE action_type = ? AND action_at > (%s)", oneHourAgo),
		actionType,
	).Scan(&newest)
	return newest, err
}

// MarkNotFollowing records that we're not actually following a user
func MarkNotFollowing(db *sql.DB, username string) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO not_following (username, detected_at) VALUES (?, strftime('%s', 'now'))",
		username,
	)
	return err
}

// RemoveFromFollowing removes a user from the following table
func RemoveFromFollowing(db *sql.DB, username string) error {
	_, err := db.Exec("DELETE FROM following WHERE username = ?", username)
	return err
}
