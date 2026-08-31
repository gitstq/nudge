// Package store is nudge's dependency-free persistence layer.
//
// Events are appended as JSON lines to a write-ahead log (events.log) and
// mirrored in memory for fast queries. A compacted snapshot is rewritten
// atomically when the log grows, so crash recovery only ever replays a small
// tail. Devices, publish keys and outbound channels are small and live in a
// single atomically-rewritten state.json.
//
// No external database, no cgo: a directory is the whole database.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Level values accepted by the API.
var validLevels = map[string]bool{
	"info": true, "success": true, "warning": true, "error": true,
}

// NormalizeLevel returns the canonical level name, defaulting to "info".
func NormalizeLevel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if validLevels[s] {
		return s
	}
	return "info"
}

// Event is a single inbox entry.
type Event struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Level     string    `json:"level"`
	Tags      []string  `json:"tags,omitempty"`
	URL       string    `json:"url,omitempty"`
	Source    string    `json:"source"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// Device is a browser Web Push subscription.
type Device struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Endpoint   string    `json:"endpoint"`
	P256DH     string    `json:"p256dh"`
	Auth       string    `json:"auth"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
	LastStatus int       `json:"last_status,omitempty"`
	LastError  string    `json:"last_error,omitempty"`
}

// PublishKey authenticates inbound publishers. The raw token is never
// stored; only its SHA-256 hash and a display prefix are retained.
type PublishKey struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Topic     string    `json:"topic"` // empty = may publish to any topic
	Hash      string    `json:"hash"`
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
}

// Channel is an outbound fan-out target (webhook/discord/slack/telegram/ntfy).
type Channel struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Target    string    `json:"target"`
	Topics    []string  `json:"topics,omitempty"` // empty = all topics
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	LastOKAt  time.Time `json:"last_ok_at,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

// State is the atomically-persisted small object set.
type State struct {
	Devices  []*Device     `json:"devices"`
	Keys     []*PublishKey `json:"keys"`
	Channels []*Channel    `json:"channels"`
}

// Store is safe for concurrent use.
type Store struct {
	dir string

	mu     sync.RWMutex
	events []*Event
	state  *State

	logFile *os.File
	writer  *bufio.Writer

	maxEvents int
	maxAge    time.Duration

	// listeners receive every newly appended event (SSE broker fan-out).
	listenersMu sync.Mutex
	listeners   map[int]chan *Event
	nextListen  int
}

// Open loads (or initializes) a store in dir and replays the durable log.
func Open(dir string, maxEvents int, maxAge time.Duration) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	s := &Store{
		dir:       dir,
		state:     &State{Devices: []*Device{}, Keys: []*PublishKey{}, Channels: []*Channel{}},
		listeners: map[int]chan *Event{},
		maxEvents: maxEvents,
		maxAge:    maxAge,
	}
	if err := s.loadSnapshot(); err != nil {
		return nil, err
	}
	if err := s.replayLog(); err != nil {
		return nil, err
	}
	if err := s.openLog(); err != nil {
		return nil, err
	}
	if err := s.loadState(); err != nil {
		return nil, err
	}
	s.evictLocked()
	return s, nil
}

func (s *Store) snapshotPath() string { return filepath.Join(s.dir, "events.snapshot.json") }
func (s *Store) logPath() string      { return filepath.Join(s.dir, "events.log") }
func (s *Store) statePath() string    { return filepath.Join(s.dir, "state.json") }

// ---- event persistence ---------------------------------------------------

type snapshotEnvelope struct {
	Events []*Event `json:"events"`
}

func (s *Store) loadSnapshot() error {
	b, err := os.ReadFile(s.snapshotPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}
	var env snapshotEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		return fmt.Errorf("parse snapshot: %w", err)
	}
	s.events = env.Events
	return nil
}

func (s *Store) replayLog() error {
	f, err := os.Open(s.logPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open wal: %w", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	byID := map[string]*Event{}
	for _, e := range s.events {
		byID[e.ID] = e
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec struct {
			Op     string          `json:"op"`
			Event  *Event          `json:"event"`
			ID     string          `json:"id"`
			Read   *bool           `json:"read"`
			IDs    []string        `json:"ids"`
			All    bool            `json:"all"`
			EventB json.RawMessage `json:"event_b"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			continue // skip corrupt tail line
		}
		switch rec.Op {
		case "add":
			var e Event
			if err := json.Unmarshal(rec.EventB, &e); err == nil {
				byID[e.ID] = &e
			}
		case "read":
			if rec.All {
				for _, e := range byID {
					e.Read = true
				}
			}
			for _, id := range rec.IDs {
				if e := byID[id]; e != nil {
					e.Read = true
				}
			}
		case "delete":
			if rec.All {
				byID = map[string]*Event{}
			} else {
				for _, id := range rec.IDs {
					delete(byID, id)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("replay wal: %w", err)
	}
	out := make([]*Event, 0, len(byID))
	for _, e := range byID {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	s.events = out
	return nil
}

func (s *Store) openLog() error {
	f, err := os.OpenFile(s.logPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open wal for append: %w", err)
	}
	s.logFile = f
	s.writer = bufio.NewWriter(f)
	return nil
}

// appendRec writes one mutation record durably (fsync) to the WAL.
func (s *Store) appendRec(rec any) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := s.writer.Write(b); err != nil {
		return err
	}
	if err := s.writer.Flush(); err != nil {
		return err
	}
	return s.logFile.Sync()
}

// compactLocked rewrites the in-memory events to a snapshot and truncates the
// WAL. Caller holds mu (write).
func (s *Store) compactLocked() error {
	tmp := s.snapshotPath() + ".tmp"
	b, _ := json.Marshal(snapshotEnvelope{Events: s.events})
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.snapshotPath()); err != nil {
		return err
	}
	if err := s.logFile.Truncate(0); err != nil {
		return err
	}
	if _, err := s.logFile.Seek(0, 0); err != nil {
		return err
	}
	s.writer.Reset(s.logFile)
	return nil
}

