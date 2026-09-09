For a large-scale Go system using DDD and an event bus, I’d structure the repository around **bounded contexts**, with a strict separation between **domain**, **application/use cases**, and **infrastructure/adapters**.

A good enterprise-oriented baseline looks like this:

```text
myapp/
├── cmd/
│   ├── api/
│   │   └── main.go
│   ├── worker/
│   │   └── main.go
│   └── migrate/
│       └── main.go
│
├── internal/
│   ├── shared/
│   │   ├── domain/
│   │   │   ├── aggregate.go
│   │   │   ├── entity.go
│   │   │   ├── value_object.go
│   │   │   ├── domain_event.go
│   │   │   ├── repository.go
│   │   │   └── errors.go
│   │   │
│   │   ├── application/
│   │   │   ├── command.go
│   │   │   ├── query.go
│   │   │   └── unit_of_work.go
│   │   │
│   │   └── infrastructure/
│   │       ├── clock/
│   │       ├── id/
│   │       ├── transaction/
│   │       └── logging/
│   │
│   ├── eventbus/
│   │   ├── event.go
│   │   ├── bus.go
│   │   ├── publisher.go
│   │   ├── subscriber.go
│   │   ├── middleware.go
│   │   ├── local/
│   │   │   └── bus.go
│   │   ├── kafka/
│   │   │   └── bus.go
│   │   └── outbox/
│   │       ├── publisher.go
│   │       └── relay.go
│   │
│   ├── identity/
│   │   ├── domain/
│   │   ├── application/
│   │   ├── infrastructure/
│   │   └── interfaces/
│   │
│   ├── customer/
│   │   ├── domain/
│   │   │   ├── customer.go
│   │   │   ├── customer_id.go
│   │   │   ├── customer_repository.go
│   │   │   ├── events/
│   │   │   │   ├── customer_created.go
│   │   │   │   └── customer_updated.go
│   │   │   └── services/
│   │   │       └── customer_policy.go
│   │   │
│   │   ├── application/
│   │   │   ├── commands/
│   │   │   │   ├── create_customer.go
│   │   │   │   └── update_customer.go
│   │   │   ├── queries/
│   │   │   │   └── get_customer.go
│   │   │   ├── handlers/
│   │   │   └── dto/
│   │   │
│   │   ├── infrastructure/
│   │   │   ├── persistence/
│   │   │   │   ├── postgres/
│   │   │   │   └── repository.go
│   │   │   ├── messaging/
│   │   │   │   ├── event_handlers.go
│   │   │   │   └── event_mappers.go
│   │   │   └── projections/
│   │   │
│   │   └── interfaces/
│   │       ├── http/
│   │       │   ├── handler.go
│   │       │   ├── routes.go
│   │       │   └── response.go
│   │       ├── grpc/
│   │       └── consumers/
│   │
│   ├── order/
│   │   ├── domain/
│   │   ├── application/
│   │   ├── infrastructure/
│   │   └── interfaces/
│   │
│   ├── payment/
│   │   ├── domain/
│   │   ├── application/
│   │   ├── infrastructure/
│   │   └── interfaces/
│   │
│   └── ...
│
├── pkg/
│   └── ...
│
├── api/
│   ├── openapi/
│   └── proto/
│
├── migrations/
│
├── deployments/
│   ├── docker/
│   ├── kubernetes/
│   └── helm/
│
├── configs/
│
├── scripts/
│
├── tests/
│   ├── integration/
│   ├── contract/
│   └── e2e/
│
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
└── README.md
```

## The important architectural boundary

For a large system, I would **not** organize the whole application like this:

```text
internal/
├── domain/
├── application/
├── repository/
├── handlers/
└── services/
```

That works for small applications, but eventually every bounded context starts sharing the same packages and boundaries become blurry.

Instead, use:

```text
internal/
├── customer/
├── order/
├── payment/
├── identity/
└── ...
```

Each bounded context owns its own DDD layers.

Inside a context:

```text
customer/
├── domain/
├── application/
├── infrastructure/
└── interfaces/
```

