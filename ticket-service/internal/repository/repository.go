// Package repository is the only code that talks to the Ticket Service's
// database (ticket-db). It stores an event's ID but never any event data, and
// it never queries the event database.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when no ticket has the requested ID.
var ErrNotFound = errors.New("ticket not found")

// CapacityError is returned when a ticket would oversell its event.
type CapacityError struct {
	Remaining int32
}

func (e *CapacityError) Error() string {
	return fmt.Sprintf("not enough seats left: %d remaining", e.Remaining)
}

const statusConfirmed = "CONFIRMED"

// Ticket is the database representation of a ticket.
type Ticket struct {
	ID          string
	EventID     string
	HolderName  string
	HolderEmail string
	Quantity    int32
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Repository stores tickets in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Migrate creates the tickets table if it does not exist yet. event_id is a
// plain UUID column - deliberately NOT a foreign key, because the events
// table lives in another service's database.
func (r *Repository) Migrate(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS tickets (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			event_id     UUID NOT NULL,
			holder_name  TEXT NOT NULL CHECK (length(holder_name) BETWEEN 1 AND 100),
			holder_email TEXT NOT NULL CHECK (position('@' in holder_email) > 1),
			quantity     INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 10),
			status       TEXT NOT NULL CHECK (status IN ('CONFIRMED', 'CANCELLED')),
			created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS tickets_event_id_idx ON tickets (event_id);`)
	if err != nil {
		return fmt.Errorf("migrate tickets: %w", err)
	}
	return nil
}

const columns = `id, event_id, holder_name, holder_email, quantity, status, created_at, updated_at`

func scan(row pgx.Row) (*Ticket, error) {
	var t Ticket
	err := row.Scan(&t.ID, &t.EventID, &t.HolderName, &t.HolderEmail, &t.Quantity, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// checkCapacity locks the event's seat count for the rest of the transaction
// and returns a CapacityError if adding `quantity` seats would exceed
// `capacity`. excludeID lets an update ignore the ticket's own current seats.
//
// The capacity comes from the Event Service (over gRPC); the sold count comes
// from this service's own table. The advisory lock serialises concurrent
// purchases for the same event so two buyers cannot take the last seat.
func checkCapacity(ctx context.Context, tx pgx.Tx, eventID, excludeID string, quantity, capacity int32) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, eventID); err != nil {
		return err
	}
	var sold int32
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(quantity), 0)::int FROM tickets
		WHERE event_id = $1 AND status = $2 AND ($3 = '' OR id::text <> $3)`,
		eventID, statusConfirmed, excludeID).Scan(&sold)
	if err != nil {
		return err
	}
	if sold+quantity > capacity {
		return &CapacityError{Remaining: max(capacity-sold, 0)}
	}
	return nil
}

// Create inserts a CONFIRMED ticket if the event still has enough seats.
func (r *Repository) Create(ctx context.Context, t Ticket, capacity int32) (*Ticket, error) {
	var created *Ticket
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := checkCapacity(ctx, tx, t.EventID, "", t.Quantity, capacity); err != nil {
			return err
		}
		var err error
		created, err = scan(tx.QueryRow(ctx, `
			INSERT INTO tickets (event_id, holder_name, holder_email, quantity, status)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING `+columns,
			t.EventID, t.HolderName, t.HolderEmail, t.Quantity, statusConfirmed))
		return err
	})
	return created, err
}

func (r *Repository) Get(ctx context.Context, id string) (*Ticket, error) {
	return scan(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM tickets WHERE id = $1`, id))
}

// List returns all tickets, optionally only those for one event.
func (r *Repository) List(ctx context.Context, eventID string) ([]*Ticket, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+columns+` FROM tickets
		WHERE $1 = '' OR event_id::text = $1
		ORDER BY created_at`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []*Ticket
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

// Update changes a ticket's editable fields. When capacity is non-nil and the
// ticket stays/becomes CONFIRMED, the seat limit is re-checked.
func (r *Repository) Update(ctx context.Context, t Ticket, capacity *int32) (*Ticket, error) {
	var updated *Ticket
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if capacity != nil && t.Status == statusConfirmed {
			if err := checkCapacity(ctx, tx, t.EventID, t.ID, t.Quantity, *capacity); err != nil {
				return err
			}
		}
		var err error
		updated, err = scan(tx.QueryRow(ctx, `
			UPDATE tickets
			SET holder_name = $2, holder_email = $3, quantity = $4, status = $5, updated_at = now()
			WHERE id = $1
			RETURNING `+columns,
			t.ID, t.HolderName, t.HolderEmail, t.Quantity, t.Status))
		return err
	})
	return updated, err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tickets WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
