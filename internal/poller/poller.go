// Package poller provides a periodic configuration poller that fetches
// GatewayConfig from the Central Server and forwards it to a channel when
// the version changes. An immediate fetch is performed on startup so the daemon
// receives its first config without waiting for the first tick.
package poller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/rs/zerolog"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// configPollRequest is the JSON body sent to the /config endpoint.
type configPollRequest struct {
	// GatewayID identifies this daemon instance.
	GatewayID string `json:"gateway_id"`
	// CurrentVersion is the version hash of the last config the daemon holds.
	// The Central Server returns 304 when this matches the latest version.
	CurrentVersion string `json:"current_version"`
}

// Poller periodically fetches GatewayConfig from the Central Server.
// It performs an immediate fetch on Start and then ticks every intervalSec seconds.
//
// Thread-safety: Start may only be called once per Poller instance.
type Poller struct {
	// serverURL is the base URL of the Central Server, without trailing slash.
	serverURL string
	// gatewayID is this daemon's unique identifier, included in every poll request.
	gatewayID string
	// apiKey is the bearer token for authenticating poll requests.
	apiKey string
	// intervalSec is the poll period in seconds.
	intervalSec int
	// log is the structured logger.
	log zerolog.Logger
	// httpClient is reused across requests for connection pooling.
	httpClient *http.Client
	// currentVersion holds the last fetched config version hash (atomic pointer).
	currentVersion unsafe.Pointer // *string
}

// NewPoller constructs a Poller.
//
// serverURL must be the base URL of the Central Server without a trailing slash.
// gatewayID is the unique daemon identifier sent with every poll request.
// apiKey is the bearer token for HTTP Authorization headers.
// intervalSec controls how often the poll ticker fires; minimum is 5.
func NewPoller(serverURL, gatewayID, apiKey string, intervalSec int, log zerolog.Logger) *Poller {
	if intervalSec < 5 {
		intervalSec = 5
	}
	empty := ""
	p := &Poller{
		serverURL:   serverURL,
		gatewayID:   gatewayID,
		apiKey:      apiKey,
		intervalSec: intervalSec,
		log:         log.With().Str("component", "poller").Logger(),
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
	atomic.StorePointer(&p.currentVersion, unsafe.Pointer(&empty))
	return p
}

// Start runs the polling loop until ctx is cancelled. It performs an immediate
// fetch before the first tick so the daemon receives its initial config without
// waiting for intervalSec seconds. Sends new configs to configChan when the
// version changes; does not close configChan on exit.
//
// Returns ctx.Err() when the context is cancelled.
func (p *Poller) Start(ctx context.Context, configChan chan<- model.GatewayConfig) error {
	p.log.Info().
		Int("interval_sec", p.intervalSec).
		Str("gateway_id", p.gatewayID).
		Msg("config poller starting")

	// immediate fetch before first tick.
	p.fetchAndForward(ctx, configChan)

	ticker := time.NewTicker(time.Duration(p.intervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info().Msg("config poller stopping")
			return ctx.Err()
		case <-ticker.C:
			p.fetchAndForward(ctx, configChan)
		}
	}
}

// fetchAndForward performs a single /config poll. On HTTP 200, the new config is
// decoded and forwarded to configChan. On HTTP 304, nothing is sent. On any other
// status or network error, a warning is logged and the cached config is retained.
func (p *Poller) fetchAndForward(ctx context.Context, configChan chan<- model.GatewayConfig) {
	ver := p.loadVersion()
	cfg, changed, err := p.fetch(ctx, ver)
	if err != nil {
		p.log.Warn().Err(err).Msg("config poll failed; retaining cached config")
		return
	}
	if !changed {
		p.log.Debug().Str("version", ver).Msg("config unchanged (304)")
		return
	}
	p.storeVersion(cfg.Version)
	p.log.Info().Str("version", cfg.Version).Msg("new config received")

	select {
	case configChan <- cfg:
	case <-ctx.Done():
	}
}

// fetch sends POST /config to the Central Server.
// Returns (config, true, nil) on HTTP 200, (zero, false, nil) on HTTP 304,
// or (zero, false, err) on any other condition.
func (p *Poller) fetch(ctx context.Context, currentVersion string) (model.GatewayConfig, bool, error) {
	body, err := json.Marshal(configPollRequest{
		GatewayID:      p.gatewayID,
		CurrentVersion: currentVersion,
	})
	if err != nil {
		return model.GatewayConfig{}, false, fmt.Errorf("poller.fetch: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.serverURL+"/config", bytes.NewReader(body))
	if err != nil {
		return model.GatewayConfig{}, false, fmt.Errorf("poller.fetch: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return model.GatewayConfig{}, false, fmt.Errorf("poller.fetch: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return model.GatewayConfig{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return model.GatewayConfig{}, false, fmt.Errorf("poller.fetch: unexpected status %d", resp.StatusCode)
	}

	var cfg model.GatewayConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return model.GatewayConfig{}, false, fmt.Errorf("poller.fetch: decode response: %w", err)
	}

	return cfg, true, nil
}

// loadVersion reads the current version atomically.
func (p *Poller) loadVersion() string {
	ptr := atomic.LoadPointer(&p.currentVersion)
	return *(*string)(ptr)
}

// storeVersion stores a new version atomically.
func (p *Poller) storeVersion(v string) {
	atomic.StorePointer(&p.currentVersion, unsafe.Pointer(&v))
}
