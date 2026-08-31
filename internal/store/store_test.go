package store

import (
	"testing"
	"time"
)

func TestAddListAndOrdering(t *testing.T) {
	s := openTestStore(t, 100, 0)
	for i := 0; i < 3; i++ {
		if _, err := s.AddEvent(&Event{Topic: "t", Title: "e", Body: "b"}); err != nil {
			t.Fatalf("add: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	got := s.Events(EventFilter{Limit: 10})
	if len(got) != 3 {
		t.Fatalf("want 3 events, got %d", len(got))
	}
	// newest first
	if got[0].CreatedAt.Before(got[2].CreatedAt) {
		t.Fatal("events not newest-first")
	}
	if got[0].Level != "info" {
		t.Fatalf("default level = %q", got[0].Level)
	}
}

func TestDurabilityAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.AddEvent(&Event{Topic: "backup", Title: "done", Body: "ok", Level: "success", Tags: []string{"nightly"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead(false, []string{e.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(dir, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got := s2.Events(EventFilter{Topic: "backup"})
	if len(got) != 1 || !got[0].Read {
		t.Fatalf("durable state lost: %+v", got)
	}
}

func TestRetentionMaxEvents(t *testing.T) {
	s := openTestStore(t, 5, 0)
	for i := 0; i < 8; i++ {
		_, _ = s.AddEvent(&Event{Topic: "x", Title: "t", Body: "b"})
		time.Sleep(time.Millisecond)
	}
	if got := s.Events(EventFilter{Limit: 50}); len(got) != 5 {
		t.Fatalf("want 5 retained, got %d", len(got))
	}
}

func TestRetentionMaxAge(t *testing.T) {
	s := openTestStore(t, 100, time.Hour)
	old := &Event{Topic: "x", Title: "old", Body: "b", CreatedAt: time.Now().Add(-2 * time.Hour)}
	if _, err := s.AddEvent(old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddEvent(&Event{Topic: "x", Title: "new", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	got := s.Events(EventFilter{Limit: 50})
	if len(got) != 1 || got[0].Title != "new" {
		t.Fatalf("age eviction failed: %+v", got)
	}
}

func TestMarkReadAndDelete(t *testing.T) {
	s := openTestStore(t, 100, 0)
	a, _ := s.AddEvent(&Event{Topic: "x", Title: "a", Body: ""})
	b, _ := s.AddEvent(&Event{Topic: "x", Title: "b", Body: ""})
	if err := s.MarkRead(false, []string{a.ID}); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Events(EventFilter{Unread: true, Limit: 10})); n != 1 {
		t.Fatalf("want 1 unread, got %d", n)
	}
	if err := s.DeleteEvents(false, []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Events(EventFilter{Limit: 10})); n != 1 {
		t.Fatalf("want 1 after delete, got %d", n)
	}
}

func TestSubscribeBroadcast(t *testing.T) {
	s := openTestStore(t, 100, 0)
	ch, cancel := s.Subscribe()
	defer cancel()
	go func() {
		_, _ = s.AddEvent(&Event{Topic: "x", Title: "live", Body: "now"})
	}()
	select {
	case e := <-ch:
		if e.Title != "live" {
			t.Fatalf("got %q", e.Title)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive broadcast")
	}
}

func TestKeysDevicesChannels(t *testing.T) {
	s := openTestStore(t, 100, 0)
	k, err := s.AddKey(&PublishKey{Name: "n", Hash: "h", Prefix: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.FindKeyByHash("h"); !ok {
		t.Fatal("key not found by hash")
	}
	if _, err := s.AddDevice(&Device{Name: "d", Endpoint: "https://push/x", P256DH: string(make([]byte, 65)), Auth: string(make([]byte, 16))}); err != nil {
		t.Fatal(err)
	}
	// idempotent re-subscription updates rather than duplicates
	if _, err := s.AddDevice(&Device{Name: "d2", Endpoint: "https://push/x", P256DH: string(make([]byte, 65)), Auth: string(make([]byte, 16))}); err != nil {
		t.Fatal(err)
	}
	if len(s.Devices()) != 1 {
		t.Fatalf("want 1 device, got %d", len(s.Devices()))
	}
	if _, err := s.AddChannel(&Channel{Name: "c", Type: "webhook", Target: "http://x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteKey(k.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.Keys()) != 0 {
		t.Fatal("key delete failed")
	}
}

func TestNormalizeLevel(t *testing.T) {
	cases := map[string]string{"ERROR": "error", "warn": "info", "": "info", "success": "success"}
	for in, want := range cases {
		if got := NormalizeLevel(in); got != want {
			t.Errorf("NormalizeLevel(%q)=%q want %q", in, got, want)
		}
	}
}

func openTestStore(t *testing.T, maxEvents int, maxAge time.Duration) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), maxEvents, maxAge)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
