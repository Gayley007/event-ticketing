package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Gayley007/event-ticketing/api-gateway/internal/apierror"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// jsonOut renders protobuf messages with snake_case field names (matching the
// .proto files) and includes empty fields, e.g. "event": null.
var jsonOut = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}

// writeProto sends a protobuf message as JSON.
func writeProto(w http.ResponseWriter, code int, m proto.Message) {
	b, err := jsonOut.Marshal(m)
	if err != nil {
		apierror.Write(w, http.StatusInternalServerError, apierror.Body{Error: "could not encode response", Code: "INTERNAL"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b)
}

// decodeJSON parses the request body into dst. It rejects malformed JSON,
// wrong field types, unknown fields and trailing data with a clear message.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		apierror.Write(w, http.StatusUnsupportedMediaType, apierror.Body{Error: "Content-Type must be application/json", Code: "INVALID_ARGUMENT"})
		return false
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		apierror.BadRequest(w, describeJSONError(err))
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		apierror.BadRequest(w, "request body must contain a single JSON object")
		return false
	}
	return true
}

func describeJSONError(err error) string {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var maxErr *http.MaxBytesError
	switch {
	case errors.Is(err, io.EOF):
		return "request body is required"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "malformed JSON: body ended unexpectedly"
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("malformed JSON at position %d", syntaxErr.Offset)
	case errors.As(err, &typeErr):
		return fmt.Sprintf("field %q must be of type %s", typeErr.Field, typeErr.Type)
	case strings.HasPrefix(err.Error(), "json: unknown field"):
		return strings.TrimPrefix(err.Error(), "json: ")
	case errors.As(err, &maxErr):
		return "request body is too large"
	default:
		return "invalid JSON body"
	}
}

// parseTime converts an optional RFC 3339 string to a protobuf Timestamp.
// An empty string yields nil so the service reports "starts_at is required".
func parseTime(field, value string) (*timestamppb.Timestamp, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("%s must be an RFC 3339 timestamp, e.g. 2026-12-17T18:00:00+06:00", field)
	}
	return timestamppb.New(t), nil
}
