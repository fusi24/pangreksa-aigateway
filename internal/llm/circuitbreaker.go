// Package llm provides the LLM provider dispatch layer including circuit breaker
// logic and HTTP dispatch to upstream provider APIs.
package llm

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// State constants represent the three circuit breaker states.
const (
	// StateClosed is the normal operating state; all requests are forwarded.
	StateClosed = "closed"
	// StateOpen is the tripped state; all requests are immediately rejected.
	StateOpen = "open"
	// StateHalfOpen is the probe state; one request is allowed to test provider health.
	StateHalfOpen = "half_open"
)

const (
	// failureThreshold is the number of failures within the observation window that
	// causes the breaker to open.
	failureThreshold = 5
	// observationWindow is the rolling period over which failures are counted.
	observationWindow = 10 * time.Second
	// halfOpenDelay is the time the breaker waits in Open state before transitioning
	// to HalfOpen to probe whether the provider has recovered.
	halfOpenDelay = 30 * time.Second
)

// providerState holds per-provider circuit breaker data.
type providerState struct {
	// state is the current circuit breaker state.
	state string
	// failures is the slice of timestamps of recent failures.
	failures []time.Time
	// openedAt is the time the breaker entered Open state.
	openedAt time.Time
	// halfOpenInFlight is true when a probe request has been let through in HalfOpen state.
	halfOpenInFlight bool
}

// CircuitBreaker tracks provider health and prevents cascading failures.
// It opens after failureThreshold failures within observationWindow and
// transitions to HalfOpen after halfOpenDelay.
//
// Thread-safety: safe for concurrent use; all state access is guarded by a mutex.
type CircuitBreaker struct {
	// mu guards the providers map.
	mu sync.Mutex
	// providers maps provider name → *providerState.
	providers map[string]*providerState
	// log is the structured logger.
	log zerolog.Logger
}

// NewCircuitBreaker constructs a CircuitBreaker with no initial provider state.
// Provider state is created lazily on first Allow/RecordSuccess/RecordFailure call.
func NewCircuitBreaker(log zerolog.Logger) *CircuitBreaker {
	return &CircuitBreaker{
		providers: make(map[string]*providerState),
		log:       log.With().Str("component", "circuit-breaker").Logger(),
	}
}

// Allow returns true if a request to the named provider should be forwarded.
// Returns false when the breaker is Open and the half-open probe has already
// been dispatched. Returns true (and sets halfOpenInFlight) for the first probe
// request in HalfOpen state.
func (cb *CircuitBreaker) Allow(provider string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ps := cb.getOrCreate(provider)

	switch ps.state {
	case StateClosed:
		return true

	case StateOpen:
		// check whether enough time has elapsed to attempt a probe.
		if time.Since(ps.openedAt) >= halfOpenDelay {
			ps.state = StateHalfOpen
			ps.halfOpenInFlight = true
			cb.log.Info().Str("provider", provider).Msg("circuit breaker transitioning to half-open")
			return true
		}
		return false

	case StateHalfOpen:
		if ps.halfOpenInFlight {
			// already one probe in flight; reject additional requests.
			return false
		}
		ps.halfOpenInFlight = true
		return true

	default:
		return true
	}
}

// RecordSuccess records a successful provider call. Transitions Open/HalfOpen
// state back to Closed and resets the failure counter.
func (cb *CircuitBreaker) RecordSuccess(provider string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ps := cb.getOrCreate(provider)
	if ps.state != StateClosed {
		cb.log.Info().Str("provider", provider).Str("from_state", ps.state).Msg("circuit breaker closing")
	}
	ps.state = StateClosed
	ps.failures = ps.failures[:0]
	ps.halfOpenInFlight = false
}

// RecordFailure records a failed provider call. If the number of failures within
// observationWindow reaches failureThreshold the breaker opens.
func (cb *CircuitBreaker) RecordFailure(provider string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ps := cb.getOrCreate(provider)
	ps.halfOpenInFlight = false

	now := time.Now()
	ps.failures = append(ps.failures, now)

	// evict failures older than the observation window.
	ps.failures = pruneFailures(ps.failures, now)

	if len(ps.failures) >= failureThreshold && ps.state == StateClosed {
		ps.state = StateOpen
		ps.openedAt = now
		cb.log.Warn().
			Str("provider", provider).
			Int("failure_count", len(ps.failures)).
			Msg("circuit breaker opened")
		return
	}

	// if probe failed in half-open, re-open.
	if ps.state == StateHalfOpen {
		ps.state = StateOpen
		ps.openedAt = now
		cb.log.Warn().Str("provider", provider).Msg("circuit breaker re-opened after half-open probe failure")
	}
}

// getOrCreate returns the providerState for provider, creating a new Closed entry
// if one does not exist. Must be called with mu held.
func (cb *CircuitBreaker) getOrCreate(provider string) *providerState {
	if ps, ok := cb.providers[provider]; ok {
		return ps
	}
	ps := &providerState{state: StateClosed}
	cb.providers[provider] = ps
	return ps
}

// pruneFailures removes timestamps older than observationWindow from the slice.
func pruneFailures(failures []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-observationWindow)
	i := 0
	for i < len(failures) && failures[i].Before(cutoff) {
		i++
	}
	return failures[i:]
}
