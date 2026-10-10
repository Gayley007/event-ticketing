// Command ticket-service runs Service A: the owner of the Ticket resource.
// It depends on the Event Service, which it reaches only over gRPC.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"github.com/Gayley007/event-ticketing/gen/eventpb"
	"github.com/Gayley007/event-ticketing/gen/ticketpb"
	"github.com/Gayley007/event-ticketing/internal/env"
	"github.com/Gayley007/event-ticketing/internal/grpcutil"
	"github.com/Gayley007/event-ticketing/internal/postgres"
	"github.com/Gayley007/event-ticketing/ticket-service/internal/eventclient"
	"github.com/Gayley007/event-ticketing/ticket-service/internal/repository"
	"github.com/Gayley007/event-ticketing/ticket-service/internal/server"
)

type config struct {
	GRPCPort         string
	EventServiceAddr string
	DB               postgres.Config
	Resilience       eventclient.Config
}

func loadConfig() (config, error) {
	var l env.Loader
	cfg := config{
		GRPCPort:         l.Required("GRPC_PORT"),
		EventServiceAddr: l.Required("EVENT_SERVICE_ADDR"),
		DB: postgres.Config{
			Host:     l.Required("DB_HOST"),
			Port:     l.Required("DB_PORT"),
			User:     l.Required("DB_USER"),
			Password: l.Required("DB_PASSWORD"),
			Name:     l.Required("DB_NAME"),
		},
		Resilience: eventclient.Config{
			Timeout:          l.Duration("EVENT_CALL_TIMEOUT", time.Second),
			MaxAttempts:      l.Int("EVENT_CALL_MAX_ATTEMPTS", 3),
			BaseBackoff:      l.Duration("EVENT_CALL_BACKOFF", 200*time.Millisecond),
			FailureThreshold: l.Int("CB_FAILURE_THRESHOLD", 3),
			OpenTimeout:      l.Duration("CB_OPEN_TIMEOUT", 15*time.Second),
		},
	}
	return cfg, l.Err()
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil)).With("service", "ticket-service")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DB, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := repository.New(pool)
	if err := repo.Migrate(ctx); err != nil {
		return err
	}

	// The connection to the Event Service is lazy: the ticket service starts
	// even if the event service is down, and reconnects quickly once it is back.
	conn, err := grpc.NewClient(cfg.EventServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.Config{BaseDelay: 500 * time.Millisecond, Multiplier: 1.6, Jitter: 0.2, MaxDelay: 3 * time.Second},
			MinConnectTimeout: cfg.Resilience.Timeout,
		}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	events := eventclient.New(eventpb.NewEventServiceClient(conn), cfg.Resilience, log)
	log.Info("event-service client configured",
		"addr", cfg.EventServiceAddr,
		"timeout", cfg.Resilience.Timeout,
		"max_attempts", cfg.Resilience.MaxAttempts,
		"backoff", cfg.Resilience.BaseBackoff,
		"cb_failure_threshold", cfg.Resilience.FailureThreshold,
		"cb_open_timeout", cfg.Resilience.OpenTimeout)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return err
	}

	srv := grpc.NewServer(grpc.UnaryInterceptor(grpcutil.UnaryLogger(log)))
	ticketpb.RegisterTicketServiceServer(srv, server.New(repo, events, log))
	reflection.Register(srv)

	go func() {
		<-ctx.Done()
		log.Info("shutting down")
		srv.GracefulStop()
	}()

	log.Info("gRPC server listening", "port", cfg.GRPCPort)
	return srv.Serve(lis)
}
