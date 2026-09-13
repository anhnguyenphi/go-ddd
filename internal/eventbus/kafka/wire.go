package kafka

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/myapp/internal/eventbus"
)

// Codec (de)serializes an Event to the bytes carried as a Kafka message value.
// It is a seam for swapping wire formats (e.g. protobuf, Avro + schema
// registry) later without touching the transport or retry/DLQ logic.
type Codec interface {
	Encode(eventbus.Event) ([]byte, error)
	Decode([]byte) (eventbus.Event, error)
}

// JSONCodec is the default Codec: the whole Event envelope, JSON-encoded.
type JSONCodec struct{}

// wireEvent mirrors eventbus.Event with JSON tags. Kept separate from Event
// itself so the wire shape (tags, future versioning) can evolve independently
// of the in-process struct.
type wireEvent struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Source        string            `json:"source"`
	AggregateID   string            `json:"aggregate_id"`
	OccurredAt    time.Time         `json:"occurred_at"`
	Version       int               `json:"version"`
	CorrelationID string            `json:"correlation_id"`
	CausationID   string            `json:"causation_id"`
	Payload       []byte            `json:"payload"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func (JSONCodec) Encode(e eventbus.Event) ([]byte, error) {
	w := wireEvent{
		ID:            e.ID,
		Name:          e.Name,
		Source:        e.Source,
		AggregateID:   e.AggregateID,
		OccurredAt:    e.OccurredAt,
		Version:       e.Version,
		CorrelationID: e.CorrelationID,
		CausationID:   e.CausationID,
		Payload:       e.Payload,
		Metadata:      e.Metadata,
	}
	b, err := json.Marshal(w)
	if err != nil {
		return nil, fmt.Errorf("kafka: encode event %s: %w", e.Name, err)
	}
	return b, nil
}

func (JSONCodec) Decode(b []byte) (eventbus.Event, error) {
	var w wireEvent
	if err := json.Unmarshal(b, &w); err != nil {
		return eventbus.Event{}, fmt.Errorf("kafka: decode event: %w", err)
	}
	return eventbus.Event{
		ID:            w.ID,
		Name:          w.Name,
		Source:        w.Source,
		AggregateID:   w.AggregateID,
		OccurredAt:    w.OccurredAt,
		Version:       w.Version,
		CorrelationID: w.CorrelationID,
		CausationID:   w.CausationID,
		Payload:       w.Payload,
		Metadata:      w.Metadata,
	}, nil
}
