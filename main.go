package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc/status"

	"github.com/UnitVectorY-Labs/ghook4action/internal/consumer"
	gh "github.com/UnitVectorY-Labs/ghook4action/internal/github"
	"github.com/UnitVectorY-Labs/ghook4action/internal/rules"
)

var Version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err.Error())
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(Version)
		return nil
	}
	rulesFile := flag.String("rules-file", env("RULES_FILE", "rules.json"), "JSON rules configuration")
	subscription := flag.String("pubsub-subscription", env("PUBSUB_SUBSCRIPTION", ""), "Full Pub/Sub subscription name")
	healthPort := flag.String("health-port", env("HEALTH_PORT", "8080"), "Health server port")
	check := flag.Bool("check-config", false, "Validate rules and exit without network access")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	cfg, err := rules.Load(*rulesFile)
	if err != nil {
		return fmt.Errorf("load rules: %w", err)
	}
	if *check {
		fmt.Println("rules configuration valid")
		return nil
	}
	if _, _, err := consumer.ParseSubscription(*subscription); err != nil {
		return err
	}
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if path := os.Getenv("GITHUB_TOKEN_FILE"); path != "" {
		if token != "" {
			return errors.New("set only one of GITHUB_TOKEN and GITHUB_TOKEN_FILE")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return errors.New("cannot read GITHUB_TOKEN_FILE")
		}
		token = strings.TrimSpace(string(data))
	}
	if token == "" {
		return errors.New("GITHUB_TOKEN or GITHUB_TOKEN_FILE is required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	var ready atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); fmt.Fprintln(w, "ok") })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ready")
	})
	listener, err := net.Listen("tcp", ":"+*healthPort)
	if err != nil {
		return fmt.Errorf("health listener: %w", err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	serverErrors := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
			cancel()
		}
	}()
	defer func() {
		ready.Store(false)
		shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		server.Shutdown(shutdownCtx)
	}()
	ready.Store(true)
	slog.Info("starting subscriber", "version", Version, "subscription", *subscription)
	err = consumer.Run(ctx, *subscription, cfg, gh.NewClient(token))
	select {
	case <-serverErrors:
		return errors.New("health server failed")
	default:
	}
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("subscriber stopped (gRPC code %s)", status.Code(err))
	}
	return nil
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
