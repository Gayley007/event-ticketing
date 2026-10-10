// Package validation holds the Event bounded context's input rules. They run
// inside the service before anything touches the database, so invalid data
// can never be stored even if a caller bypasses the API gateway.
package validation

import (
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Gayley007/event-ticketing/gen/eventpb"
	"github.com/Gayley007/event-ticketing/internal/validate"
)

const (
	MaxNameLen  = 100
	MaxVenueLen = 150
	MaxCapacity = 100000
	MaxPrice    = 1_000_000
)

// fields is the common shape of create and update requests.
type fields struct {
	name     string
	venue    string
	startsAt *timestamppb.Timestamp
	capacity int32
	price    float64
}

func check(f fields, now time.Time, v *validate.Errors) {
	name := strings.TrimSpace(f.name)
	v.Add(name == "", "name is required")
	v.Add(utf8.RuneCountInString(name) > MaxNameLen, "name must be at most 100 characters")

	venue := strings.TrimSpace(f.venue)
	v.Add(venue == "", "venue is required")
	v.Add(utf8.RuneCountInString(venue) > MaxVenueLen, "venue must be at most 150 characters")

	if f.startsAt == nil {
		v.Add(true, "starts_at is required (RFC 3339, e.g. 2026-12-17T18:00:00+06:00)")
	} else {
		v.Add(!f.startsAt.IsValid(), "starts_at is not a valid timestamp")
		v.Add(f.startsAt.IsValid() && !f.startsAt.AsTime().After(now), "starts_at must be in the future")
	}

	v.Add(f.capacity <= 0, "capacity must be greater than 0")
	v.Add(f.capacity > MaxCapacity, "capacity must be at most 100000")

	v.Add(f.price < 0, "price must not be negative")
	v.Add(f.price > MaxPrice, "price must be at most 1000000")
}

// CreateEvent validates a create request.
func CreateEvent(r *eventpb.CreateEventRequest, now time.Time) error {
	var v validate.Errors
	check(fields{r.GetName(), r.GetVenue(), r.GetStartsAt(), r.GetCapacity(), r.GetPrice()}, now, &v)
	return v.Err()
}

// UpdateEvent validates an update request, including the target ID.
func UpdateEvent(r *eventpb.UpdateEventRequest, now time.Time) error {
	var v validate.Errors
	v.Add(!validate.IsUUID(r.GetId()), "id must be a valid UUID")
	check(fields{r.GetName(), r.GetVenue(), r.GetStartsAt(), r.GetCapacity(), r.GetPrice()}, now, &v)
	return v.Err()
}

// ID validates a resource identifier.
func ID(id string) error {
	var v validate.Errors
	v.Add(!validate.IsUUID(id), "id must be a valid UUID")
	return v.Err()
}
