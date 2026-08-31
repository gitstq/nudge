package api

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gitstq/nudge/internal/push"
)

// TestDeviceWebPushEndToEnd stands up a fake browser push service, registers
// a device with a real P-256 key pair, publishes an event through the API and
// verifies that the dispatcher delivers an aes128gcm body which decrypts back
// to the original payload.
func TestDeviceWebPushEndToEnd(t *testing.T) {
	h := newHarness(t)

	uaPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding

	type received struct {
		body []byte
		auth string
		enc  string
		ttl  string
	}
	gotCh := make(chan received, 1)
	pushSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotCh <- received{body: b, auth: r.Header.Get("Authorization"), enc: r.Header.Get("Content-Encoding"), ttl: r.Header.Get("TTL")}
		w.WriteHeader(http.StatusCreated)
	}))
	defer pushSvc.Close()

	// Register device pointing at the fake push service.
	code, _ := h.do("POST", "/api/v1/devices", h.admin, map[string]string{
		"name":     "fake-browser",
		"endpoint": pushSvc.URL + "/push/1",
		"p256dh":   b64.EncodeToString(uaPriv.PublicKey().Bytes()),
		"auth":     b64.EncodeToString(auth),
	})
	if code != 201 {
		t.Fatalf("device register %d", code)
	}

	key := h.createKey("")
	h.do("POST", "/api/v1/notify", key, map[string]string{"title": "PushHi", "body": "delivered"})

	var got received
	select {
	case got = <-gotCh:
	case <-time.After(4 * time.Second):
		t.Fatal("push service never received the message")
	}
	if got.enc != "aes128gcm" || got.ttl == "" || got.auth == "" {
		t.Fatalf("bad push headers: enc=%q ttl=%q auth=%q", got.enc, got.ttl, got.auth)
	}
	if !startsVAPID(got.auth) {
		t.Fatalf("authorization is not a VAPID header: %q", got.auth)
	}
	plain, err := push.Decrypt(uaPriv.Bytes(), auth, got.body)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatalf("payload not JSON: %v (%s)", err, plain)
	}
	if payload["title"] != "PushHi" || payload["body"] != "delivered" {
		t.Fatalf("decrypted payload mismatch: %s", plain)
	}
}

func startsVAPID(h string) bool {
	return len(h) > 6 && h[:6] == "vapid "
}
