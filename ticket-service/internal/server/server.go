// Package server implements the TicketService gRPC API.
//
// Creating, updating and retrieving tickets requires event information that
// this service does not own, so those handlers call the Event Service through
// the resilient eventclient.
package server

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Gayley007/event-ticketing/gen/eventpb"
	"github.com/Gayley007/event-ticketing/gen/ticketpb"
	"github.com/Gayley007/event-ticketing/ticket-service/internal/repository"
	"github.com/Gayley007/event-ticketing/ticket-service/internal/validation"
)

// Store is the persistence the server depends on (satisfied by *repository.Repository).
type Store interface {
	Create(ctx context.Context, t repository.Ticket, capacity int32) (*repository.Ticket, error)
	Get(ctx context.Context, id string) (*repository.Ticket, error)
	List(ctx context.Context, eventID string) ([]*repository.Ticket, error)
	Update(ctx context.Context, t repository.Ticket, capacity *int32) (*repository.Ticket, error)
	Delete(ctx context.Context, id string) error
}

// Events is how the server reads events (satisfied by *eventclient.Client).
type Events interface {
	GetEvent(ctx context.Context, id string) (*eventpb.Event, error)
}

type Server struct {
	ticketpb.UnimplementedTicketServiceServer
	store  Store
	events Events
	log    *slog.Logger
	now    func() time.Time
}

func New(store Store, events Events, log *slog.Logger) *Server {
	return &Server{store: store, events: events, log: log, now: time.Now}
}

