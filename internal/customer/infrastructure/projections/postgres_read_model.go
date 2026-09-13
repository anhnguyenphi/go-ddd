package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/example/myapp/internal/customer/application/dto"
	"github.com/example/myapp/internal/customer/infrastructure/messaging"
	"github.com/example/myapp/internal/eventbus"
	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/pkg/pagination"
)

// PostgresReadModel is the customer query side backed by a denormalised
// `customer_reads` table (see migrations/0003_customer_read_model.up.sql). It
// never touches the `customers` write table — it is kept eventually
// consistent by subscribing to this context's own integration events, the
// same way MemoryReadModel is. Swap the two behind ReadModel per environment
// (see internal/bootstrap/database.go).
type PostgresReadModel struct {
	db *sql.DB
}

// NewPostgresReadModel builds a read model over db. It assumes the
// customer_reads table already exists (migration 0003).
func NewPostgresReadModel(db *sql.DB) *PostgresReadModel {
	return &PostgresReadModel{db: db}
}

var _ ReadModel = (*PostgresReadModel)(nil)

// Register subscribes the projection's handlers to the bus.
func (m *PostgresReadModel) Register(bus eventbus.Subscriber) error {
	if err := bus.Subscribe(messaging.EventCustomerCreated, m.onCreated); err != nil {
		return err
	}
	if err := bus.Subscribe(messaging.EventCustomerEmailChanged, m.onEmailChanged); err != nil {
		return err
	}
	return bus.Subscribe(messaging.EventCustomerDeactivated, m.onDeactivated)
}

func (m *PostgresReadModel) onCreated(ctx context.Context, e eventbus.Event) error {
	var p struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	const q = `
		INSERT INTO customer_reads (id, name, email, status, created_at)
		VALUES ($1, $2, $3, 'active', $4)
		ON CONFLICT (id) DO UPDATE SET
			name  = EXCLUDED.name,
			email = EXCLUDED.email`
	_, err := m.db.ExecContext(ctx, q, p.ID, p.Name, p.Email, nonZero(e.OccurredAt))
	return err
}

func (m *PostgresReadModel) onEmailChanged(ctx context.Context, e eventbus.Event) error {
	var p struct {
		ID       string `json:"id"`
		NewEmail string `json:"new_email"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	const q = `UPDATE customer_reads SET email = $2 WHERE id = $1`
	_, err := m.db.ExecContext(ctx, q, p.ID, p.NewEmail)
	return err
}

func (m *PostgresReadModel) onDeactivated(ctx context.Context, e eventbus.Event) error {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	const q = `UPDATE customer_reads SET status = 'inactive' WHERE id = $1`
	_, err := m.db.ExecContext(ctx, q, p.ID)
	return err
}

// GetByID implements custapp.ReadModel.
func (m *PostgresReadModel) GetByID(ctx context.Context, id string) (dto.Customer, error) {
	const q = `SELECT id, name, email, status, created_at FROM customer_reads WHERE id = $1`
	var c dto.Customer
	err := m.db.QueryRowContext(ctx, q, id).Scan(&c.ID, &c.Name, &c.Email, &c.Status, &c.CreatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return dto.Customer{}, shareddomain.ErrNotFound
	case err != nil:
		return dto.Customer{}, err
	}
	c.CreatedAt = c.CreatedAt.UTC()
	return c, nil
}

// List implements custapp.ReadModel.
func (m *PostgresReadModel) List(ctx context.Context, page pagination.Page) (pagination.Result[dto.Customer], error) {
	const q = `
		SELECT id, name, email, status, created_at
		FROM customer_reads
		ORDER BY created_at, id
		LIMIT $1 OFFSET $2`
	rows, err := m.db.QueryContext(ctx, q, page.Limit, page.Offset)
	if err != nil {
		return pagination.Result[dto.Customer]{}, err
	}
	defer rows.Close()

	items := make([]dto.Customer, 0, page.Limit)
	for rows.Next() {
		var c dto.Customer
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Status, &c.CreatedAt); err != nil {
			return pagination.Result[dto.Customer]{}, err
		}
		c.CreatedAt = c.CreatedAt.UTC()
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return pagination.Result[dto.Customer]{}, err
	}

	var total int
	if err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM customer_reads`).Scan(&total); err != nil {
		return pagination.Result[dto.Customer]{}, err
	}

	return pagination.Result[dto.Customer]{
		Items:  items,
		Total:  total,
		Limit:  page.Limit,
		Offset: page.Offset,
	}, nil
}
