package history

import (
	"database/sql"
	"log"
	"time"

	"hinatracer/internal/crashlog"
)

// Resetting a target's statistics permanently deletes its history from
// before the reset: raw samples and every rolled-up bucket (minute, hour,
// day; week and month charts are built from those), for that target only,
// in one transaction. There is no restore.

// resetFailHook, when set (tests), fails a deletion before its commit.
var resetFailHook func(key string) error

// deleteBeforeTx deletes key's samples and buckets that start before at.
func deleteBeforeTx(tx *sql.Tx, key string, at int64) error {
	if _, err := tx.Exec(`DELETE FROM samples WHERE key=? AND ts<?`, key, at); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM rollups WHERE key=? AND ts<?`, key, at); err != nil {
		return err
	}
	if resetFailHook != nil {
		return resetFailHook(key)
	}
	return nil
}

// ResetHistory permanently deletes the keys' history from before at, all
// keys in one transaction (all or nothing). Safe from any goroutine; it
// hits the database, so call it off the UI thread.
func (s *Store) ResetHistory(at time.Time, keys ...string) error {
	if s == nil || len(keys) == 0 {
		return nil
	}
	ms := at.UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		s.flushLocked() // pending pre-reset samples go to disk, then are deleted
		tx, err := s.db.Begin()
		if err != nil {
			s.lastErr = err.Error()
			return err
		}
		defer func() { _ = tx.Rollback() }() // no-op after Commit
		for _, k := range keys {
			if err := deleteBeforeTx(tx, k, ms); err != nil {
				s.lastErr = err.Error()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			s.lastErr = err.Error()
			return err
		}
	}
	if s.resets == nil {
		s.resets = map[string]int64{}
	}
	for _, k := range keys {
		s.resets[k] = ms
		keep := s.mem[k][:0:0]
		for _, x := range s.mem[k] {
			if x.At.UnixMilli() >= ms {
				keep = append(keep, x)
			}
		}
		s.mem[k] = keep
	}
	return nil
}

// ResetAt is when key was last reset in this run (unix ms, 0 = not):
// the WebUI drops chart points it still holds from before it.
func (s *Store) ResetAt(key string) int64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resets[key]
}

// migrateResetsLocked starts, in the background, the one-time migration of
// the 1.2.10 test builds' resets table (reset points that only hid older
// data): that data is deleted as a reset now does, and the table dropped,
// in one transaction.
func (s *Store) migrateResetsLocked() {
	db := s.db
	if db == nil {
		return
	}
	var n int
	if db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='resets'`).Scan(&n) != nil || n == 0 {
		return
	}
	s.migrating.Add(1)
	go func() {
		defer s.migrating.Done()
		defer crashlog.Recover("history.migrateResets")
		if err := s.migrateResets(db); err != nil {
			log.Printf("history: resets migration: %v", err)
		}
	}()
}

func (s *Store) migrateResets(db *sql.DB) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != db {
		return nil // reopened or closed meanwhile; the next open retries
	}
	rows, err := db.Query(`SELECT key, ts FROM resets`)
	if err != nil {
		return err
	}
	pts := map[string]int64{}
	for rows.Next() {
		var k string
		var ts int64
		if rows.Scan(&k, &ts) == nil {
			pts[k] = ts
		}
	}
	_ = rows.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for k, ts := range pts {
		if err := deleteBeforeTx(tx, k, ts); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DROP TABLE resets`); err != nil {
		return err
	}
	return tx.Commit()
}