// maybeCompactLocked compacts once the WAL exceeds 2000 records.
func (s *Store) maybeCompactLocked() {
	if fi, err := s.logFile.Stat(); err == nil && fi.Size() > 1<<20 {
		_ = s.compactLocked() // best effort; WAL remains authoritative
	}
}

func (s *Store) evictLocked() {
	now := time.Now()
	kept := s.events[:0]
	for _, e := range s.events {
		if s.maxAge > 0 && now.Sub(e.CreatedAt) > s.maxAge {
			continue
		}
		kept = append(kept, e)
	}
	if s.maxEvents > 0 && len(kept) > s.maxEvents {
		kept = kept[len(kept)-s.maxEvents:]
	}
	s.events = kept
}

// AddEvent validates, stores, durably logs and broadcasts an event.
func (s *Store) AddEvent(e *Event) (*Event, error) {
	if e == nil {
		return nil, errors.New("nil event")
	}
	e.Topic = strings.TrimSpace(e.Topic)
	if e.Topic == "" {
		e.Topic = "default"
	}
	e.Title = strings.TrimSpace(e.Title)
	e.Body = strings.TrimSpace(e.Body)
	if e.Title == "" && e.Body == "" {
		return nil, errors.New("title and body are both empty")
	}
	if e.Title == "" {
		e.Title = e.Topic
	}
	e.Level = NormalizeLevel(e.Level)
	if e.Source == "" {
		e.Source = "api"
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	if e.ID == "" {
		e.ID = newID("ev")
	}
	if e.Tags == nil {
		e.Tags = []string{}
	}

	s.mu.Lock()
	s.events = append(s.events, e)
	s.evictLocked()
	err := s.appendRec(map[string]any{"op": "add", "event_b": e})
	if err == nil {
		s.maybeCompactLocked()
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	s.broadcast(e)
	return e, nil
}

// EventFilter narrows a listing query.
type EventFilter struct {
	Topic  string
	Unread bool
	Limit  int
}

// Events returns a newest-first copy of matching events.
func (s *Store) Events(f EventFilter) []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*Event{}
	for i := len(s.events) - 1; i >= 0; i-- {
		e := s.events[i]
		if f.Topic != "" && e.Topic != f.Topic {
			continue
		}
		if f.Unread && e.Read {
			continue
		}
		cp := *e
		out = append(out, &cp)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out
}

// Get returns a copy of one event.
func (s *Store) Get(id string) (*Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.events {
		if e.ID == id {
			cp := *e
			return &cp, true
		}
	}
	return nil, false
}

// MarkRead flags events read; all=true wins over ids. Durable.
func (s *Store) MarkRead(all bool, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for _, e := range s.events {
		if all || want[e.ID] {
			e.Read = true
		}
	}
	return s.appendRec(map[string]any{"op": "read", "all": all, "ids": ids})
}

// DeleteEvents removes events; all=true wins over ids. Durable.
func (s *Store) DeleteEvents(all bool, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if all {
		s.events = s.events[:0]
	} else {
		drop := map[string]bool{}
		for _, id := range ids {
			drop[id] = true
		}
		kept := s.events[:0]
		for _, e := range s.events {
			if !drop[e.ID] {
				kept = append(kept, e)
			}
		}
		s.events = kept
	}
	return s.appendRec(map[string]any{"op": "delete", "all": all, "ids": ids})
}

// Stats is a lightweight inbox summary.
type Stats struct {
	Total    int            `json:"total"`
	Unread   int            `json:"unread"`
	ByTopic  map[string]int `json:"by_topic"`
	ByLevel  map[string]int `json:"by_level"`
	Devices  int            `json:"devices"`
	Keys     int            `json:"keys"`
	Channels int            `json:"channels"`
	LatestAt *time.Time     `json:"latest_at,omitempty"`
}

// Stats computes the current summary.
func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Stats{ByTopic: map[string]int{}, ByLevel: map[string]int{}}
	for _, e := range s.events {
		st.Total++
		if !e.Read {
			st.Unread++
		}
		st.ByTopic[e.Topic]++
		st.ByLevel[e.Level]++
		t := e.CreatedAt
		if st.LatestAt == nil || t.After(*st.LatestAt) {
			st.LatestAt = &t
		}
	}
	st.Devices = len(s.state.Devices)
	st.Keys = len(s.state.Keys)
	st.Channels = len(s.state.Channels)
	return st
}

// ---- devices / keys / channels ------------------------------------------

func (s *Store) loadState() error {
	b, err := os.ReadFile(s.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return fmt.Errorf("parse state: %w", err)
	}
	if st.Devices == nil {
		st.Devices = []*Device{}
	}
	if st.Keys == nil {
		st.Keys = []*PublishKey{}
	}
	if st.Channels == nil {
		st.Channels = []*Channel{}
	}
	s.state = &st
	return nil
}

// saveStateLocked atomically rewrites state.json. Caller holds mu.
func (s *Store) saveStateLocked() error {
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.statePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.statePath())
}

