package eventclient

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Gayley007/event-ticketing/gen/eventpb"
)

// fakeRPC returns queued errors (nil = success) and counts calls.
type fakeRPC struct {
	eventpb.EventServiceClient
	errs  []error
	calls int
}

func (f *fakeRPC) GetEvent(ctx context.Context, in *eventpb.GetEventRequest, _ ...grpc.CallOption) (*eventpb.Event, error) {
	f.calls++
	var err error
	if len(f.errs) > 0 {
		err, f.errs = f.errs[0], f.errs[1:]
	}
	if err != nil {
		return nil, err
	}
	return &eventpb.Event{Id: in.GetId()}, nil
}

func newTestClient(rpc eventpb.EventServiceClient) *Client {
	return New(rpc, Config{
		Timeout:          50 * time.Millisecond,
		MaxAttempts:      3,
		BaseBackoff:      time.Millisecond,
		FailureThreshold: 2,
		OpenTimeout:      time.Hour,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

var unavailable = status.Error(codes.Unavailable, "connection refused")

func TestRetriesTransientErrorsThenSucceeds(t *testing.T) {
	rpc := &fakeRPC{errs: []error{unavailable, unavailable, nil}}
	ev, err := newTestClient(rpc).GetEvent(context.Background(), "e1")
	if err != nil || ev.GetId() != "e1" {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if rpc.calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", rpc.calls)
	}
}

func TestDoesNotRetryOrTripOnNotFound(t *testing.T) {
	rpc := &fakeRPC{errs: []error{
		status.Error(codes.NotFound, "nope"), status.Error(codes.NotFound, "nope"), status.Error(codes.NotFound, "nope"),
	}}
	c := newTestClient(rpc)
	for range 3 {
		if _, err := c.GetEvent(context.Background(), "x"); status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}
	}
	if rpc.calls != 3 {
		t.Fatalf("NotFound must not be retried: expected 3 calls, got %d", rpc.calls)
	}
	if c.State() != gobreaker.StateClosed {
		t.Fatalf("NotFound must not open the breaker, state=%s", c.State())
	}
}

func TestBreakerOpensAndFailsFast(t *testing.T) {
	errs := make([]error, 6)
	for i := range errs {
		errs[i] = unavailable
	}
	rpc := &fakeRPC{errs: errs}
	c := newTestClient(rpc)

	for range 2 { // FailureThreshold = 2 failed calls (3 attempts each)
		if _, err := c.GetEvent(context.Background(), "e1"); status.Code(err) != codes.Unavailable {
			t.Fatalf("expected Unavailable, got %v", err)
		}
	}
	if c.State() != gobreaker.StateOpen {
		t.Fatalf("expected open breaker, got %s", c.State())
	}

	callsBefore := rpc.calls
	_, err := c.GetEvent(context.Background(), "e1")
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable while open, got %v", err)
	}
	if rpc.calls != callsBefore {
		t.Fatal("open breaker must not call the event service")
	}
}