This is essentially **DDD + Hexagonal/Clean Architecture + modular monolith/microservice friendly boundaries**.

---

# 1. Domain layer

The domain layer should contain **business rules**, not infrastructure concerns.

Example:

```go
package customer

type Customer struct {
	id     CustomerID
	name   string
	email  Email
	status Status
}

func NewCustomer(
	id CustomerID,
	name string,
	email Email,
) (*Customer, error) {
	if name == "" {
		return nil, ErrInvalidName
	}

	return &Customer{
		id:     id,
		name:   name,
		email:  email,
		status: Active,
	}, nil
}

func (c *Customer) ChangeEmail(email Email) error {
	c.email = email

	return nil
}
```

Repositories are interfaces owned by the domain:

```go
type CustomerRepository interface {
	GetByID(ctx context.Context, id CustomerID) (*Customer, error)
	Save(ctx context.Context, customer *Customer) error
}
```

The domain should **not** know whether you're using:

```text
PostgreSQL
MongoDB
Redis
Kafka
HTTP
gRPC
```

---

# 2. Domain events

I'd make domain events first-class concepts.

For example:

```go
package events

type CustomerCreated struct {
	CustomerID string
	Name       string
	Email      string
}
```

Or define a common event contract:

```go
type DomainEvent interface {
	EventName() string
	OccurredAt() time.Time
	AggregateID() string
}
```

An aggregate can collect events:

```go
type AggregateRoot struct {
	events []DomainEvent
}

func (a *AggregateRoot) AddEvent(event DomainEvent) {
	a.events = append(a.events, event)
}

func (a *AggregateRoot) PullEvents() []DomainEvent {
	events := a.events
	a.events = nil

	return events
}
```

Then:

```go
func (c *Customer) Activate() error {
	// business logic

	c.AddEvent(events.CustomerActivated{
		CustomerID: c.ID().String(),
	})

	return nil
}
```

This gives you a clean distinction between:

```text
Domain Event
     ↓
Application
     ↓
Event Bus
     ↓
Other bounded contexts / external systems
```

---

# 3. Application layer

The application layer orchestrates use cases.

It should answer:

> "What steps need to happen to execute this business operation?"

For example:

```go
type CreateCustomerHandler struct {
	repository CustomerRepository
	eventBus   eventbus.Bus
	uow        UnitOfWork
}

func (h *CreateCustomerHandler) Handle(
	ctx context.Context,
	cmd CreateCustomer,
) error {

	return h.uow.Within(ctx, func(ctx context.Context) error {

		customer, err := customer.NewCustomer(
			NewCustomerID(),
			cmd.Name,
			cmd.Email,
		)
		if err != nil {
			return err
		}

		if err := h.repository.Save(ctx, customer); err != nil {
			return err
		}

		return nil
	})
}
```

The application layer shouldn't implement business rules belonging to aggregates.

Think:

```text
Application = orchestration

Domain = business rules
```

---

# 4. Infrastructure

Infrastructure implements interfaces defined by the domain/application layers.

For example:

```text
customer/domain/customer_repository.go
```

contains:

```go
type CustomerRepository interface {
	GetByID(...)
	Save(...)
}
```

While:

```text
customer/infrastructure/persistence/postgres/repository.go
```

contains:

```go
type Repository struct {
	db *sql.DB
}

func (r *Repository) GetByID(...) (*customer.Customer, error) {
	// PostgreSQL implementation
}
```

This dependency direction is important:

```text
        interfaces
             ↑
        application
             ↑
          domain
             ↑
    infrastructure
```

More precisely, infrastructure **depends on** domain abstractions.

---

# 5. Event bus

For enterprise systems I'd introduce a generic event bus abstraction.

```go
type Bus interface {
	Publish(
		ctx context.Context,
		event Event,
	) error

	Subscribe(
		eventName string,
		handler Handler,
	) error
}
```

Events might have metadata:

