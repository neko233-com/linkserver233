package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/neko233-com/linkserver233/internal/agentdocs"
	"github.com/neko233-com/linkserver233/internal/buildinfo"
	"github.com/neko233-com/linkserver233/internal/config"
	"github.com/neko233-com/linkserver233/internal/server"
	"github.com/neko233-com/linkserver233/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "linkserver233: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runServe(nil)
	}

	first := args[0]
	if strings.HasPrefix(first, "-") {
		return runServe(args)
	}

	switch first {
	case "serve":
		return runServe(args[1:])
	case "agent":
		fmt.Print(agentdocs.AgentGuide)
		return nil
	case "version":
		printVersion()
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", first)
	}
}

func runServe(args []string) error {
	cfg, err := config.ParseServeArgs(args)
	if err != nil {
		return err
	}

	linkStore, err := store.NewFileStore(cfg.DataPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	srv := server.New(cfg, linkStore, logger)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv,
		ReadHeaderTimeout: cfg.ReadTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv.StartJanitor(shutdownCtx)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("link server started", "addr", cfg.Addr, "data", cfg.DataPath, "base_url", cfg.BaseURL)
		serverErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-shutdownCtx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		logger.Info("shutting down link server")
		if err := httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}

		err := <-serverErr
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func printVersion() {
	fmt.Printf("linkserver233 %s\n", buildinfo.Version)
	fmt.Printf("commit: %s\n", buildinfo.Commit)
	fmt.Printf("built_at: %s\n", buildinfo.BuiltAt)
}

func printUsage() {
	fmt.Print(`linkserver233 - long links, short links, and custom path redirects

Usage:
  linkserver233 [flags]
  linkserver233 serve [flags]
  linkserver233 agent      print the agent/LLM usage guide
  linkserver233 version

Serve flags:
  --addr                    listen address (default :8080)
  --data                    JSON data file path (default data/links.json)
  --base-url                public base URL used in API responses
  --admin-token             optional Bearer token for API access
  --code-length             generated short-code length (default 7)
  --default-ttl             default link lifetime, e.g. 30d/12h (0 to disable)
  --max-ttl                 maximum allowed link lifetime (0 for unlimited)
  --require-expiry          reject links that would never expire
  --allow-private-targets   allow redirect targets on private/internal hosts
  --rate-limit              per-client requests per minute (0 to disable)
  --rate-limit-burst        per-client burst allowance
  --janitor-interval        interval for purging expired links (0 to disable)

Environment variables:
  LINKSERVER_ADDR, LINKSERVER_DATA, LINKSERVER_BASE_URL, LINKSERVER_ADMIN_TOKEN,
  LINKSERVER_CODE_LENGTH, LINKSERVER_DEFAULT_TTL, LINKSERVER_MAX_TTL,
  LINKSERVER_REQUIRE_EXPIRY, LINKSERVER_ALLOW_PRIVATE_TARGETS,
  LINKSERVER_RATE_LIMIT_PER_MIN, LINKSERVER_RATE_LIMIT_BURST,
  LINKSERVER_JANITOR_INTERVAL

Examples:
  linkserver233 --addr :8080 --base-url https://go.example.com --default-ttl 30d
  linkserver233 agent
  linkserver233 version
`)
}
