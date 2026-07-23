package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mikusaa/flaredns/backend/internal/config"
	"github.com/mikusaa/flaredns/backend/internal/security"
	"github.com/mikusaa/flaredns/backend/internal/server"
	"github.com/mikusaa/flaredns/backend/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "reset-password":
			if err := resetPassword(cfg); err != nil {
				log.Fatal(err)
			}
			return
		default:
			log.Fatalf("unknown command %q", os.Args[1])
		}
	}
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	s, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	password, created, err := s.EnsureAdmin(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	count, err := s.TokenCount(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	cipher, err := security.LoadOrCreateCipher(cfg.DataDir, count)
	if err != nil {
		log.Fatal(err)
	}
	app, err := server.New(cfg, s, cipher)
	if err != nil {
		log.Fatal(err)
	}
	if created {
		slog.Warn("initial administrator created", "username", "admin", "password", password, "notice", "this password is only logged once")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app.StartCleanup(ctx)
	httpServer := &http.Server{Addr: cfg.Addr, Handler: app.Router(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second}
	go func() {
		slog.Info("FlareDNS started", "addr", cfg.Addr, "public_url", cfg.PublicURL)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdown)
}

func resetPassword(cfg config.Config) error {
	s, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer s.Close()

	password, err := security.RandomToken(18)
	if err != nil {
		return err
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	if err := s.ResetPassword(context.Background(), "admin", hash); err != nil {
		return err
	}

	fmt.Printf("FlareDNS administrator password reset successfully\nusername: admin\npassword: %s\n", password)
	return nil
}
