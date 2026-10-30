package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0x464e/opnsense-ip-invites/internal/config"
	"github.com/0x464e/opnsense-ip-invites/internal/firewall"
	"github.com/0x464e/opnsense-ip-invites/internal/httpapi"
	"github.com/0x464e/opnsense-ip-invites/internal/service"
	"github.com/0x464e/opnsense-ip-invites/internal/store"
	frontend "github.com/0x464e/opnsense-ip-invites/web"
)

var version = "dev"

func main() {
	syscall.Umask(0077)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "config.yaml", "YAML configuration path")
	check := flag.Bool("check-config", false, "validate YAML without contacting OPNsense or creating a database")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	c, err := config.Load(*path)
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	if *check {
		fmt.Println("Configuration is valid.")
		return nil
	}
	key, err := config.Secret("OPNSENSE_API_KEY")
	if err != nil {
		return err
	}
	secret, err := config.Secret("OPNSENSE_API_SECRET")
	if err != nil {
		return err
	}
	adminSecret, err := config.Secret("INVITES_ADMIN_PROXY_SECRET")
	if err != nil {
		return err
	}
	api, err := firewall.New(c.OPNsense.URL, key, secret, c.OPNsense.CAFile, c.OPNsense.Timeout.Duration)
	if err != nil {
		return err
	}
	db, err := store.Open(c.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	svc, err := service.New(ctx, c, db, api)
	if err != nil {
		return err
	}
	app, err := httpapi.New(svc, adminSecret, frontend.Assets())
	if err != nil {
		return err
	}
	server := &http.Server{Addr: c.Server.Listen, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); svc.Run(ctx) }()
	serverDone := make(chan error, 1)
	go func() {
		slog.Info("listening", "address", c.Server.Listen, "version", version)
		serverDone <- server.ListenAndServe()
	}()
	select {
	case err = <-serverDone:
		stop()
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		_ = server.Close()
	}
	stop()
	<-workerDone
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return shutdownErr
}
