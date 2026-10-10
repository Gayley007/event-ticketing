// Command api-gateway is the single REST entry point of the system.
// Clients (Postman) call it over HTTP/JSON; it forwards each request to the
// owning service over gRPC and maps gRPC status codes back to HTTP.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Gayley007/event-ticketing/api-gateway/internal/handlers"
	"github.com/Gayley007/event-ticketing/api-gateway/internal/middleware"
	"github.com/Gayley007/event-ticketing/gen/eventpb"
	"github.com/Gayley007/event-ticketing/gen/ticketpb"
	"github.com/Gayley007/event-ticketing/internal/env"
)

type config struct {
	HTTPPort          string
	EventServiceAddr  string
	TicketServiceAddr string
	RequestTimeout    time.Duration
	ConnectTimeout    time.Duration
}

func loadConfig() (config, error) {
	var l env.Loader
	cfg := config{
		HTTPPort:          l.Required("HTTP_PORT"),
		EventServiceAddr:  l.Required("EVENT_SERVICE_ADDR"),
		TicketServiceAddr: l.Required("TICKET_SERVICE_ADDR"),
		RequestTimeout:    l.Duration("REQUEST_TIMEOUT", 10*time.Second),
		ConnectTimeout:    l.Duration("GRPC_CONNECT_TIMEOUT", 2*time.Second),
	}
	return cfg, l.Err()
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil)).With("service", "api-gateway")
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// A short connect timeout means a stopped service produces a quick
	// 503 UNAVAILABLE instead of the request waiting for the full deadline.
	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.Config{BaseDelay: 500 * time.Millisecond, Multiplier: 1.6, Jitter: 0.2, MaxDelay: 3 * time.Second},
			MinConnectTimeout: cfg.ConnectTimeout,
		}),
	}

	eventConn, err := grpc.NewClient(cfg.EventServiceAddr, dialOpts...)
	if err != nil {
		return err
	}
	defer eventConn.Close()

	ticketConn, err := grpc.NewClient(cfg.TicketServiceAddr, dialOpts...)
	if err != nil {
		return err
	}
	defer ticketConn.Close()

	events := handlers.NewEvents(eventpb.NewEventServiceClient(eventConn))
	tickets := handlers.NewTickets(ticketpb.NewTicketServiceClient(ticketConn))

	// Resource-based routes (Go 1.22+ method and path-pattern routing).
	mux := http.NewServeMux()
	mux.HandleFunc("POST /events", events.Create)
	mux.HandleFunc("GET /events", events.List)
	mux.HandleFunc("GET /events/{id}", events.Get)
	mux.HandleFunc("PUT /events/{id}", events.Update)
	mux.HandleFunc("DELETE /events/{id}", events.Delete)

	mux.HandleFunc("POST /tickets", tickets.Create)
	mux.HandleFunc("GET /tickets", tickets.List)
	mux.HandleFunc("GET /tickets/{id}", tickets.Get)
	mux.HandleFunc("PUT /tickets/{id}", tickets.Update)
	mux.HandleFunc("DELETE /tickets/{id}", tickets.Delete)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	handler := middleware.Recover(log, middleware.Logging(log, middleware.Timeout(cfg.RequestTimeout, mux)))

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("REST gateway listening", "port", cfg.HTTPPort, "event_service", cfg.EventServiceAddr, "ticket_service", cfg.TicketServiceAddr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
