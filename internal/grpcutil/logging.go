// Package grpcutil holds small gRPC helpers shared by both services.
package grpcutil

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryLogger logs every RPC with its method, resulting status code and duration.
func UnaryLogger(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		st := status.Convert(err)
		attrs := []any{"method", info.FullMethod, "code", st.Code().String(), "duration", time.Since(start).Round(time.Millisecond)}
		if err != nil {
			log.Warn("rpc failed", append(attrs, "error", st.Message())...)
		} else {
			log.Info("rpc ok", attrs...)
		}
		return resp, err
	}
}
