// Package watch implements the Git watcher daemon for automatic redeployment.
// It provides polling-based and webhook-based change detection, payload
// parsing (GitHub, GitLab, generic), and async deployment triggers.
package watch

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/getdavit/davit/internal/app"
	"github.com/getdavit/davit/internal/caddy"
	"github.com/getdavit/davit/internal/config"
	"github.com/getdavit/davit/internal/state"
)

// Daemon runs the Git watcher daemon, managing both polling and webhook modes.
type Daemon struct {
	db      *state.DB
	cfg     *config.Config
	manager *app.Manager

	pollSched  *PollingScheduler
	webhookSrv *WebhookServer
}

// NewDaemon creates a new Daemon using the provided config and database.
func NewDaemon(db *state.DB, cfg *config.Config) *Daemon {
	caddyClient := caddy.NewClient(cfg.Caddy.AdminAPI)
	manager := app.NewManager(db, caddyClient,
		cfg.Ports.AutoAssignRangeStart,
		cfg.Ports.AutoAssignRangeEnd)

	return &Daemon{
		db:      db,
		cfg:     cfg,
		manager: manager,
	}
}

// Run starts the daemon and blocks until SIGINT/SIGTERM.
func (d *Daemon) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down watcher daemon...")
		cancel()
	}()

	// Load active watchers
	watchers, err := d.db.GetActiveWatchers()
	if err != nil {
		return fmt.Errorf("failed to load active watchers: %w", err)
	}

	if len(watchers) == 0 {
		log.Println("No active watchers configured. Daemon will wait for configuration changes.")
		// Block until cancelled
		<-ctx.Done()
		return nil
	}

	log.Printf("Loaded %d active watchers\n", len(watchers))

	// Start polling scheduler for polling-mode watchers
	d.pollSched = NewPollingScheduler(d.db, d.manager, d.cfg)
	d.pollSched.Start(ctx, watchers)

	// Start webhook server if any watchers use webhook mode
	hasWebhook := false
	for _, w := range watchers {
		if w.Method == "webhook" {
			hasWebhook = true
			break
		}
	}
	if hasWebhook {
		d.webhookSrv = NewWebhookServer(d.db, d.manager)
		go func() {
			if err := d.webhookSrv.Start(ctx, d.cfg.Daemon.WebhookListenAddr); err != nil {
				log.Printf("Webhook server error: %v\n", err)
			}
		}()
	} else {
		log.Println("No webhook watchers configured; skipping webhook server")
	}

	// Block until shutdown
	<-ctx.Done()
	log.Println("Watcher daemon stopped.")
	return nil
}