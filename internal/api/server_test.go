package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gitstq/nudge/internal/auth"
	"github.com/gitstq/nudge/internal/config"
	"github.com/gitstq/nudge/internal/push"
	"github.com/gitstq/nudge/internal/store"
)

type harness struct {
	t        *testing.T
	st       *store.Store
	vapid    *push.VAPIDKeys
	admin    string
	srv      *httptest.Server
	received chan []byte
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st, err := store.Open(t.TempDir(), 1000, 0)
	t.Cleanup(func() { st.Close() })
	if err != nil {
		t.Fatal(err)
	}
	vapid, err := push.GenerateVAPID()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{RatePerMinute: 0, MaxBodyBytes: 1 << 20, QueueSize: 64}
	s := NewServer(st, cfg, vapid, auth.HashToken("admin-secret"))
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	h := &harness{t: t, st: st, vapid: vapid, admin: "admin-secret", srv: ts, received: make(chan []byte, 4)}
	disp := push.NewDispatcher(st, vapid, "mailto:t@t", 64)
	t.Cleanup(disp.Start(2))
	return h
}

func (h *harness) do(method, path, token string, body any) (int, map[string]any) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

func (h *harness) createKey(topic string) string {
	st, body := h.do("POST", "/api/v1/keys", h.admin, map[string]string{"name": "k", "topic": topic})
	if st != 201 {
		h.t.Fatalf("create key status %d body %v", st, body)
	}
	return body["token"].(string)
}

func TestAuthRequired(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.do("GET", "/api/v1/events", "", nil); code != 401 {
		t.Fatalf("want 401, got %d", code)
	}
	if code, _ := h.do("GET", "/api/v1/stats", "wrong", nil); code != 401 {
		t.Fatalf("want 401 for bad token, got %d", code)
	}
}

func TestPublishAndList(t *testing.T) {
	h := newHarness(t)
	key := h.createKey("")
	code, body := h.do("POST", "/api/v1/notify", key, map[string]any{
		"topic": "backup", "title": "Done", "body": "all good", "level": "success",
	})
	if code != 201 {
		t.Fatalf("publish %d %v", code, body)
	}
	code, list := h.do("GET", "/api/v1/events?topic=backup", h.admin, nil)
	if code != 200 {
		t.Fatalf("list %d", code)
	}
	evs := list["events"].([]any)
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	first := evs[0].(map[string]any)
	if first["level"] != "success" || first["source"] != "api" {
		t.Fatalf("event wrong: %v", first)
	}
}

func TestTopicScopedKeyForbidden(t *testing.T) {
	h := newHarness(t)
	key := h.createKey("alerts")
	code, _ := h.do("POST", "/api/v1/notify", key, map[string]string{"topic": "other", "title": "x"})
	if code != 403 {
		t.Fatalf("want 403, got %d", code)
	}
	code, body := h.do("POST", "/api/v1/notify", key, map[string]string{"body": "y"})
	if code != 201 {
		t.Fatalf("bound-topic publish should pass: %d %v", code, body)
	}
}

func TestNtfyCompat(t *testing.T) {
	h := newHarness(t)
	key := h.createKey("")
	req, _ := http.NewRequest("POST", h.srv.URL+"/nightly-job", strings.NewReader("database dumped"))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Title", "Cron OK")
	req.Header.Set("X-Priority", "5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("ntfy compat %d: %s", resp.StatusCode, b)
	}
	_, list := h.do("GET", "/api/v1/events?topic=nightly-job", h.admin, nil)
	evs := list["events"].([]any)
	if len(evs) != 1 {
		t.Fatalf("want 1 ntfy event, got %d", len(evs))
	}
	e := evs[0].(map[string]any)
	if e["title"] != "Cron OK" || e["level"] != "error" || e["source"] != "ntfy" {
		t.Fatalf("ntfy mapping wrong: %v", e)
	}
}