```go
type Event struct {
	ID            string
	Name          string
	Type          string
	AggregateID   string
	OccurredAt    time.Time
	Version       int
	CorrelationID string
	CausationID   string
	Payload       []byte
	Metadata      map[string]string
}
```

This becomes extremely valuable once you have distributed systems.

For example:

```text
CustomerCreated
       │
       ▼
    Event Bus
       │
       ├───────────────► CRM
       │
       ├───────────────► Notification
       │
       ├───────────────► Analytics
       │
       └───────────────► Loyalty
```

---

# 6. Use the Outbox Pattern

For a large enterprise system, I'd strongly recommend **Outbox** rather than doing:

```go
db.Save(customer)

eventBus.Publish(event)
```

because this can fail:

```text
Database commit ✅
Event publish    ❌
```

Now the state changed but nobody receives the event.

Instead:

```text
BEGIN TRANSACTION

    save customer

    save event to outbox

COMMIT
```

Then:

```text
                    ┌──────────────┐
                    │ PostgreSQL   │
                    │              │
                    │ Customer     │
                    │ Outbox       │
                    └──────┬───────┘
                           │
                           │ poll
                           ▼
                    ┌──────────────┐
                    │ Outbox Relay │
                    └──────┬───────┘
                           │
                           ▼
                    ┌──────────────┐
                    │ Kafka / Bus  │
                    └──────────────┘
```

Your repository/unit-of-work should make this atomic.

Something conceptually like:

```go
func (u *UnitOfWork) Within(
	ctx context.Context,
	fn func(ctx context.Context) error,
) error {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	ctx = WithTx(ctx, tx)

	if err := fn(ctx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
```

The outbox record is written inside that same transaction.

---

# 7. Event handlers

Each bounded context can consume events without directly depending on another bounded context's internals.

For example:

```text
customer/
    publishes
        CustomerCreated

            ↓

event bus

            ↓

notification/
    handles
        CustomerCreated
```

The notification context might have:

```text
notification/
├── domain/
├── application/
│   └── handlers/
│       └── customer_created.go
├── infrastructure/
│   └── messaging/
│       └── customer_events.go
└── interfaces/
```

Handler:

```go
type CustomerCreatedHandler struct {
	notificationService NotificationService
}

func (h *CustomerCreatedHandler) Handle(
	ctx context.Context,
	event events.CustomerCreated,
) error {

	return h.notificationService.SendWelcomeEmail(
		ctx,
		event.Email,
	)
}
```

Notice that notification does not need:

```go
customer.Customer
customer.CustomerRepository
```

It only depends on the **published contract/event**.

---

# 8. Event contracts

At enterprise scale I'd separate:

```text
Domain Events
```

from:

```text
Integration Events
```

For example:

```text
CustomerCreated
```

inside the domain isn't necessarily the same thing as the public event:

```text
customer.v1.created
```

You can have:

```text
customer/domain/events/
    customer_created.go

customer/infrastructure/messaging/
    customer_created_mapper.go
```

Mapper:

```go
func ToIntegrationEvent(
	event domain.CustomerCreated,
) eventbus.Event {
	return eventbus.Event{
		Name: "customer.v1.created",
		// ...
	}
}
```

This protects your domain model from external messaging contracts.

---

# 9. Version your integration events

For enterprise systems:

```text
customer.v1.created
customer.v2.created
```

rather than blindly changing:

```text
customer.created
```

This allows consumers to migrate independently.

Typical Kafka topics could be:

```text
customer.v1.events
order.v1.events
payment.v1.events
```

or event-type-based:

```text
customer.created.v1
customer.updated.v1
order.placed.v1
payment.completed.v1
```

I'd generally prefer **bounded-context-oriented topics** for large systems unless there's a specific reason to split them further.

---

# 10. Dependency Injection

Avoid global variables and package-level service locators.

Compose the system in `cmd/api/main.go`:

```go
func main() {
	cfg := config.Load()

	db := postgres.New(cfg.Database)
	eventBus := kafka.New(cfg.Kafka)

	customerRepo := customerpostgres.NewRepository(db)

	createCustomer := customerapp.NewCreateCustomerHandler(
		customerRepo,
		eventBus,
		// ...
	)

	httpServer := http.NewServer(
		createCustomer,
	)

	httpServer.Run()
}
```