// CreateTicket confirms the event exists, has not started and has free seats
// before storing the ticket. Without a confirmed event it refuses to sell.
func (s *Server) CreateTicket(ctx context.Context, req *ticketpb.CreateTicketRequest) (*ticketpb.Ticket, error) {
	if err := validation.CreateTicket(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	ev, err := s.events.GetEvent(ctx, req.GetEventId())
	if err != nil {
		return nil, eventError(err, req.GetEventId(), "cannot create ticket")
	}
	if !ev.GetStartsAt().AsTime().After(s.now()) {
		return nil, status.Errorf(codes.FailedPrecondition, "event %q has already started; tickets are no longer sold", ev.GetName())
	}

	t, err := s.store.Create(ctx, repository.Ticket{
		EventID:     req.GetEventId(),
		HolderName:  strings.TrimSpace(req.GetHolderName()),
		HolderEmail: strings.TrimSpace(req.GetHolderEmail()),
		Quantity:    req.GetQuantity(),
	}, ev.GetCapacity())
	if err != nil {
		return nil, s.storeError(err, "")
	}

	resp := toProto(t)
	resp.Event = summary(ev)
	return resp, nil
}

// GetTicket returns the ticket enriched with event details. If the Event
// Service cannot be reached, the ticket (which this service owns) is still
// returned, with event=null and a warning - graceful degradation.
func (s *Server) GetTicket(ctx context.Context, req *ticketpb.GetTicketRequest) (*ticketpb.Ticket, error) {
	if err := validation.ID(req.GetId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	t, err := s.store.Get(ctx, req.GetId())
	if err != nil {
		return nil, s.storeError(err, req.GetId())
	}

	resp := toProto(t)
	ev, err := s.events.GetEvent(ctx, t.EventID)
	switch {
	case err == nil:
		resp.Event = summary(ev)
	case status.Code(err) == codes.NotFound:
		resp.Warning = "the event for this ticket no longer exists"
	default:
		s.log.Warn("returning ticket without event details (fallback)", "ticket_id", t.ID, "error", status.Convert(err).Message())
		resp.Warning = "event details are temporarily unavailable: " + status.Convert(err).Message()
	}
	return resp, nil
}

func (s *Server) ListTickets(ctx context.Context, req *ticketpb.ListTicketsRequest) (*ticketpb.ListTicketsResponse, error) {
	if err := validation.ListTickets(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	tickets, err := s.store.List(ctx, req.GetEventId())
	if err != nil {
		return nil, s.storeError(err, "")
	}
	resp := &ticketpb.ListTicketsResponse{Tickets: make([]*ticketpb.Ticket, 0, len(tickets))}
	for _, t := range tickets {
		resp.Tickets = append(resp.Tickets, toProto(t))
	}
	return resp, nil
}

// UpdateTicket edits holder details, quantity or status. Cancelling never
// needs the Event Service, but a CONFIRMED ticket re-checks the event's
// capacity, which does.
func (s *Server) UpdateTicket(ctx context.Context, req *ticketpb.UpdateTicketRequest) (*ticketpb.Ticket, error) {
	if err := validation.UpdateTicket(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	existing, err := s.store.Get(ctx, req.GetId())
	if err != nil {
		return nil, s.storeError(err, req.GetId())
	}

	var (
		ev       *eventpb.Event
		capacity *int32
	)
	if req.GetStatus() == validation.StatusConfirmed {
		ev, err = s.events.GetEvent(ctx, existing.EventID)
		if err != nil {
			return nil, eventError(err, existing.EventID, "cannot update ticket")
		}
		c := ev.GetCapacity()
		capacity = &c
	}

	t, err := s.store.Update(ctx, repository.Ticket{
		ID:          existing.ID,
		EventID:     existing.EventID,
		HolderName:  strings.TrimSpace(req.GetHolderName()),
		HolderEmail: strings.TrimSpace(req.GetHolderEmail()),
		Quantity:    req.GetQuantity(),
		Status:      req.GetStatus(),
	}, capacity)
	if err != nil {
		return nil, s.storeError(err, req.GetId())
	}

	resp := toProto(t)
	if ev != nil {
		resp.Event = summary(ev)
	}
	return resp, nil
}

func (s *Server) DeleteTicket(ctx context.Context, req *ticketpb.DeleteTicketRequest) (*ticketpb.DeleteTicketResponse, error) {
	if err := validation.ID(req.GetId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.store.Delete(ctx, req.GetId()); err != nil {
		return nil, s.storeError(err, req.GetId())
	}
	return &ticketpb.DeleteTicketResponse{}, nil
}

// eventError translates an error from the Event Service into one that makes
// sense to a Ticket Service caller, keeping the meaningful status code.
func eventError(err error, eventID, action string) error {
	st := status.Convert(err)
	switch st.Code() {
	case codes.NotFound:
		return status.Errorf(codes.NotFound, "event %s does not exist", eventID)
	case codes.Unavailable, codes.DeadlineExceeded:
		return status.Errorf(codes.Unavailable, "%s: %s", action, st.Message())
	case codes.InvalidArgument:
		return status.Error(codes.InvalidArgument, st.Message())
	default:
		return status.Errorf(codes.Internal, "%s: unexpected error from event service", action)
	}
}

func (s *Server) storeError(err error, id string) error {
	var capErr *repository.CapacityError
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return status.Errorf(codes.NotFound, "ticket %s not found", id)
	case errors.As(err, &capErr):
		return status.Errorf(codes.FailedPrecondition, "event is sold out for this request: only %d seat(s) remaining", capErr.Remaining)
	default:
		s.log.Error("database error", "err", err)
		return status.Error(codes.Internal, "internal error while accessing tickets")
	}
}

func toProto(t *repository.Ticket) *ticketpb.Ticket {
	return &ticketpb.Ticket{
		Id:          t.ID,
		EventId:     t.EventID,
		HolderName:  t.HolderName,
		HolderEmail: t.HolderEmail,
		Quantity:    t.Quantity,
		Status:      t.Status,
		CreatedAt:   timestamppb.New(t.CreatedAt),
		UpdatedAt:   timestamppb.New(t.UpdatedAt),
	}
}

func summary(ev *eventpb.Event) *ticketpb.EventSummary {
	return &ticketpb.EventSummary{
		Id:       ev.GetId(),
		Name:     ev.GetName(),
		Venue:    ev.GetVenue(),
		StartsAt: ev.GetStartsAt(),
		Price:    ev.GetPrice(),
	}
}
