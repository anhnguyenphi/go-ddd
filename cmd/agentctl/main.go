// Command agentctl is a developer/agent tool for exercising the Kafka event
// bus directly, end to end, without writing a throwaway Go program each time:
//
//	agentctl kafka publish -name customer.v1.created -aggregate-id c-1 \
//	    -payload '{"id":"c-1","name":"Ada","email":"ada@example.com"}'
//	agentctl kafka consume -name customer.v1.created -aggregate-id c-1 -timeout 10s
//
// publish reuses the production kafka.Bus.Publish path (same topic-naming
// convention, same JSON wire codec), so a message it produces is
// indistinguishable from one the real app would have produced — useful for
// driving a consumer (the read-model projection, notification, ...) without
// going through the HTTP/gRPC API and the outbox.
//
// consume reads with its own reader, deliberately not joining the app's
// consumer group (Config.GroupID), so inspecting a topic never steals
// partitions or commits offsets for the real consumers.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/eventbus/kafka"
	"github.com/example/myapp/internal/shared/infrastructure/id"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "agentctl:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 || args[0] != "kafka" {
		usage()
		if len(args) == 0 {
			return errors.New("no command given")
		}
		return fmt.Errorf("unknown command %q", strings.Join(args, " "))
	}

	switch args[1] {
	case "publish":
		return runPublish(args[2:])
	case "consume":
		return runConsume(args[2:])
	default:
		usage()
		return fmt.Errorf("unknown kafka subcommand %q", args[1])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `agentctl - drive the Kafka event bus for end-to-end testing

Usage:
  agentctl kafka publish -name <event-name> -aggregate-id <id> -payload <json> [flags]
  agentctl kafka consume -name <event-name> | -topic <topic> [flags]

Run "agentctl kafka publish -h" or "agentctl kafka consume -h" for flags.`)
}

// ---- publish ---------------------------------------------------------------

func runPublish(args []string) error {
	fs := flag.NewFlagSet("kafka publish", flag.ExitOnError)
	brokers := fs.String("brokers", "localhost:9092", "comma-separated Kafka brokers")
	name := fs.String("name", "", "event name, e.g. customer.v1.created (required)")
	topic := fs.String("topic", "", "override the destination topic (default: derived from -name, matching production)")
	source := fs.String("source", "", "producing context, e.g. customer (default: first dot-segment of -name)")
	aggregateID := fs.String("aggregate-id", "", "aggregate the event is about (also the partition key)")
	version := fs.Int("version", 1, "payload schema version")
	payload := fs.String("payload", "", "JSON payload body")
	payloadFile := fs.String("payload-file", "", "read the JSON payload from this file instead of -payload")
	correlationID := fs.String("correlation-id", "", "default: a fresh id")
	causationID := fs.String("causation-id", "", "")
	eventID := fs.String("id", "", "default: a fresh id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		return errors.New("-name is required")
	}
	body, err := readPayload(*payload, *payloadFile)
	if err != nil {
		return err
	}
	if !json.Valid(body) {
		return errors.New("-payload/-payload-file must be valid JSON")
	}

	src := *source
	if src == "" {
		if i := strings.IndexByte(*name, '.'); i >= 0 {
			src = (*name)[:i]
		} else {
			src = *name
		}
	}

	e := eventbus.Event{
		ID:            orDefault(*eventID, id.New),
		Name:          *name,
		Source:        src,
		AggregateID:   *aggregateID,
		OccurredAt:    time.Now().UTC(),
		Version:       *version,
		CorrelationID: orDefault(*correlationID, id.New),
		CausationID:   *causationID,
		Payload:       body,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg := kafka.Config{Brokers: splitCSV(*brokers)}
	if *topic != "" {
		cfg.TopicFor = func(string) string { return *topic }
	}
	bus := kafka.New(cfg, logger)
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := bus.Publish(ctx, e); err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	dest := *topic
	if dest == "" {
		dest = kafka.DefaultTopicFor(*name)
	}
	fmt.Printf("published %s (id=%s aggregate_id=%s) to topic %q\n", e.Name, e.ID, e.AggregateID, dest)
	return nil
}

func readPayload(inline, file string) ([]byte, error) {
	switch {
	case file != "":
		return os.ReadFile(file)
	case inline != "":
		return []byte(inline), nil
	default:
		return nil, errors.New("one of -payload or -payload-file is required")
	}
}

func orDefault(v string, gen func() string) string {
	if v != "" {
		return v
	}
	return gen()
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---- consume ----------------------------------------------------------------

func runConsume(args []string) error {
	fs := flag.NewFlagSet("kafka consume", flag.ExitOnError)
	brokers := fs.String("brokers", "localhost:9092", "comma-separated Kafka brokers")
	topic := fs.String("topic", "", "topic to read (default: derived from -name, matching production)")
	name := fs.String("name", "", "only print events with this name (also used to derive -topic)")
	aggregateID := fs.String("aggregate-id", "", "only print events with this aggregate id")
	count := fs.Int("count", 1, "stop after this many matching messages")
	timeout := fs.Duration("timeout", 10*time.Second, "give up after this long")
	fromBeginning := fs.Bool("from-beginning", true, "read from the start of the topic (false: only new messages)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	t := *topic
	if t == "" {
		if *name == "" {
			return errors.New("one of -topic or -name is required")
		}
		t = kafka.DefaultTopicFor(*name)
	}

	startOffset := kafkago.LastOffset
	if *fromBeginning {
		startOffset = kafkago.FirstOffset
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     splitCSV(*brokers),
		Topic:       t,
		Partition:   0,
		MinBytes:    1,
		MaxBytes:    10e6,
		StartOffset: startOffset,
		// No GroupID: this reads the partition directly and commits nothing,
		// so it never interferes with the real app's consumer group.
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	codec := kafka.JSONCodec{}
	matched := 0
	for matched < *count {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			return fmt.Errorf("fetch: %w", err)
		}
		ev, err := codec.Decode(msg.Value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agentctl: skipping undecodable message at offset %d: %v\n", msg.Offset, err)
			continue
		}
		if *name != "" && ev.Name != *name {
			continue
		}
		if *aggregateID != "" && ev.AggregateID != *aggregateID {
			continue
		}
		if err := printEvent(os.Stdout, t, msg, ev); err != nil {
			return err
		}
		matched++
	}

	if matched == 0 {
		return fmt.Errorf("no matching message on topic %q within %s", t, *timeout)
	}
	return nil
}

func printEvent(w io.Writer, topic string, msg kafkago.Message, ev eventbus.Event) error {
	var payload any = json.RawMessage(ev.Payload)
	if !json.Valid(ev.Payload) {
		payload = ev.Payload // fall back to raw bytes (base64 via json.Marshal)
	}
	out := map[string]any{
		"topic":          topic,
		"partition":      msg.Partition,
		"offset":         msg.Offset,
		"id":             ev.ID,
		"name":           ev.Name,
		"source":         ev.Source,
		"aggregate_id":   ev.AggregateID,
		"occurred_at":    ev.OccurredAt,
		"version":        ev.Version,
		"correlation_id": ev.CorrelationID,
		"causation_id":   ev.CausationID,
		"payload":        payload,
		"metadata":       ev.Metadata,
	}
	enc := json.NewEncoder(w)
	return enc.Encode(out)
}
