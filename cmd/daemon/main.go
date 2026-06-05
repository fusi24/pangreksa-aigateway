// Command daemon is the Pangreksa AI Gateway Engine daemon process.
// It loads configuration from environment variables, wires all subsystems,
// performs a liveness probe against the Central Server, then starts the
// config poller and provider proxy listeners.
//
// Startup sequence:
//  1. Load daemon config from env vars
//  2. Init logger
//  3. Init crypto key
//  4. Init Redis cache (L2)
//  5. Init local cache (L1)
//  6. Init invalidation subscriber
//  7. Init entitlement resolver
//  8. Init prompt/skill/mcp registries
//  9. Init guardrail/budget/ratelimit engines
//  10. Init circuit breaker + dispatcher
//  11. Init Kafka producer
//  12. Init hot pipeline
//  13. BLOCK on liveness probe
//  14. Start config poller goroutine
//  15. Start invalidation subscriber goroutine
//  16. Start proxy servers (one per provider)
//  17. Wait for SIGINT/SIGTERM or config updates
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/pangreksa/ai-gateway-engine/internal/cache/invalidation"
	localcache "github.com/pangreksa/ai-gateway-engine/internal/cache/local"
	rediscache "github.com/pangreksa/ai-gateway-engine/internal/cache/redis"
	"github.com/pangreksa/ai-gateway-engine/internal/daemon"
	daemoncfg "github.com/pangreksa/ai-gateway-engine/internal/daemon/config"
	"github.com/pangreksa/ai-gateway-engine/internal/entitlement"
	"github.com/pangreksa/ai-gateway-engine/internal/llm"
	"github.com/pangreksa/ai-gateway-engine/internal/poller"
	"github.com/pangreksa/ai-gateway-engine/internal/policy/budget"
	"github.com/pangreksa/ai-gateway-engine/internal/policy/guardrail"
	"github.com/pangreksa/ai-gateway-engine/internal/policy/ratelimit"
	"github.com/pangreksa/ai-gateway-engine/internal/probe"
	"github.com/pangreksa/ai-gateway-engine/internal/producer"
	"github.com/pangreksa/ai-gateway-engine/internal/proxy"
	mcpreg "github.com/pangreksa/ai-gateway-engine/internal/registry/mcp"
	promptreg "github.com/pangreksa/ai-gateway-engine/internal/registry/prompt"
	skillreg "github.com/pangreksa/ai-gateway-engine/internal/registry/skill"
	"github.com/pangreksa/ai-gateway-engine/pkg/crypto"
	"github.com/pangreksa/ai-gateway-engine/pkg/logger"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

const (
	// shutdownTimeout is the maximum time allowed for graceful shutdown flush.
	shutdownTimeout = 30 * time.Second
)

func main() {
	// 1. Load daemon config from environment variables.
	cfg, err := daemoncfg.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "daemon: failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 2. Init logger.
	log := logger.New()
	log.Info().Str("gateway_id", cfg.GatewayID).Msg("daemon starting")

	// 3. Init crypto key (optional: empty key disables decryption of API keys).
	var cryptoKey []byte
	if cfg.EncryptKeyHex != "" {
		cryptoKey, err = crypto.KeyFromHex(cfg.EncryptKeyHex)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to decode ENCRYPT_KEY")
		}
	}

	// 4. Init Redis cache (L2).
	rCache, err := rediscache.NewCache(cfg.RedisURL, *log)
	if err != nil {
		log.Fatal().Err(err).Str("redis_url", cfg.RedisURL).Msg("failed to connect to Redis")
	}
	defer func() {
		if closeErr := rCache.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("redis close error")
		}
	}()
	log.Info().Str("redis_url", cfg.RedisURL).Msg("redis connected")

	// 5. Init local cache (L1).
	lCache := localcache.NewCache()

	// 6. Init invalidation subscriber.
	invalidationSub := invalidation.NewSubscriber(rCache, lCache, *log)

	// 7. Init entitlement resolver.
	entResolver := entitlement.NewResolver(lCache, rCache, cfg.ConfigServerURL, cfg.DaemonAPIKey, *log)

	// 8. Init registries.
	promptReg := promptreg.NewRegistry(*log)
	skillReg := skillreg.NewRegistry(*log)
	mcpReg := mcpreg.NewRegistry(*log)

	// 9. Init policy engines.
	guardrailEng := guardrail.NewEngine(*log)
	budgetEng := budget.NewEngine(rCache, *log)
	ratelimitEng := ratelimit.NewEngine(rCache, *log)

	// 10. Init circuit breaker and dispatcher.
	// Dispatcher is created with an empty GatewayConfig; updated after the first poll.
	cb := llm.NewCircuitBreaker(*log)
	disp := llm.NewDispatcher(model.GatewayConfig{}, cryptoKey, cb, *log)

	// 11. Init Kafka producer.
	kafkaProd, err := producer.NewProducer(cfg.KafkaBrokers, "llm.transactions", *log)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Kafka producer")
	}

	// 12. Init hot pipeline.
	pipeline := daemon.NewHotPipeline(
		entResolver,
		promptReg,
		skillReg,
		mcpReg,
		guardrailEng,
		budgetEng,
		ratelimitEng,
		disp,
		kafkaProd,
		cfg.GatewayID,
		*log,
	)

	// 13. BLOCK on liveness probe against the Central Server.
	prober := probe.NewProber(cfg.ConfigServerURL, cfg.MaxLivenessRetries, *log)
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	if probeErr := prober.Run(probeCtx); probeErr != nil {
		probeCancel()
		log.Fatal().Err(probeErr).Msg("Central Server liveness probe failed; exiting")
	}
	probeCancel()

	// root context — cancelled on SIGINT/SIGTERM.
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Start Kafka producer background goroutine.
	kafkaProd.Start(rootCtx)

	// configChan carries new GatewayConfig from the poller.
	configChan := make(chan model.GatewayConfig, 4)

	g, gCtx := errgroup.WithContext(rootCtx)

	// 14. Start config poller goroutine.
	configPoller := poller.NewPoller(
		cfg.ConfigServerURL, cfg.GatewayID, cfg.DaemonAPIKey, cfg.PollIntervalSec, *log,
	)
	g.Go(func() error {
		defer safeRecover("config-poller", log)
		return configPoller.Start(gCtx, configChan)
	})

	// 15. Start invalidation subscriber goroutine.
	g.Go(func() error {
		defer safeRecover("invalidation-sub", log)
		return invalidationSub.Run(gCtx)
	})

	// Wait for the first config before starting proxy servers.
	// The poller fetches immediately on start; if the server has no config yet
	// (e.g. seed data not loaded) the daemon keeps retrying every poll interval
	// rather than giving up. A hint is logged each cycle so the wait is visible.
	var initialCfg model.GatewayConfig
	waitTick := time.NewTicker(time.Duration(cfg.PollIntervalSec) * time.Second)
	defer waitTick.Stop()
