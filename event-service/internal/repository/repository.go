// Package repository is the only code that talks to the Event Service's
// database (event-db). No other service has credentials for it.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when no event has the requested ID.
var ErrNotFound = errors.New("event not found")

// Event is the database representation of an event.
type Event struct {
	ID        string
	Name      string
	Venue     string
	StartsAt  time.Time
	Capacity  int32
	Price     float64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository stores events in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Migrate creates the events table if it does not exist yet.
// The CHECK constraints are a second line of defence behind validation.
func (r *Repository) Migrate(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS events (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name       TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
			venue      TEXT NOT NULL CHECK (length(venue) BETWEEN 1 AND 150),
			starts_at  TIMESTAMPTZ NOT NULL,
			capacity   INTEGER NOT NULL CHECK (capacity > 0),
			price      DOUBLE PRECISION NOT NULL CHECK (price >= 0),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("migrate events: %w", err)
	}
	return nil
}

const columns = `id, name, venue, starts_at, capacity, price, created_at, updated_at`

func scan(row pgx.Row) (*Event, error) {
	var e Event
	err := row.Scan(&e.ID, &e.Name, &e.Venue, &e.StartsAt, &e.Capacity, &e.Price, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *Repository) Create(ctx context.Context, e Event) (*Event, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO events (name, venue, starts_at, capacity, price)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+columns,
		e.Name, e.Venue, e.StartsAt, e.Capacity, e.Price)
	return scan(row)
}

func (r *Repository) Get(ctx context.Context, id string) (*Event, error) {
	return scan(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM events WHERE id = $1`, id))
}

func (r *Repository) List(ctx context.Context) ([]*Event, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columns+` FROM events ORDER BY starts_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *Repository) Update(ctx context.Context, e Event) (*Event, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE events
		SET name = $2, venue = $3, starts_at = $4, capacity = $5, price = $6, updated_at = now()
		WHERE id = $1
		RETURNING `+columns,
		e.ID, e.Name, e.Venue, e.StartsAt, e.Capacity, e.Price)
	return scan(row)
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM events WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
