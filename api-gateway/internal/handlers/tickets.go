package handlers

import (
	"net/http"

	"github.com/Gayley007/event-ticketing/api-gateway/internal/apierror"
	"github.com/Gayley007/event-ticketing/gen/ticketpb"
)

// createTicketInput is the JSON body for POST /tickets.
type createTicketInput struct {
	EventID     string `json:"event_id"`
	HolderName  string `json:"holder_name"`
	HolderEmail string `json:"holder_email"`
	Quantity    int32  `json:"quantity"`
}

// updateTicketInput is the JSON body for PUT /tickets/{id}.
type updateTicketInput struct {
	HolderName  string `json:"holder_name"`
	HolderEmail string `json:"holder_email"`
	Quantity    int32  `json:"quantity"`
	Status      string `json:"status"`
}

type Tickets struct {
	client ticketpb.TicketServiceClient
}

func NewTickets(client ticketpb.TicketServiceClient) *Tickets {
	return &Tickets{client: client}
}

// Create handles POST /tickets -> 201 Created.
func (h *Tickets) Create(w http.ResponseWriter, r *http.Request) {
	var in createTicketInput
	if !decodeJSON(w, r, &in) {
		return
	}
	t, err := h.client.CreateTicket(r.Context(), &ticketpb.CreateTicketRequest{
		EventId: in.EventID, HolderName: in.HolderName, HolderEmail: in.HolderEmail, Quantity: in.Quantity,
	})
	if err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	w.Header().Set("Location", "/tickets/"+t.GetId())
	writeProto(w, http.StatusCreated, t)
}

// Get handles GET /tickets/{id} -> 200 OK (with event details when available).
func (h *Tickets) Get(w http.ResponseWriter, r *http.Request) {
	t, err := h.client.GetTicket(r.Context(), &ticketpb.GetTicketRequest{Id: r.PathValue("id")})
	if err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, t)
}

// List handles GET /tickets and GET /tickets?event_id={id} -> 200 OK.
func (h *Tickets) List(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.ListTickets(r.Context(), &ticketpb.ListTicketsRequest{EventId: r.URL.Query().Get("event_id")})
	if err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

// Update handles PUT /tickets/{id} -> 200 OK.
func (h *Tickets) Update(w http.ResponseWriter, r *http.Request) {
	var in updateTicketInput
	if !decodeJSON(w, r, &in) {
		return
	}
	t, err := h.client.UpdateTicket(r.Context(), &ticketpb.UpdateTicketRequest{
		Id: r.PathValue("id"), HolderName: in.HolderName, HolderEmail: in.HolderEmail, Quantity: in.Quantity, Status: in.Status,
	})
	if err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, t)
}

// Delete handles DELETE /tickets/{id} -> 204 No Content.
func (h *Tickets) Delete(w http.ResponseWriter, r *http.Request) {
	if _, err := h.client.DeleteTicket(r.Context(), &ticketpb.DeleteTicketRequest{Id: r.PathValue("id")}); err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
