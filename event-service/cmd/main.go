// Command event-service runs Service B: the owner of the Event resource.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/Gayley007/event-ticketing/event-service/internal/repository"
	"github.com/Gayley007/event-ticketing/event-service/internal/server"
	"github.com/Gayley007/event-ticketing/gen/eventpb"
	"github.com/Gayley007/event-ticketing/internal/env"
	"github.com/Gayley007/event-ticketing/internal/grpcutil"
	"github.com/Gayley007/event-ticketing/internal/postgres"
)

type config struct {
	GRPCPort string
	DB       postgres.Config
}

func loadConfig() (config, error) {
	var l env.Loader
	cfg := config{
		GRPCPort: l.Required("GRPC_PORT"),
		DB: postgres.Config{
			Host:     l.Required("DB_HOST"),
			Port:     l.Required("DB_PORT"),
			User:     l.Required("DB_USER"),
			Password: l.Required("DB_PASSWORD"),
			Name:     l.Required("DB_NAME"),
		},
	}
	return cfg, l.Err()
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil)).With("service", "event-service")
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

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return err
	}

	srv := grpc.NewServer(grpc.UnaryInterceptor(grpcutil.UnaryLogger(log)))
	eventpb.RegisterEventServiceServer(srv, server.New(repo, log))
	reflection.Register(srv) // lets grpcurl / Postman gRPC discover the API

	go func() {
		<-ctx.Done()
		log.Info("shutting down")
		srv.GracefulStop()
	}()

	log.Info("gRPC server listening", "port", cfg.GRPCPort)
	return srv.Serve(lis)
}
