package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gitstq/nudge/internal/store"
)

// Dispatcher fans every stored event out to browser push subscriptions and
// configured outbound channels using a bounded worker pool.
type Dispatcher struct {
	st      *store.Store
	vapid   *VAPIDKeys
	subject string
	client  *http.Client
	queue   chan *store.Event
}

// NewDispatcher builds a dispatcher.
func NewDispatcher(st *store.Store, vapid *VAPIDKeys, subject string, queueSize int) *Dispatcher {
	return &Dispatcher{
		st:      st,
		vapid:   vapid,
		subject: subject,
		client:  &http.Client{Timeout: 10 * time.Second},
		queue:   make(chan *store.Event, queueSize),
	}
}

// Start subscribes to the store and spawns workers. The returned stop
// unsubscribes; in-flight deliveries are awaited through ctx cancellation in
// the caller's shutdown sequence.
func (d *Dispatcher) Start(workers int) (stop func()) {
	ch, unsub := d.st.Subscribe()
	go func() {
		for e := range ch {
			select {
			case d.queue <- e:
			default:
				log.Printf("dispatcher: queue full, dropping event %s", e.ID)
			}
		}
	}()
	for i := 0; i < workers; i++ {
		go d.worker()
	}
	return unsub
}

func (d *Dispatcher) worker() {
	for e := range d.queue {
		d.dispatchDevices(e)
		d.dispatchChannels(e)
	}
}

// pushPayload is the JSON delivered to the service worker.
type pushPayload struct {
	ID      string   `json:"id"`
	Topic   string   `json:"topic"`
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	Level   string   `json:"level"`
	Tags    []string `json:"tags"`
	URL     string   `json:"url,omitempty"`
	Created string   `json:"created_at"`
}

func (d *Dispatcher) dispatchDevices(e *store.Event) {
	payload, _ := json.Marshal(pushPayload{
		ID: e.ID, Topic: e.Topic, Title: e.Title, Body: e.Body,
		Level: e.Level, Tags: e.Tags, URL: e.URL, Created: e.CreatedAt.Format(time.RFC3339),
	})
	for _, dev := range d.st.Devices() {
		sub := Subscription{Endpoint: dev.Endpoint, P256DH: dev.P256DH, Auth: dev.Auth}
		status, err := Send(d.client, sub, d.vapid, d.subject, payload, 2419200)
		switch {
		case err == nil:
			d.st.TouchDevice(dev.ID, status, "")
		case err == ErrGone:
			log.Printf("dispatcher: device %s gone (%d), removing", dev.ID, status)
			_ = d.st.DeleteDevice(dev.ID)
		default:
			d.st.TouchDevice(dev.ID, status, err.Error())
			log.Printf("dispatcher: device %s failed: %v", dev.ID, err)
		}
	}
}

func (d *Dispatcher) dispatchChannels(e *store.Event) {
	for _, ch := range d.st.Channels() {
		if !ch.Enabled {
			continue
		}
		if len(ch.Topics) > 0 {
			matched := false
			for _, t := range ch.Topics {
				if t == e.Topic {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		req, err := buildChannelRequest(ch, e)
		if err != nil {
			d.st.TouchChannel(ch.ID, false, err.Error())
			continue
		}
		resp, err := d.client.Do(req)
		if err != nil {
			d.st.TouchChannel(ch.ID, false, err.Error())
			continue
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			d.st.TouchChannel(ch.ID, false, fmt.Sprintf("HTTP %d", resp.StatusCode))
		} else {
			d.st.TouchChannel(ch.ID, true, "")
		}
	}
}

var levelColor = map[string]int{
	"info": 0x3b82f6, "success": 0x22c55e, "warning": 0xf59e0b, "error": 0xef4444,
}

// buildChannelRequest constructs the provider-specific HTTP request.
func buildChannelRequest(ch *store.Channel, e *store.Event) (*http.Request, error) {
	title := e.Title
	body := e.Body
	switch strings.ToLower(ch.Type) {
	case "webhook":
		b, _ := json.Marshal(e)
		req, err := http.NewRequest(http.MethodPost, ch.Target, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil

	case "discord":
		embeds := []map[string]any{{
			"title":       title,
			"description": body,
			"color":       levelColor[e.Level],
			"footer":      map[string]string{"text": "nudge · " + e.Topic},
		}}
		b, _ := json.Marshal(map[string]any{"username": "nudge", "embeds": embeds})
		req, err := http.NewRequest(http.MethodPost, ch.Target, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil

	case "slack":
		text := fmt.Sprintf("*%s*\n%s", title, body)
		if e.URL != "" {
			text += "\n" + e.URL
		}
		b, _ := json.Marshal(map[string]string{"text": text})
		req, err := http.NewRequest(http.MethodPost, ch.Target, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil

	case "ntfy":
		req, err := http.NewRequest(http.MethodPost, ch.Target, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Title", title)
		req.Header.Set("Priority", "default")
		return req, nil

	case "telegram":
		// Target format: "<botToken>|<chatID>"
		parts := strings.SplitN(ch.Target, "|", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("telegram target must be <botToken>|<chatID>")
		}
		text := title + "\n" + body
		if e.URL != "" {
			text += "\n" + e.URL
		}
		form := url.Values{}
		form.Set("chat_id", strings.TrimSpace(parts[1]))
		form.Set("text", text)
		req, err := http.NewRequest(http.MethodPost,
			"https://api.telegram.org/bot"+strings.TrimSpace(parts[0])+"/sendMessage",
			strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil

	default:
		return nil, fmt.Errorf("unknown channel type %q", ch.Type)
	}
}

// TestChannel sends a synthetic event-shaped request so the UI can verify a
// channel without waiting for real traffic.
func TestChannel(client *http.Client, ch *store.Channel) (int, error) {
	demo := &store.Event{Topic: "nudge-test", Title: "nudge 通道测试", Body: "如果你看到这条消息，说明通道配置成功。", Level: "success"}
	req, err := buildChannelRequest(ch, demo)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(req.Context(), 8*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
