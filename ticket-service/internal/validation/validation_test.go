package validation

import (
	"testing"

	"github.com/Gayley007/event-ticketing/gen/ticketpb"
)

func TestCreateTicket(t *testing.T) {
	valid := func() *ticketpb.CreateTicketRequest {
		return &ticketpb.CreateTicketRequest{
			EventId:     "8c6c7a1e-5d0b-4f55-9c1f-0f5e0c1f6a10",
			HolderName:  "Pema Wangmo",
			HolderEmail: "pema@rub.edu.bt",
			Quantity:    2,
		}
	}

	if err := CreateTicket(valid()); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	cases := map[string]func(*ticketpb.CreateTicketRequest){
		"bad event id":   func(r *ticketpb.CreateTicketRequest) { r.EventId = "123" },
		"empty name":     func(r *ticketpb.CreateTicketRequest) { r.HolderName = "  " },
		"bad email":      func(r *ticketpb.CreateTicketRequest) { r.HolderEmail = "pema-at-rub" },
		"no tld email":   func(r *ticketpb.CreateTicketRequest) { r.HolderEmail = "pema@localhost" },
		"zero quantity":  func(r *ticketpb.CreateTicketRequest) { r.Quantity = 0 },
		"quantity > max": func(r *ticketpb.CreateTicketRequest) { r.Quantity = 11 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := valid()
			mutate(r)
			if CreateTicket(r) == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestUpdateTicketStatus(t *testing.T) {
	r := &ticketpb.UpdateTicketRequest{
		Id: "8c6c7a1e-5d0b-4f55-9c1f-0f5e0c1f6a10", HolderName: "Pema", HolderEmail: "pema@rub.edu.bt", Quantity: 1, Status: "REFUNDED",
	}
	if UpdateTicket(r) == nil {
		t.Fatal("unknown status must be rejected")
	}
	r.Status = StatusCancelled
	if err := UpdateTicket(r); err != nil {
		t.Fatalf("CANCELLED should be valid: %v", err)
	}
}
