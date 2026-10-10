// Package eventclient is the Ticket Service's only way to reach the Event
// Service. Every call is protected by three resilience patterns, layered from
// the outside in:
//
//  1. Circuit breaker - after repeated failures it "opens" and rejects calls
//     immediately, giving the Event Service time to recover instead of piling
//     more requests onto it.
//  2. Retry with exponential backoff (+ jitter) - transient failures
//     (UNAVAILABLE, DEADLINE_EXCEEDED) are retried a few times, waiting
//     longer between each attempt.
//  3. Timeout - every individual attempt has its own deadline, so a slow or
//     unreachable Event Service can never make a request hang.
package eventclient

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Gayley007/event-ticketing/gen/eventpb"
)

// Config holds the resilience settings (all sourced from environment variables).
type Config struct {
	Timeout          time.Duration // deadline for a single attempt
	MaxAttempts      int           // total attempts, including the first
	BaseBackoff      time.Duration // wait before the 2nd attempt; doubles each time
	FailureThreshold int           // consecutive failed calls that open the breaker
	OpenTimeout      time.Duration // how long the breaker stays open before a trial call
}

// Client wraps the generated EventService gRPC client.
type Client struct {
	rpc eventpb.EventServiceClient
	cb  *gobreaker.CircuitBreaker[*eventpb.Event]
	cfg Config
	log *slog.Logger
}

func New(rpc eventpb.EventServiceClient, cfg Config, log *slog.Logger) *Client {
	c := &Client{rpc: rpc, cfg: cfg, log: log}
	c.cb = gobreaker.NewCircuitBreaker[*eventpb.Event](gobreaker.Settings{
		Name:        "event-service",
		MaxRequests: 1,               // one trial call while half-open
		Interval:    0,               // never reset the failure count while closed
		Timeout:     cfg.OpenTimeout, // open -> half-open after this long
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(cfg.FailureThreshold)
		},
		// Only infrastructure failures count against the breaker. A NOT_FOUND
		// or INVALID_ARGUMENT means the Event Service is healthy and answered,
		// so a few bad IDs must not open the circuit.
		IsSuccessful: func(err error) bool {
			return err == nil || !isTransient(err)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker state changed", "breaker", name, "from", from.String(), "to", to.String())
		},
	})
	return c
}

// GetEvent fetches one event through breaker -> retry -> timeout.
func (c *Client) GetEvent(ctx context.Context, id string) (*eventpb.Event, error) {
	ev, err := c.cb.Execute(func() (*eventpb.Event, error) {
		return c.getWithRetry(ctx, id)
	})
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		c.log.Warn("circuit open: failing fast without calling event-service", "event_id", id)
		return nil, status.Error(codes.Unavailable, "event service is unavailable (circuit breaker open); please try again shortly")
	}
	return ev, err
}

// State exposes the breaker state (used in logs and tests).
func (c *Client) State() gobreaker.State {
	return c.cb.State()
}

func (c *Client) getWithRetry(ctx context.Context, id string) (*eventpb.Event, error) {
	var lastErr error
	backoff := c.cfg.BaseBackoff

	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		ev, err := c.rpc.GetEvent(attemptCtx, &eventpb.GetEventRequest{Id: id})
		cancel()

		if err == nil {
			if attempt > 1 {
				c.log.Info("event-service call succeeded after retry", "event_id", id, "attempt", attempt)
			}
			return ev, nil
		}
		if !isTransient(err) {
			return nil, err // business error (e.g. NOT_FOUND): retrying cannot help
		}

		lastErr = err
		st := status.Convert(err)
		c.log.Warn("event-service call failed",
			"event_id", id, "attempt", attempt, "max_attempts", c.cfg.MaxAttempts,
			"code", st.Code().String(), "error", st.Message())

		if attempt == c.cfg.MaxAttempts {
			break
		}

		// Exponential backoff with up to 50% jitter so many clients do not
		// retry in lock-step.
		wait := backoff + time.Duration(rand.Int64N(int64(backoff)/2+1))
		c.log.Info("retrying event-service call", "event_id", id, "next_attempt", attempt+1, "backoff", wait.Round(time.Millisecond))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		backoff *= 2
	}

	return nil, status.Errorf(codes.Unavailable,
		"event service is unavailable after %d attempts: %s",
		c.cfg.MaxAttempts, status.Convert(lastErr).Message())
}

// isTransient reports whether err is worth retrying and should count as a
// failure for the circuit breaker.
func isTransient(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}
