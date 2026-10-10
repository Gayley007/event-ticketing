// Package apierror converts gRPC errors from the services into HTTP
// responses. All gRPC -> HTTP status mapping lives in this one table.
package apierror

import (
	"encoding/json"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mapping struct {
	httpStatus int
	name       string
}

var grpcToHTTP = map[codes.Code]mapping{
	codes.InvalidArgument:    {http.StatusBadRequest, "INVALID_ARGUMENT"},
	codes.NotFound:           {http.StatusNotFound, "NOT_FOUND"},
	codes.AlreadyExists:      {http.StatusConflict, "ALREADY_EXISTS"},
	codes.FailedPrecondition: {http.StatusConflict, "FAILED_PRECONDITION"},
	codes.Unavailable:        {http.StatusServiceUnavailable, "UNAVAILABLE"},
	codes.DeadlineExceeded:   {http.StatusGatewayTimeout, "DEADLINE_EXCEEDED"},
	codes.Unimplemented:      {http.StatusNotImplemented, "UNIMPLEMENTED"},
	codes.Canceled:           {499, "CANCELLED"},
}

// Body is the JSON error shape returned for every failed request.
type Body struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// FromGRPC maps a gRPC error to an HTTP status and error body. Unknown codes
// become 500 with a generic message so internal details never leak.
func FromGRPC(err error) (int, Body) {
	st := status.Convert(err)
	if m, ok := grpcToHTTP[st.Code()]; ok {
		return m.httpStatus, Body{Error: st.Message(), Code: m.name}
	}
	return http.StatusInternalServerError, Body{Error: "internal server error", Code: "INTERNAL"}
}

// WriteGRPC writes the HTTP equivalent of a gRPC error.
func WriteGRPC(w http.ResponseWriter, err error) {
	code, body := FromGRPC(err)
	Write(w, code, body)
}

// Write sends an error body with the given status.
func Write(w http.ResponseWriter, code int, body Body) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// BadRequest is used for problems the gateway detects itself (e.g. malformed JSON).
func BadRequest(w http.ResponseWriter, msg string) {
	Write(w, http.StatusBadRequest, Body{Error: msg, Code: "INVALID_ARGUMENT"})
}
