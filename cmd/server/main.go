// Command server is the Central Server binary for the Pangreksa AI Gateway Engine.
//
// Responsibilities:
//   - Serves configuration to Gateway Daemon instances (POST /config)
//   - Resolves user entitlements on cache miss (POST /entitlement)
//   - Exposes admin endpoints for config and entitlement management
//   - Consumes TransactionEvents from Kafka and persists them to PostgreSQL
//   - Broadcasts cache invalidation signals via Redis pub/sub
//
// Environment variables required:
//   - DATABASE_URL   — PostgreSQL DSN
//   - ADMIN_API_KEY  — Bearer token for /admin/* endpoints
//   - DAEMON_API_KEY — Bearer token for daemon-facing endpoints
//
// Optional:
//   - REDIS_URL                      — Redis connection URL
//   - KAFKA_BROKERS                  — comma-separated Kafka broker list
//   - SERVER_PORT                    — HTTP listen port (default: 9000)
//   - ENCRYPT_KEY                    — 64-hex-char AES-256 key
//   - LOG_LEVEL                      — trace|debug|info|warn|error (default: info)
//   - OTEL_EXPORTER_OTLP_ENDPOINT    — OTLP gRPC endpoint
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/internal/server/api"
	"github.com/pangreksa/ai-gateway-engine/internal/server/auth"
	serverconfig "github.com/pangreksa/ai-gateway-engine/internal/server/config"
	"github.com/pangreksa/ai-gateway-engine/internal/server/configstore"
	"github.com/pangreksa/ai-gateway-engine/internal/server/consumer"
	"github.com/pangreksa/ai-gateway-engine/internal/server/entitlement"
	"github.com/pangreksa/ai-gateway-engine/internal/server/publisher"
	"github.com/pangreksa/ai-gateway-engine/internal/server/report"
	"github.com/pangreksa/ai-gateway-engine/internal/server/repository"
	"github.com/pangreksa/ai-gateway-engine/internal/server/sse"
	"github.com/pangreksa/ai-gateway-engine/pkg/logger"
)

const (
	// shutdownTimeout is the maximum time to wait for in-flight requests and
	// goroutines to finish after a shutdown signal is received.
	shutdownTimeout = 30 * time.Second

	// migrationsDir is the path to SQL migration files, relative to the binary's
	// working directory. In production this resolves to /app/migrations.
	migrationsDir = "migrations"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "server: fatal: %v\n", err)
		os.Exit(1)
	}
}

