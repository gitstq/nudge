// Command nudge is a zero-dependency, self-hosted notification inbox for
// developers. Run with no arguments (or `nudge serve`) to start the server;
// use `nudge send` to publish from scripts and `nudge keys` to mint a VAPID
// key pair.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gitstq/nudge/internal/api"
	"github.com/gitstq/nudge/internal/auth"
	"github.com/gitstq/nudge/internal/config"
	"github.com/gitstq/nudge/internal/push"
	"github.com/gitstq/nudge/internal/store"
)

const version = "v1.0.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("nudge: ")

	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	var err error
	switch cmd {
	case "serve":
		err = runServe(args[1:])
	case "send":
		err = runSend(args[1:])
	case "keys":
		err = runKeys()
	case "version", "--version", "-v":
		fmt.Println("nudge", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", cmd)
		printUsage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func printUsage() {
	fmt.Print(`nudge — self-hosted developer notification inbox

Usage:
  nudge serve [--addr :8080] [--data ./data]   Start the server (default command)
  nudge send --server URL --token TOKEN        Publish a notification
               --title T --body B [--topic X] [--level info]
  nudge keys                                    Generate and print a VAPID key pair
  nudge version                                 Print version

Environment:
  NUDGE_ADDR, NUDGE_DATA_DIR, NUDGE_BASE_URL, NUDGE_ADMIN_TOKEN,
  NUDGE_VAPID_SUBJECT, NUDGE_MAX_EVENTS, NUDGE_MAX_AGE,
  NUDGE_RATE_PER_MIN, NUDGE_MAX_BODY_BYTES
`)
}

// ---- serve ----------------------------------------------------------------

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "", "listen address (default :8080 or NUDGE_ADDR)")
	dataDir := fs.String("data", "", "data directory (default ./data or NUDGE_DATA_DIR)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := config.Default().WithAddr(*addr).WithDataDir(*dataDir)
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}

	vapid, err := loadOrCreateVAPID(cfg.DataDir)
	if err != nil {
		return err
	}
	adminToken, err := loadOrCreateAdmin(cfg)
	if err != nil {
		return err
	}

	st, err := store.Open(cfg.DataDir, cfg.MaxEvents, cfg.MaxAge)
	if err != nil {
		return err
	}
	defer st.Close()

	disp := push.NewDispatcher(st, vapid, cfg.VAPIDSubject, cfg.QueueSize)
	stopDisp := disp.Start(4)
	defer stopDisp()

	srv := api.NewServer(st, cfg, vapid, auth.HashToken(adminToken))
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // SSE connections stay open
		IdleTimeout:       75 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s (data: %s)", cfg.Addr, cfg.DataDir)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		log.Printf("received %s, shutting down…", sig)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return httpSrv.Shutdown(ctx)
}

func loadOrCreateVAPID(dir string) (*push.VAPIDKeys, error) {
	path := filepath.Join(dir, "vapid.json")
	if b, err := os.ReadFile(path); err == nil {
		return push.UnmarshalVAPID(b)
	}
	keys, err := push.GenerateVAPID()
	if err != nil {
		return nil, err
	}
	b, err := keys.Marshal()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	log.Printf("generated new VAPID key pair at %s", path)
	return keys, nil
}

func loadOrCreateAdmin(cfg config.Config) (string, error) {
	if cfg.AdminToken != "" {
		return cfg.AdminToken, nil
	}
	path := filepath.Join(cfg.DataDir, "admin.token")
	if b, err := os.ReadFile(path); err == nil {
		return string(bytes.TrimSpace(b)), nil
	}
	token, err := auth.GenerateToken("nudg_a")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return "", err
	}
	log.Printf("generated admin token (saved to %s): %s", path, token)
	return token, nil
}

// ---- send -----------------------------------------------------------------

func runSend(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	server := fs.String("server", envOr("NUDGE_SERVER", "http://localhost:8080"), "nudge server URL")
	token := fs.String("token", os.Getenv("NUDGE_TOKEN"), "publish token")
	topic := fs.String("topic", "default", "topic name")
	title := fs.String("title", "", "notification title")
	body := fs.String("body", "", "notification body")
	level := fs.String("level", "info", "info|success|warning|error")
	tags := fs.String("tags", "", "comma-separated tags")
	url := fs.String("url", "", "action URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *token == "" {
		return fmt.Errorf("--token (or NUDGE_TOKEN) is required")
	}
	if *title == "" && *body == "" {
		// Read body from stdin when a pipe is present.
		if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
			b, _ := io.ReadAll(io.LimitReader(os.Stdin, 60*1024))
			*body = string(b)
		}
	}
	if *title == "" && *body == "" {
		return fmt.Errorf("title or body is required")
	}
	payload := map[string]any{
		"topic": *topic, "title": *title, "body": *body, "level": *level, "url": *url,
	}
	if *tags != "" {
		var t []string
		for _, x := range splitCSVLocal(*tags) {
			t = append(t, x)
		}
		payload["tags"] = t
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, *server+"/api/v1/notify", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+*token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %s: %s", resp.Status, string(out))
	}
	fmt.Println(string(out))
	return nil
}

// ---- keys -----------------------------------------------------------------

func runKeys() error {
	k, err := push.GenerateVAPID()
	if err != nil {
		return err
	}
	b, err := k.Marshal()
	if err != nil {
		return err
	}
	var pretty bytes.Buffer
	_ = json.Indent(&pretty, b, "", "  ")
	fmt.Println(pretty.String())
	fmt.Println("\napplicationServerKey (browser):", k.PublicB64())
	return nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitCSVLocal(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
