// Package probe implements the startup liveness check performed by the daemon
// before it begins serving requests. The probe polls the Central Server health
// endpoint until it responds with HTTP 200, using exponential back-off with jitter
// to avoid thundering-herd problems during Central Server restarts.
package probe

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

const (
	// initialDelay is the first back-off interval between probe attempts.
	initialDelay = 2 * time.Second
	// maxDelay is the ceiling for the exponential back-off.
	maxDelay = 30 * time.Second
)

// Prober performs the startup liveness check against the Central Server.
//
// Thread-safety: Prober is not safe for concurrent use; create one per startup sequence.
type Prober struct {
	// serverURL is the base URL of the Central Server, without trailing slash.
	serverURL string
	// maxRetries is the maximum number of probe attempts. 0 means infinite.
	maxRetries int
	// log is the structured logger for probe events.
	log zerolog.Logger
	// httpClient is used for the health check request.
	httpClient *http.Client
}

// NewProber constructs a Prober with the given server URL and retry settings.
//
// serverURL should be the base HTTP URL of the Central Server without trailing slash.
// maxRetries controls the maximum number of attempts; pass 0 for infinite retries.
func NewProber(serverURL string, maxRetries int, log zerolog.Logger) *Prober {
	return &Prober{
		serverURL:  serverURL,
		maxRetries: maxRetries,
		log:        log.With().Str("component", "probe").Logger(),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Run blocks until the Central Server responds with HTTP 200 OK on the /health endpoint,
// or until the maximum number of retries is exceeded. Returns nil when the server is
// reachable, or an error if maxRetries > 0 and all attempts have been exhausted.
//
// ctx cancellation immediately terminates the probe loop.
func (p *Prober) Run(ctx context.Context) error {
	url := p.serverURL + "/health"
	delay := initialDelay
	attempt := 0

	p.log.Info().Str("url", url).Msg("starting liveness probe")

	for {
		attempt++

		ok, err := p.check(ctx, url)
		if ok {
			p.log.Info().Int("attempt", attempt).Msg("liveness probe succeeded")
			return nil
		}

		if err != nil {
			p.log.Warn().Err(err).Int("attempt", attempt).Msg("liveness probe failed")
		}

		if p.maxRetries > 0 && attempt >= p.maxRetries {
			return fmt.Errorf("probe.Run: Central Server unreachable after %d attempt(s)", attempt)
		}

		// wait with exponential back-off and jitter before next attempt.
		jitter := time.Duration(rand.Int63n(int64(delay / 2)))
		wait := delay + jitter
		p.log.Debug().Dur("wait", wait).Msg("waiting before next probe attempt")

		select {
		case <-ctx.Done():
			return fmt.Errorf("probe.Run: context cancelled: %w", ctx.Err())
		case <-time.After(wait):
		}

		delay = nextDelay(delay)
	}
}

// check performs a single HTTP GET to the health endpoint.
// Returns (true, nil) on HTTP 200, (false, nil) on non-200, or (false, err) on network error.
func (p *Prober) check(ctx context.Context, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return false, fmt.Errorf("probe.check: build request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("probe.check: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}

	return false, fmt.Errorf("probe.check: unexpected status %d", resp.StatusCode)
}

// nextDelay doubles delay up to maxDelay.
func nextDelay(d time.Duration) time.Duration {
	d *= 2
	if d > maxDelay {
		return maxDelay
	}
	return d
}