For larger systems, use a composition root:

```text
internal/bootstrap/
├── database.go
├── messaging.go
├── repositories.go
├── services.go
└── application.go
```

Then:

```go
func BuildApplication(cfg Config) (*Application, error)
```

---

# 11. Interfaces/adapters

I would explicitly keep external interfaces separated.

For example:

```text
customer/interfaces/
├── http/
├── grpc/
├── consumers/
└── jobs/
```

Meaning a customer use case might be triggered by:

```text
HTTP
gRPC
Kafka
Cron
CLI
```

but all invoke the same application layer.

```text
             HTTP
               │
               ▼
          Application
               ▲
               │
gRPC ──────────┤
               │
Kafka ─────────┤
               │
Cron ──────────┘
```

---

# 12. Recommended package dependency rules

I'd enforce these rules with architecture tests.

### Domain

Can depend on:

```text
Go standard library
shared domain abstractions
```

Cannot depend on:

```text
database
Kafka
HTTP
gRPC
frameworks
```

### Application

Can depend on:

```text
domain
shared application abstractions
```

Should not depend directly on:

```text
Postgres implementation
Kafka implementation
HTTP framework
```

### Infrastructure

Can depend on:

```text
domain
application
external libraries
```

### Interfaces

Can depend on:

```text
application
DTOs
transport libraries
```

---

# 13. A more realistic enterprise structure

Once the system gets very large, I'd probably evolve it into something like:

```text
internal/
│
├── platform/
│   ├── config/
│   ├── database/
│   ├── messaging/
│   ├── observability/
│   ├── security/
│   └── http/
│
├── shared/
│   ├── domain/
│   ├── application/
│   └── infrastructure/
│
├── eventbus/
│
├── identity/
├── customer/
├── catalog/
├── inventory/
├── order/
├── payment/
├── shipment/
├── notification/
└── reporting/
```

The distinction is:

```text
platform/
```

contains technical capabilities used by the entire application.

```text
shared/
```

contains genuinely shared business/architectural abstractions.

```text
customer/
order/
payment/
```

contain business capabilities.

That distinction becomes important because `shared` otherwise tends to become a gigantic dumping ground.

---

# 14. One thing I'd avoid

Don't create a giant:

```text
internal/common/
```

with things like:

```text
common/
├── user.go
├── order.go
├── customer.go
├── payment.go
├── kafka.go
├── database.go
├── utils.go
└── helpers.go
```

That's usually where a supposedly DDD architecture slowly turns into a distributed monolith with shared mutable concepts.

Prefer:

```text
customer/domain
order/domain
payment/domain
```

and communicate across contexts through:

```text
commands
queries
events
```

---

# 15. The architecture I'd target

For a serious enterprise Go system, my default would be:

```text
                    ┌─────────────────────┐
                    │      API / gRPC     │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │   Application       │
                    │   Use Cases         │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │       Domain        │
                    │ Aggregates / Rules  │
                    │ Entities / Events   │
                    └──────────┬──────────┘
                               │
                ┌──────────────┴──────────────┐
                ▼                             ▼
       ┌────────────────┐            ┌────────────────┐
       │  Persistence    │            │ Domain Events  │
       │  Repository     │            │                │
       └───────┬────────┘            └───────┬────────┘
               │                             │
               ▼                             ▼
          PostgreSQL                     Outbox
                                             │
                                             ▼
                                         Kafka/Event Bus
                                             │
                     ┌───────────────────────┼────────────────────┐
                     ▼                       ▼                    ▼
                Notification              Billing             Analytics
```

The key architectural principle is:

**DDD defines the business boundaries; the event bus connects those boundaries; the outbox makes event publication reliable.**

For a system of this scale, I would also strongly consider making each bounded context independently deployable eventually, while initially keeping them in one repository. That gives you a **modular monolith → microservices** migration path without forcing microservices complexity on day one.