// AddDevice registers a push subscription.
func (s *Store) AddDevice(d *Device) (*Device, error) {
	if d.Endpoint == "" || d.P256DH == "" || d.Auth == "" {
		return nil, errors.New("endpoint, p256dh and auth are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.state.Devices {
		if ex.Endpoint == d.Endpoint { // idempotent re-subscription
			ex.Name = d.Name
			ex.P256DH = d.P256DH
			ex.Auth = d.Auth
			ex.LastSeenAt = time.Now().UTC()
			d = ex
			return d, s.saveStateLocked()
		}
	}
	d.ID = newID("dev")
	d.CreatedAt = time.Now().UTC()
	d.LastSeenAt = d.CreatedAt
	s.state.Devices = append(s.state.Devices, d)
	return d, s.saveStateLocked()
}

// Devices returns all registered devices.
func (s *Store) Devices() []*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Device, len(s.state.Devices))
	for i, d := range s.state.Devices {
		cp := *d
		out[i] = &cp
	}
	return out
}

// DeleteDevice removes a subscription by id.
func (s *Store) DeleteDevice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.state.Devices[:0]
	for _, d := range s.state.Devices {
		if d.ID != id {
			kept = append(kept, d)
		}
	}
	s.state.Devices = kept
	return s.saveStateLocked()
}

