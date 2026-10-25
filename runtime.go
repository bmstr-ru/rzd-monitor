package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"bmstr.ru/rzd-monitor/max"
	"bmstr.ru/rzd-monitor/schedule"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client, err := max.New(os.Getenv("MAX_BOT_TOKEN"), os.Getenv("MAX_API_URL"))
	if err != nil {
		return err
	}
	max.Default = client
	mode := os.Getenv("MAX_UPDATE_MODE")
	if mode == "" {
		mode = "polling"
	}
	if mode != "polling" && mode != "webhook" {
		return fmt.Errorf("MAX_UPDATE_MODE must be polling or webhook")
	}
	if err := client.Verify(ctx); err != nil {
		return fmt.Errorf("MAX authorization: %w", err)
	}
	log.Print("MAX bot authorized")
	if mode == "polling" {
		schedule.Start()
		return client.Poll(ctx, handleUpdate)
	}
	endpoint := os.Getenv("MAX_WEBHOOK_URL")
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || (u.Port() != "" && u.Port() != "443") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("MAX_WEBHOOK_URL must be a public HTTPS URL on port 443 without query or fragment")
	}
	secret := os.Getenv("MAX_WEBHOOK_SECRET")
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]{5,256}$`).MatchString(secret) {
		return fmt.Errorf("MAX_WEBHOOK_SECRET must contain 5–256 letters, digits, underscores or hyphens")
	}
	addr := os.Getenv("MAX_LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	updates := make(chan max.Update, 100)
	mux := http.NewServeMux()
	path := u.Path
	if path == "" {
		path = "/"
	}
	mux.Handle(path, max.Webhook(secret, updates))
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := client.Subscribe(ctx, endpoint, secret); err != nil {
		return err
	}
	schedule.Start()
	log.Printf("MAX webhook listening on %s", addr)
	// MAX retries webhooks. Ignore repeated message IDs within a bounded window.
	seen := make(map[string]bool)
	var order []string
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-serverErrors:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case update := <-updates:
			if update.Type == "message_created" && update.Message != nil && update.Message.Body.MID != "" {
				id := update.Message.Body.MID
				if seen[id] {
					continue
				}
				seen[id] = true
				order = append(order, id)
				if len(order) > 10000 {
					delete(seen, order[0])
					order = order[1:]
				}
			}
			handleUpdate(update)
		}
	}
}