func TestReadAndDelete(t *testing.T) {
	h := newHarness(t)
	key := h.createKey("")
	_, body := h.do("POST", "/api/v1/notify", key, map[string]string{"title": "a"})
	id := body["id"].(string)
	if code, _ := h.do("POST", "/api/v1/events/read", h.admin, map[string]any{"ids": []string{id}}); code != 200 {
		t.Fatalf("read %d", code)
	}
	if code, b := h.do("GET", "/api/v1/events?unread=1", h.admin, nil); code != 200 && len(b["events"].([]any)) != 0 {
		t.Fatal("unread filter failed")
	}
	if code, _ := h.do("DELETE", "/api/v1/events/"+id, h.admin, nil); code != 200 {
		t.Fatalf("delete %d", code)
	}
	_, list := h.do("GET", "/api/v1/events", h.admin, nil)
	if len(list["events"].([]any)) != 0 {
		t.Fatal("event not deleted")
	}
}

func TestSSEReceivesLiveEvent(t *testing.T) {
	h := newHarness(t)
	req, _ := http.NewRequest("GET", h.srv.URL+"/api/v1/stream?token="+h.admin, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	// drain the initial ": connected" comment
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	key := h.createKey("")
	time.Sleep(50 * time.Millisecond)
	h.do("POST", "/api/v1/notify", key, map[string]string{"title": "live", "body": "now"})

	deadline := time.NewTimer(3 * time.Second)
	for {
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for SSE event")
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "data: ") {
			var e map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e); err == nil && e["title"] == "live" {
				return
			}
		}
	}
}

func TestWebhookChannelFanout(t *testing.T) {
	h := newHarness(t)
	var got []byte
	gotCh := make(chan []byte, 1)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotCh <- b
		w.WriteHeader(200)
	}))
	defer mock.Close()
	code, _ := h.do("POST", "/api/v1/channels", h.admin, map[string]any{
		"name": "hook", "type": "webhook", "target": mock.URL,
	})
	if code != 201 {
		t.Fatalf("channel create %d", code)
	}
	key := h.createKey("")
	h.do("POST", "/api/v1/notify", key, map[string]string{"title": "fanout", "body": "x"})
	select {
	case got = <-gotCh:
	case <-time.After(3 * time.Second):
		t.Fatal("webhook did not receive event")
	}
	var e map[string]any
	if err := json.Unmarshal(got, &e); err != nil || e["title"] != "fanout" {
		t.Fatalf("bad webhook payload: %s", got)
	}
}

func TestDeviceValidation(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.do("POST", "/api/v1/devices", h.admin, map[string]string{"name": "x"}); code != 400 {
		t.Fatalf("want 400 for incomplete device, got %d", code)
	}
	code, body := h.do("POST", "/api/v1/devices", h.admin, map[string]string{
		"name": "phone", "endpoint": "https://push/x/y",
		"p256dh": strings.Repeat("A", 87), "auth": strings.Repeat("B", 22),
	})
	if code != 201 {
		t.Fatalf("device add %d %v", code, body)
	}
	code, list := h.do("GET", "/api/v1/devices", h.admin, nil)
	if code != 200 || len(list["devices"].([]any)) != 1 {
		t.Fatal("device not listed")
	}
}

func TestVAPIDPublic(t *testing.T) {
	h := newHarness(t)
	code, body := h.do("GET", "/api/v1/vapid-public", "", nil)
	if code != 200 || body["public_key"] != h.vapid.PublicB64() {
		t.Fatalf("vapid public mismatch: %d %v", code, body)
	}
}

func TestHealthAndStatic(t *testing.T) {
	h := newHarness(t)
	resp, err := http.Get(h.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("health %d", resp.StatusCode)
	}
	resp2, err := http.Get(h.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	b, _ := io.ReadAll(resp2.Body)
	if !strings.Contains(string(b), "nudge") {
		t.Fatal("index not served")
	}
}