// run is the real entry point extracted so errors can be returned cleanly.
// It initialises all components, starts background goroutines, and blocks until
// a shutdown signal is received or a critical component fails.
func run() error {
	// ── 1. Load configuration ─────────────────────────────────────────────────
	cfg, err := serverconfig.Load()
	if err != nil {
		return fmt.Errorf("run: load config: %w", err)
	}

	// ── 2. Init logger ────────────────────────────────────────────────────────
	log := logger.New()
	log.Info().
		Str("server_id", cfg.ServerID).
		Int("port", cfg.ServerPort).
		Msg("central server starting")

	// base context for the entire application lifetime.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = logger.WithContext(ctx, log)

	// ── 3. Open DB + run migrations ───────────────────────────────────────────
	db, err := repository.New(ctx, cfg.DatabaseURL, migrationsDir)
	if err != nil {
		return fmt.Errorf("run: init database: %w", err)
	}
	defer db.Close()

	log.Info().Msg("database connected and migrations applied")

	// ── 4. Init all repositories ──────────────────────────────────────────────
	orgRepo := repository.NewOrganizationRepository(db)
	userRepo := repository.NewUserRepository(db)
	entRepo := repository.NewEntitlementRepository(db)
	logRepo := repository.NewRequestLogRepository(db)
	costRepo := repository.NewCostRecordRepository(db)
	cfgStoreRepo := repository.NewConfigStoreRepository(db)

	// ensure the gateway_configs table exists (created inline, not via migration).
	if err = cfgStoreRepo.EnsureTable(ctx); err != nil {
		return fmt.Errorf("run: ensure gateway_configs table: %w", err)
	}

	// console-specific repositories
	daemonRegRepo  := repository.NewDaemonRegistrationRepository(db)
	sessionRepo    := repository.NewConsoleSessionRepository(db)
	metricsRepo    := repository.NewMetricsRepository(db)
	auditRepo      := repository.NewAuditRepository(db)
	promptRepo     := repository.NewPromptRepo(db)
	skillRepo      := repository.NewSkillRepository(db)
	mcpRepo        := repository.NewMCPServerRepository(db)
	guardrailRepo  := repository.NewGuardrailRepository(db)
	budgetRuleRepo := repository.NewBudgetRuleRepository(db)
	rateRuleRepo   := repository.NewRateLimitRuleRepository(db)

	// orgRepo is wired here for future handler extensions.
	_ = orgRepo

	// ── 5. Init Redis publisher ───────────────────────────────────────────────
	var pub *publisher.Publisher
	if cfg.RedisURL != "" {
		pub, err = publisher.NewPublisher(cfg.RedisURL, *log)
		if err != nil {
			return fmt.Errorf("run: init redis publisher: %w", err)
		}
		defer func() {
			if closeErr := pub.Close(); closeErr != nil {
				log.Error().Err(closeErr).Msg("error closing redis publisher")
			}
		}()
		log.Info().Msg("redis publisher connected")
	} else {
		log.Warn().Msg("REDIS_URL not set; invalidation publishing disabled")
	}

	// ── 6. Init configstore ───────────────────────────────────────────────────
	cfgStore := configstore.NewStore(cfgStoreRepo, *log)

	// ── 7. Init entitlement service ───────────────────────────────────────────
	entSvc := entitlement.NewService(entRepo, userRepo, pub, *log)

	// ── 8. Init SSE broadcaster ───────────────────────────────────────────────
	sseBroadcaster := sse.NewBroadcaster()

	// ── 9. Init Kafka consumer (optional) ────────────────────────────────────
	var kafkaConsumer *consumer.Consumer
	if cfg.KafkaBrokers != "" {
		kafkaConsumer = consumer.NewConsumer(
			cfg.KafkaBrokers,
			"llm.transactions",
			"",
			logRepo,
			costRepo,
			sseBroadcaster,
			*log,
		)
	} else {
		log.Warn().Msg("KAFKA_BROKERS not set; Kafka consumer disabled")
	}

	// ── 10. Init console services ─────────────────────────────────────────────
	jwtSvc     := auth.NewJWTService(cfg.JWTSecret)
	reportGen  := report.NewGenerator(db.Pool, *log)

	// ── 11. Init HTTP handler + mux ───────────────────────────────────────────
	mux := http.NewServeMux()

	// existing daemon-facing + admin handler
	handler := api.NewHandler(
		cfgStore,
		entSvc,
		pub,
		cfg.ServerID,
		cfg.AdminAPIKey,
		cfg.DaemonAPIKey,
		*log,
	)
	handler.Register(mux)

	// console auth endpoints
	authHandler := api.NewConsoleAuthHandler(userRepo, sessionRepo, daemonRegRepo, jwtSvc, cfg.DaemonAPIKey, *log)
	authHandler.Register(mux)

	// console observability endpoints
	obsHandler := api.NewConsoleObservabilityHandler(
		metricsRepo, auditRepo, daemonRegRepo, logRepo, sseBroadcaster,
		cfg.AdminAPIKey, cfg.JaegerURL, *log,
	)
	obsHandler.Register(mux)

	// console config registry CRUD
	cfgHandler := api.NewConsoleConfigHandler(
		promptRepo, skillRepo, mcpRepo, guardrailRepo, budgetRuleRepo, rateRuleRepo,
		cfg.AdminAPIKey, *log,
	)
	cfgHandler.Register(mux)

	// console report endpoint
	reportHandler := api.NewConsoleReportHandler(reportGen, cfg.AdminAPIKey, *log)
	reportHandler.Register(mux)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ServerPort),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── 12. Start Kafka consumer as background goroutine ─────────────────────
	if kafkaConsumer != nil {
		go startConsumer(ctx, kafkaConsumer, *log)
	}

	// ── 13. Start HTTP server ──────────────────────────────────────────────────
	serverErrCh := make(chan error, 1)
	go func() {
		log.Info().
			Str("addr", srv.Addr).
			Msg("http server listening")
		if listenErr := srv.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			serverErrCh <- fmt.Errorf("http server: %w", listenErr)
		}
		close(serverErrCh)
	}()

	// ── 12. Handle SIGTERM / SIGINT ───────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
	case srvErr := <-serverErrCh:
		if srvErr != nil {
			return fmt.Errorf("run: http server error: %w", srvErr)
		}
		return nil
	}

	// cancel the application context to stop all background goroutines.
	cancel()

	// give in-flight HTTP requests up to shutdownTimeout to complete.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
		log.Error().Err(shutdownErr).Msg("graceful shutdown timed out")
	} else {
		log.Info().Msg("http server shut down cleanly")
	}

	return nil
}

// startConsumer runs the Kafka consumer in a background goroutine with panic recovery.
// If the consumer returns a non-cancellation error it is logged but does not crash the server.
func startConsumer(ctx context.Context, c *consumer.Consumer, log zerolog.Logger) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("panic recovered in kafka consumer goroutine")
		}
	}()

	if err := c.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("kafka consumer exited with error")
	}
}
