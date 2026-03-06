package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devstroop/walink/internal/config"
	"github.com/devstroop/walink/internal/database"
	"github.com/devstroop/walink/internal/handler"
	"github.com/devstroop/walink/internal/middleware"
	"github.com/devstroop/walink/internal/service"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	setupLogging(cfg.Logging.Level)

	log.Info().Str("version", "0.1.0").Msg("starting WaLink")

	// Open database
	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open database")
	}
	defer db.Close()

	// Create account manager
	mgr, err := service.NewAccountManager(cfg, db)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create account manager")
	}

	// Discover existing accounts from DB
	if err := mgr.DiscoverAccounts(context.Background()); err != nil {
		log.Error().Err(err).Msg("failed to discover accounts")
	}

	// Build HTTP router
	mux := http.NewServeMux()

	// Health endpoint (no auth)
	mux.HandleFunc("GET /api/health", handler.Health)

	// API v1 — all routes require auth
	api := handler.NewAPI(mgr)
	apiMux := http.NewServeMux()
	api.RegisterRoutes(apiMux)

	// Wrap API routes with auth middleware
	authed := middleware.Auth(cfg.Auth.SecretKey, apiMux)
	limited := middleware.RateLimit(cfg.Limits.MaxConcurrentRequests, authed)
	mux.Handle("/api/v1/", limited)

	// Swagger UI (no auth)
	if cfg.Swagger.Enabled {
		swaggerPath := cfg.Swagger.Path
		mux.Handle(swaggerPath+"/", handler.SwaggerUI(swaggerPath))
		mux.Handle(swaggerPath, handler.SwaggerUI(swaggerPath))
		log.Info().Str("path", swaggerPath).Msg("swagger UI enabled")
	}

	// Wrap everything with CORS
	root := middleware.CORS(cfg.CORS, mux)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      root,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info().Str("addr", addr).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-done
	log.Info().Msg("shutting down…")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mgr.ShutdownAll()
	_ = srv.Shutdown(ctx)
	log.Info().Msg("goodbye")
}

func setupLogging(level string) {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}).
		With().Timestamp().Caller().Logger()

	switch level {
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
