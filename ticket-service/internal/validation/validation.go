// Package validation holds the Ticket bounded context's input rules. They run
// before any remote call or database write.
package validation

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/Gayley007/event-ticketing/gen/ticketpb"
	"github.com/Gayley007/event-ticketing/internal/validate"
)

const (
	MaxHolderNameLen = 100
	MaxQuantity      = 10 // per ticket, to stop one buyer taking the whole event

	StatusConfirmed = "CONFIRMED"
	StatusCancelled = "CANCELLED"
)

func checkHolder(name, email string, quantity int32, v *validate.Errors) {
	name = strings.TrimSpace(name)
	v.Add(name == "", "holder_name is required")
	v.Add(utf8.RuneCountInString(name) > MaxHolderNameLen, "holder_name must be at most 100 characters")

	email = strings.TrimSpace(email)
	v.Add(email == "", "holder_email is required")
	v.Add(email != "" && !isEmail(email), "holder_email must be a valid email address")

	v.Add(quantity < 1 || quantity > MaxQuantity, "quantity must be between 1 and 10")
}

// isEmail accepts a bare address such as "pema@rub.edu.bt" (no display name).
func isEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s && strings.Contains(s[strings.LastIndex(s, "@"):], ".")
}

// CreateTicket validates a create request.
func CreateTicket(r *ticketpb.CreateTicketRequest) error {
	var v validate.Errors
	v.Add(!validate.IsUUID(r.GetEventId()), "event_id must be a valid UUID")
	checkHolder(r.GetHolderName(), r.GetHolderEmail(), r.GetQuantity(), &v)
	return v.Err()
}

// UpdateTicket validates an update request.
func UpdateTicket(r *ticketpb.UpdateTicketRequest) error {
	var v validate.Errors
	v.Add(!validate.IsUUID(r.GetId()), "id must be a valid UUID")
	checkHolder(r.GetHolderName(), r.GetHolderEmail(), r.GetQuantity(), &v)
	v.Add(r.GetStatus() != StatusConfirmed && r.GetStatus() != StatusCancelled, "status must be CONFIRMED or CANCELLED")
	return v.Err()
}

// ListTickets validates the optional event filter.
func ListTickets(r *ticketpb.ListTicketsRequest) error {
	var v validate.Errors
	v.Add(r.GetEventId() != "" && !validate.IsUUID(r.GetEventId()), "event_id filter must be a valid UUID")
	return v.Err()
}

// ID validates a resource identifier.
func ID(id string) error {
	var v validate.Errors
	v.Add(!validate.IsUUID(id), "id must be a valid UUID")
	return v.Err()
}