// TouchDevice records the latest delivery outcome.
func (s *Store) TouchDevice(id string, status int, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.state.Devices {
		if d.ID == id {
			d.LastSeenAt = time.Now().UTC()
			d.LastStatus = status
			d.LastError = errMsg
			_ = s.saveStateLocked()
			return
		}
	}
}

// AddKey registers a publish key record (hash only).
func (s *Store) AddKey(k *PublishKey) (*PublishKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k.ID = newID("key")
	k.CreatedAt = time.Now().UTC()
	s.state.Keys = append(s.state.Keys, k)
	return k, s.saveStateLocked()
}

// Keys returns all key records.
func (s *Store) Keys() []*PublishKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*PublishKey, len(s.state.Keys))
	for i, k := range s.state.Keys {
		cp := *k
		out[i] = &cp
	}
	return out
}

// DeleteKey removes a key by id.
func (s *Store) DeleteKey(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.state.Keys[:0]
	for _, k := range s.state.Keys {
		if k.ID != id {
			kept = append(kept, k)
		}
	}
	s.state.Keys = kept
	return s.saveStateLocked()
}

// FindKeyByHash returns the key matching a stored hash.
func (s *Store) FindKeyByHash(hash string) (*PublishKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, k := range s.state.Keys {
		if k.Hash == hash {
			cp := *k
			return &cp, true
		}
	}
	return nil, false
}

// AddChannel persists an outbound channel.
func (s *Store) AddChannel(c *Channel) (*Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID = newID("ch")
	c.CreatedAt = time.Now().UTC()
	c.Enabled = true
	s.state.Channels = append(s.state.Channels, c)
	return c, s.saveStateLocked()
}

// Channels returns all channels.
func (s *Store) Channels() []*Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Channel, len(s.state.Channels))
	for i, c := range s.state.Channels {
		cp := *c
		out[i] = &cp
	}
	return out
}

// DeleteChannel removes a channel.
func (s *Store) DeleteChannel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.state.Channels[:0]
	for _, c := range s.state.Channels {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	s.state.Channels = kept
	return s.saveStateLocked()
}

// TouchChannel records the last delivery result.
func (s *Store) TouchChannel(id string, ok bool, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.state.Channels {
		if c.ID == id {
			if ok {
				c.LastOKAt = time.Now().UTC()
				c.LastError = ""
			} else {
				c.LastError = errMsg
			}
			_ = s.saveStateLocked()
			return
		}
	}
}

// ---- live event stream ---------------------------------------------------

// Subscribe returns a buffered channel of new events plus an unsubscribe func.
func (s *Store) Subscribe() (<-chan *Event, func()) {
	ch := make(chan *Event, 32)
	s.listenersMu.Lock()
	id := s.nextListen
	s.nextListen++
	s.listeners[id] = ch
	s.listenersMu.Unlock()
	return ch, func() {
		s.listenersMu.Lock()
		if c, ok := s.listeners[id]; ok {
			delete(s.listeners, id)
			close(c)
		}
		s.listenersMu.Unlock()
	}
}

func (s *Store) broadcast(e *Event) {
	s.listenersMu.Lock()
	defer s.listenersMu.Unlock()
	for _, ch := range s.listeners {
		select {
		case ch <- e:
		default: // slow consumer is dropped for this event rather than blocking
		}
	}
}

// Close flushes and closes the WAL.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			return err
		}
	}
	if s.logFile != nil {
		return s.logFile.Close()
	}
	return nil
}
