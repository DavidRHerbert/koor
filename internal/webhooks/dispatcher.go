package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/DavidRHerbert/koor/internal/events"
)

// Webhook represents a registered webhook.
type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Patterns  []string  `json:"patterns"`
	Secret    string    `json:"secret,omitempty"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	LastFired time.Time `json:"last_fired,omitempty"`
	FailCount int       `json:"fail_count"`
}

// Dispatcher manages webhooks and dispatches events to matching URLs.
type Dispatcher struct {
	db     *sql.DB
	bus    *events.Bus
	sub    *events.Subscriber
	logger *slog.Logger
	client *http.Client
	stop   chan struct{}
	wg     sync.WaitGroup
}

// New creates a new webhook Dispatcher.
func New(db *sql.DB, bus *events.Bus, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		db:     db,
		bus:    bus,
		logger: logger,
		client: &http.Client{Timeout: 10 * time.Second},
		stop:   make(chan struct{}),
	}
}

// Start subscribes to all events and dispatches to matching webhooks.
func (d *Dispatcher) Start() {
	d.sub = d.bus.Subscribe("*")
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			select {
			case ev, ok := <-d.sub.Ch:
				if !ok {
					return
				}
				d.dispatch(ev)
			case <-d.stop:
				return
			}
		}
	}()
}

// Stop shuts down the dispatcher.
func (d *Dispatcher) Stop() {
	select {
	case d.stop <- struct{}{}:
	default:
	}
	if d.sub != nil {
		d.bus.Unsubscribe(d.sub)
	}
	d.wg.Wait()
}

// Register adds a new webhook. Returns the created webhook.
func (d *Dispatcher) Register(ctx context.Context, id, url string, patterns []string, secret string) (*Webhook, error) {
	patternsJSON, _ := json.Marshal(patterns)
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO webhooks (id, url, patterns, secret, active, created_at)
		 VALUES (?, ?, ?, ?, 1, datetime('now'))`,
		id, url, string(patternsJSON), secret)
	if err != nil {
		return nil, fmt.Errorf("insert webhook: %w", err)
	}
	return d.Get(ctx, id)
}

// Get retrieves a webhook by ID.
func (d *Dispatcher) Get(ctx context.Context, id string) (*Webhook, error) {
	var w Webhook
	var patternsStr, createdAt string
	var lastFired sql.NullString
	var active int
	err := d.db.QueryRowContext(ctx,
		`SELECT id, url, patterns, secret, active, created_at, last_fired, fail_count
		 FROM webhooks WHERE id = ?`, id).
		Scan(&w.ID, &w.URL, &patternsStr, &w.Secret, &active, &createdAt, &lastFired, &w.FailCount)
	if err != nil {
		return nil, err
	}
	w.Active = active == 1
	w.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	if lastFired.Valid {
		w.LastFired, _ = time.Parse("2006-01-02 15:04:05", lastFired.String)
	}
	json.Unmarshal([]byte(patternsStr), &w.Patterns)
	return &w, nil
}

// List returns all webhooks.
func (d *Dispatcher) List(ctx context.Context) ([]Webhook, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, url, patterns, secret, active, created_at, last_fired, fail_count
		 FROM webhooks ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query webhooks: %w", err)
	}
	defer rows.Close()

	var hooks []Webhook
	for rows.Next() {
		var w Webhook
		var patternsStr, createdAt string
		var lastFired sql.NullString
		var active int
		if err := rows.Scan(&w.ID, &w.URL, &patternsStr, &w.Secret, &active, &createdAt, &lastFired, &w.FailCount); err != nil {
			return nil, fmt.Errorf("scan webhook: %w", err)
		}
		w.Active = active == 1
		w.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		if lastFired.Valid {
			w.LastFired, _ = time.Parse("2006-01-02 15:04:05", lastFired.String)
		}
		json.Unmarshal([]byte(patternsStr), &w.Patterns)
		hooks = append(hooks, w)
	}
	return hooks, rows.Err()
}

// Delete removes a webhook by ID.
func (d *Dispatcher) Delete(ctx context.Context, id string) error {
	res, err := d.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// TestFire sends a test event payload to a specific webhook.
func (d *Dispatcher) TestFire(ctx context.Context, id string) error {
	wh, err := d.Get(ctx, id)
	if err != nil {
		return err
	}
	testPayload, _ := json.Marshal(map[string]any{
		"topic":  "webhook.test",
		"data":   map[string]any{"webhook_id": id, "test": true},
		"source": "koor",
	})
	return d.sendToWebhook(wh, testPayload)
}

// dispatch sends an event to all matching active webhooks.
func (d *Dispatcher) dispatch(ev events.Event) {
	ctx := context.Background()
	hooks, err := d.List(ctx)
	if err != nil {
		d.logger.Error("list webhooks for dispatch", "error", err)
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"topic":      ev.Topic,
		"data":       ev.Data,
		"source":     ev.Source,
		"event_id":   ev.ID,
		"created_at": ev.CreatedAt,
	})

	for i := range hooks {
		wh := &hooks[i]
		if !wh.Active {
			continue
		}
		if !matchesAny(wh.Patterns, ev.Topic) {
			continue
		}

		if err := d.sendToWebhook(wh, payload); err != nil {
			d.logger.Warn("webhook dispatch failed", "webhook_id", wh.ID, "url", wh.URL, "error", err)
			d.db.ExecContext(ctx,
				`UPDATE webhooks SET fail_count = fail_count + 1 WHERE id = ?`, wh.ID)
			// Auto-disable after 10 consecutive failures.
			if wh.FailCount+1 >= 10 {
				d.db.ExecContext(ctx,
					`UPDATE webhooks SET active = 0 WHERE id = ?`, wh.ID)
				d.logger.Warn("webhook auto-disabled after 10 failures", "webhook_id", wh.ID)
			}
		} else {
			d.db.ExecContext(ctx,
				`UPDATE webhooks SET last_fired = datetime('now'), fail_count = 0 WHERE id = ?`, wh.ID)
		}
	}
}

func (d *Dispatcher) sendToWebhook(wh *Webhook, payload []byte) error {
	req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Koor-Event", "true")

	// HMAC signature if secret is set.
	if wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(wh.Secret))
		mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Koor-Signature", sig)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// matchesAny checks if topic matches any of the glob patterns.
func matchesAny(patterns []string, topic string) bool {
	for _, p := range patterns {
		if p == "*" || p == "" {
			return true
		}
		if matched, _ := path.Match(p, topic); matched {
			return true
		}
	}
	return false
}
