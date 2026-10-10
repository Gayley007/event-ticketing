// Package server implements the EventService gRPC API. Handlers are thin:
// validate the request, call the repository, translate errors to gRPC codes.
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

	"github.com/Gayley007/event-ticketing/event-service/internal/repository"
	"github.com/Gayley007/event-ticketing/event-service/internal/validation"
	"github.com/Gayley007/event-ticketing/gen/eventpb"
)

// Store is the persistence the server depends on (satisfied by *repository.Repository).
type Store interface {
	Create(ctx context.Context, e repository.Event) (*repository.Event, error)
	Get(ctx context.Context, id string) (*repository.Event, error)
	List(ctx context.Context) ([]*repository.Event, error)
	Update(ctx context.Context, e repository.Event) (*repository.Event, error)
	Delete(ctx context.Context, id string) error
}

type Server struct {
	eventpb.UnimplementedEventServiceServer
	store Store
	log   *slog.Logger
	now   func() time.Time
}

func New(store Store, log *slog.Logger) *Server {
	return &Server{store: store, log: log, now: time.Now}
}

func (s *Server) CreateEvent(ctx context.Context, req *eventpb.CreateEventRequest) (*eventpb.Event, error) {
	if err := validation.CreateEvent(req, s.now()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	e, err := s.store.Create(ctx, repository.Event{
		Name:     strings.TrimSpace(req.GetName()),
		Venue:    strings.TrimSpace(req.GetVenue()),
		StartsAt: req.GetStartsAt().AsTime(),
		Capacity: req.GetCapacity(),
		Price:    req.GetPrice(),
	})
	if err != nil {
		return nil, s.storeError(err, "")
	}
	return toProto(e), nil
}

func (s *Server) GetEvent(ctx context.Context, req *eventpb.GetEventRequest) (*eventpb.Event, error) {
	if err := validation.ID(req.GetId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	e, err := s.store.Get(ctx, req.GetId())
	if err != nil {
		return nil, s.storeError(err, req.GetId())
	}
	return toProto(e), nil
}

func (s *Server) ListEvents(ctx context.Context, _ *eventpb.ListEventsRequest) (*eventpb.ListEventsResponse, error) {
	events, err := s.store.List(ctx)
	if err != nil {
		return nil, s.storeError(err, "")
	}
	resp := &eventpb.ListEventsResponse{Events: make([]*eventpb.Event, 0, len(events))}
	for _, e := range events {
		resp.Events = append(resp.Events, toProto(e))
	}
	return resp, nil
}

func (s *Server) UpdateEvent(ctx context.Context, req *eventpb.UpdateEventRequest) (*eventpb.Event, error) {
	if err := validation.UpdateEvent(req, s.now()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	e, err := s.store.Update(ctx, repository.Event{
		ID:       req.GetId(),
		Name:     strings.TrimSpace(req.GetName()),
		Venue:    strings.TrimSpace(req.GetVenue()),
		StartsAt: req.GetStartsAt().AsTime(),
		Capacity: req.GetCapacity(),
		Price:    req.GetPrice(),
	})
	if err != nil {
		return nil, s.storeError(err, req.GetId())
	}
	return toProto(e), nil
}

func (s *Server) DeleteEvent(ctx context.Context, req *eventpb.DeleteEventRequest) (*eventpb.DeleteEventResponse, error) {
	if err := validation.ID(req.GetId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.store.Delete(ctx, req.GetId()); err != nil {
		return nil, s.storeError(err, req.GetId())
	}
	return &eventpb.DeleteEventResponse{}, nil
}

// storeError maps repository errors to gRPC status codes. Unexpected database
// errors are logged but hidden from the caller behind codes.Internal.
func (s *Server) storeError(err error, id string) error {
	if errors.Is(err, repository.ErrNotFound) {
		return status.Errorf(codes.NotFound, "event %s not found", id)
	}
	s.log.Error("database error", "err", err)
	return status.Error(codes.Internal, "internal error while accessing events")
}

func toProto(e *repository.Event) *eventpb.Event {
	return &eventpb.Event{
		Id:        e.ID,
		Name:      e.Name,
		Venue:     e.Venue,
		StartsAt:  timestamppb.New(e.StartsAt),
		Capacity:  e.Capacity,
		Price:     e.Price,
		CreatedAt: timestamppb.New(e.CreatedAt),
		UpdatedAt: timestamppb.New(e.UpdatedAt),
	}
}
