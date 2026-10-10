// Package handlers turns REST requests into gRPC calls on the services.
// Handlers only translate HTTP <-> gRPC; business rules and validation live
// in the services themselves.
package handlers

import (
	"net/http"

	"github.com/Gayley007/event-ticketing/api-gateway/internal/apierror"
	"github.com/Gayley007/event-ticketing/gen/eventpb"
)

// eventInput is the JSON body for POST /events and PUT /events/{id}.
type eventInput struct {
	Name     string  `json:"name"`
	Venue    string  `json:"venue"`
	StartsAt string  `json:"starts_at"`
	Capacity int32   `json:"capacity"`
	Price    float64 `json:"price"`
}

type Events struct {
	client eventpb.EventServiceClient
}

func NewEvents(client eventpb.EventServiceClient) *Events {
	return &Events{client: client}
}

// Create handles POST /events -> 201 Created.
func (h *Events) Create(w http.ResponseWriter, r *http.Request) {
	var in eventInput
	if !decodeJSON(w, r, &in) {
		return
	}
	startsAt, err := parseTime("starts_at", in.StartsAt)
	if err != nil {
		apierror.BadRequest(w, err.Error())
		return
	}
	ev, err := h.client.CreateEvent(r.Context(), &eventpb.CreateEventRequest{
		Name: in.Name, Venue: in.Venue, StartsAt: startsAt, Capacity: in.Capacity, Price: in.Price,
	})
	if err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	w.Header().Set("Location", "/events/"+ev.GetId())
	writeProto(w, http.StatusCreated, ev)
}

// Get handles GET /events/{id} -> 200 OK.
func (h *Events) Get(w http.ResponseWriter, r *http.Request) {
	ev, err := h.client.GetEvent(r.Context(), &eventpb.GetEventRequest{Id: r.PathValue("id")})
	if err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, ev)
}

// List handles GET /events -> 200 OK.
func (h *Events) List(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.ListEvents(r.Context(), &eventpb.ListEventsRequest{})
	if err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

// Update handles PUT /events/{id} -> 200 OK.
func (h *Events) Update(w http.ResponseWriter, r *http.Request) {
	var in eventInput
	if !decodeJSON(w, r, &in) {
		return
	}
	startsAt, err := parseTime("starts_at", in.StartsAt)
	if err != nil {
		apierror.BadRequest(w, err.Error())
		return
	}
	ev, err := h.client.UpdateEvent(r.Context(), &eventpb.UpdateEventRequest{
		Id: r.PathValue("id"), Name: in.Name, Venue: in.Venue, StartsAt: startsAt, Capacity: in.Capacity, Price: in.Price,
	})
	if err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	writeProto(w, http.StatusOK, ev)
}

// Delete handles DELETE /events/{id} -> 204 No Content.
func (h *Events) Delete(w http.ResponseWriter, r *http.Request) {
	if _, err := h.client.DeleteEvent(r.Context(), &eventpb.DeleteEventRequest{Id: r.PathValue("id")}); err != nil {
		apierror.WriteGRPC(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