waitLoop:
	for {
		select {
		case initialCfg = <-configChan:
			log.Info().Str("version", initialCfg.Version).Msg("initial config received")
			applyConfig(initialCfg, disp, promptReg, skillReg, mcpReg, guardrailEng, budgetEng, ratelimitEng, log)
			break waitLoop
		case <-waitTick.C:
			log.Warn().
				Str("config_server", cfg.ConfigServerURL).
				Msg("still waiting for initial config — run './scripts/seed-dev.ps1' if this is a fresh install")
		case <-gCtx.Done():
			log.Fatal().Msg("context cancelled before initial config received")
		}
	}

	// 16. Start proxy servers — one per provider in the initial config.
	for _, proxyCfg := range initialCfg.Proxies {
		proxyCfg := proxyCfg // capture loop variable
		adapter := adapterForProvider(proxyCfg.Provider)
		if adapter == nil {
			log.Warn().Str("provider", proxyCfg.Provider).Msg("no adapter found; skipping proxy server")
			continue
		}
		srv := proxy.NewServer(proxyCfg, adapter, pipeline, *log)
		g.Go(func() error {
			defer safeRecover("proxy-"+proxyCfg.Provider, log)
			return srv.Start(gCtx)
		})
	}

	// 17. Signal handling + config hot-reload loop.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	g.Go(func() error {
		for {
			select {
			case <-gCtx.Done():
				return gCtx.Err()
			case sig := <-sigCh:
				log.Info().Str("signal", sig.String()).Msg("shutdown signal received; stopping daemon")
				rootCancel()
				return nil
			case newCfg := <-configChan:
				log.Info().Str("version", newCfg.Version).Msg("applying hot-reloaded config")
				applyConfig(newCfg, disp, promptReg, skillReg, mcpReg, guardrailEng, budgetEng, ratelimitEng, log)
			}
		}
	})

	// block until all goroutines finish.
	if waitErr := g.Wait(); waitErr != nil && waitErr != context.Canceled {
		log.Error().Err(waitErr).Msg("daemon exiting with error")
	}

	// flush any remaining Kafka events.
	if closeErr := kafkaProd.Close(); closeErr != nil {
		log.Error().Err(closeErr).Msg("kafka producer close error")
	}

	log.Info().Msg("daemon stopped")
}

// applyConfig pushes a new GatewayConfig to all hot-reloadable components atomically.
func applyConfig(
	cfg model.GatewayConfig,
	d *llm.Dispatcher,
	pr *promptreg.Registry,
	sr *skillreg.Registry,
	mr *mcpreg.Registry,
	ge *guardrail.Engine,
	be *budget.Engine,
	re *ratelimit.Engine,
	log *zerolog.Logger,
) {
	d.Update(cfg)
	pr.Update(cfg)
	sr.Update(cfg)
	mr.Update(cfg)
	ge.Update(cfg.Policies.Guardrails)
	be.Update(cfg.Policies.BudgetRules)
	re.Update(cfg.Policies.RateRules)
	log.Info().Str("version", cfg.Version).Msg("all components updated with new config")
}

// adapterForProvider returns the proxy.Adapter for the named provider.
// Returns nil for unknown providers.
func adapterForProvider(provider string) proxy.Adapter {
	switch provider {
	case "openai":
		return proxy.NewOpenAIAdapter()
	case "claude":
		return proxy.NewClaudeAdapter()
	case "moonshot":
		return proxy.NewMoonshotAdapter()
	case "ollama":
		return proxy.NewOllamaAdapter()
	default:
		return nil
	}
}

// safeRecover is a deferred panic recovery wrapper for named goroutines.
// It logs the panic with the goroutine name using the provided logger.
func safeRecover(name string, log *zerolog.Logger) {
	if r := recover(); r != nil {
		log.Error().
			Interface("panic", r).
			Str("goroutine", name).
			Msg("goroutine recovered from panic")
	}
}
