package apierror

import (
	"errors"
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFromGRPC(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{status.Error(codes.InvalidArgument, "x"), http.StatusBadRequest},
		{status.Error(codes.NotFound, "x"), http.StatusNotFound},
		{status.Error(codes.FailedPrecondition, "x"), http.StatusConflict},
		{status.Error(codes.Unavailable, "x"), http.StatusServiceUnavailable},
		{status.Error(codes.DeadlineExceeded, "x"), http.StatusGatewayTimeout},
		{status.Error(codes.Internal, "secret db detail"), http.StatusInternalServerError},
		{errors.New("not a grpc error"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		got, body := FromGRPC(c.err)
		if got != c.want {
			t.Errorf("%v: got %d, want %d", c.err, got, c.want)
		}
		if got == http.StatusInternalServerError && body.Error != "internal server error" {
			t.Errorf("500 responses must not leak details, got %q", body.Error)
		}
	}
}
