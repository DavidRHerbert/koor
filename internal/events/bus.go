package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"time"
)

// Event represents a published event.
type Event struct {
	ID        int64           `json:"id"`
	Topic     string          `json:"topic"`
	Data      json.RawMessage `json:"data"`
	Source    string          `json:"source"`
	CreatedAt time.Time       `json:"created_at"`
}

// Subscriber receives events matching a pattern.
type Subscriber struct {
	Pattern string
	Ch      chan Event
}

// Bus provides pub/sub event distribution with SQLite-backed history.
type Bus struct {
	db          *sql.DB
	maxHistory  int
	mu          sync.RWMutex
	subscribers []*Subscriber
}

// New creates a new event Bus.
func New(db *sql.DB, maxHistory int) *Bus {
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	return &Bus{
		db:         db,
		maxHistory: maxHistory,
	}
}

// Subscribe registers a subscriber for events matching pattern.
// Pattern uses path.Match glob syntax on dot-separated topics.
func (b *Bus) Subscribe(pattern string) *Subscriber {
	sub := &Subscriber{
		Pattern: pattern,
		Ch:      make(chan Event, 64),
	}
	b.mu.Lock()
	b.subscribers = append(b.subscribers, sub)
	b.mu.Unlock()
	return sub
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *Bus) Unsubscribe(sub *Subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, s := range b.subscribers {
		if s == sub {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			close(s.Ch)
			return
		}
	}
}

// Publish writes an event to SQLite history, prunes old events,
// and fans out to matching subscribers.
func (b *Bus) Publish(ctx context.Context, topic string, data json.RawMessage, source string) (*Event, error) {
	// Insert into SQLite.
	res, err := b.db.ExecContext(ctx,
		`INSERT INTO events (topic, data, source, created_at) VALUES (?, ?, ?, datetime('now'))`,
		topic, []byte(data), source)
	if err != nil {
		return nil, fmt.Errorf("insert event: %w", err)
	}
	id, _ := res.LastInsertId()

	// Read back the full event.
	ev, err := b.getByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read back event: %w", err)
	}

	// Prune old events beyond maxHistory.
	b.db.ExecContext(ctx,
		`DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT ?)`,
		b.maxHistory)

	// Fan out to subscribers.
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subscribers {
		if matchTopic(sub.Pattern, topic) {
			select {
			case sub.Ch <- *ev:
			default:
				// Drop if subscriber is slow.
			}
		}
	}

	return ev, nil
}

// History returns the last N events, optionally filtered by topic pattern.
func (b *Bus) History(ctx context.Context, last int, topicPattern string) ([]Event, error) {
	if last <= 0 {
		last = 50
	}

	var rows *sql.Rows
	var err error
	if topicPattern == "" || topicPattern == "*" {
		rows, err = b.db.QueryContext(ctx,
			`SELECT id, topic, data, source, created_at FROM events ORDER BY id DESC LIMIT ?`, last)
	} else {
		// For simple prefix patterns like "api.*", use SQL LIKE.
		// For full glob, fetch all and filter in Go.
		rows, err = b.db.QueryContext(ctx,
			`SELECT id, topic, data, source, created_at FROM events ORDER BY id DESC LIMIT ?`, last*5)
	}
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var ev Event
		var createdAt string
		if err := rows.Scan(&ev.ID, &ev.Topic, &ev.Data, &ev.Source, &createdAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		ev.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)

		if topicPattern != "" && topicPattern != "*" {
			if !matchTopic(topicPattern, ev.Topic) {
				continue
			}
		}
		events = append(events, ev)
		if len(events) >= last {
			break
		}
	}
	return events, rows.Err()
}

func (b *Bus) getByID(ctx context.Context, id int64) (*Event, error) {
	var ev Event
	var createdAt string
	err := b.db.QueryRowContext(ctx,
		`SELECT id, topic, data, source, created_at FROM events WHERE id = ?`, id).
		Scan(&ev.ID, &ev.Topic, &ev.Data, &ev.Source, &createdAt)
	if err != nil {
		return nil, err
	}
	ev.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return &ev, nil
}

// matchTopic checks if a topic matches a glob pattern.
// Both pattern and topic use dot-separated segments.
// Uses path.Match on each segment.
func matchTopic(pattern, topic string) bool {
	if pattern == "*" || pattern == "" {
		return true
	}
	matched, _ := path.Match(pattern, topic)
	return matched
}
