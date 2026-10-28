package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/example/myapp/internal/eventbus/kafka"
)

// offsetMark maps "topic/partition" to that partition's end offset (the
// offset the next produced message will get) at the time of the mark.
type offsetMark map[string]int64

func markKey(topic string, partition int) string {
	return topic + "/" + strconv.Itoa(partition)
}

// ---- mark -------------------------------------------------------------------

func runMark(args []string) error {
	fs := flag.NewFlagSet("kafka mark", flag.ExitOnError)
	brokers := fs.String("brokers", "localhost:9092", "comma-separated Kafka brokers")
	out := fs.String("o", "", "write the mark to this file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	parts, err := listPartitions(ctx, splitCSV(*brokers))
	if err != nil {
		return err
	}
	m := offsetMark{}
	for _, p := range parts {
		_, end, err := partitionBounds(ctx, splitCSV(*brokers), p.Topic, p.ID)
		if err != nil {
			return err
		}
		m[markKey(p.Topic, p.ID)] = end
	}

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if *out == "" {
		_, err = fmt.Println(string(b))
		return err
	}
	return os.WriteFile(*out, append(b, '\n'), 0o644)
}

// ---- events -----------------------------------------------------------------

func runEvents(args []string) error {
	fs := flag.NewFlagSet("kafka events", flag.ExitOnError)
	brokers := fs.String("brokers", "localhost:9092", "comma-separated Kafka brokers")
	since := fs.String("since-mark", "", "a file written by 'agentctl kafka mark'; only messages produced after it are printed (default: everything)")
	topicRe := fs.String("topic-regex", "", "only read topics matching this regexp (default: every non-internal topic)")
	name := fs.String("name", "", "only print events with this name")
	aggregateID := fs.String("aggregate-id", "", "only print events with this aggregate id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	start := offsetMark{}
	if *since != "" {
		b, err := os.ReadFile(*since)
		if err != nil {
			return fmt.Errorf("read mark: %w", err)
		}
		if err := json.Unmarshal(b, &start); err != nil {
			return fmt.Errorf("parse mark %s: %w", *since, err)
		}
	}
	var re *regexp.Regexp
	if *topicRe != "" {
		var err error
		if re, err = regexp.Compile(*topicRe); err != nil {
			return fmt.Errorf("-topic-regex: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bs := splitCSV(*brokers)
	parts, err := listPartitions(ctx, bs)
	if err != nil {
		return err
	}
	codec := kafka.JSONCodec{}
	for _, p := range parts {
		if re != nil && !re.MatchString(p.Topic) {
			continue
		}
		// A partition missing from the mark was created after it, so every
		// message on it is new.
		from := start[markKey(p.Topic, p.ID)]
		if err := readRange(ctx, bs, p.Topic, p.ID, from, func(msg kafkago.Message) error {
			ev, err := codec.Decode(msg.Value)
			if err != nil {
				return printRaw(p.Topic, msg, err)
			}
			if *name != "" && ev.Name != *name {
				return nil
			}
			if *aggregateID != "" && ev.AggregateID != *aggregateID {
				return nil
			}
			return printEvent(os.Stdout, p.Topic, msg, ev)
		}); err != nil {
			return err
		}
	}
	return nil
}

// readRange calls fn for every message in [from, end of partition) as of now.
func readRange(ctx context.Context, brokers []string, topic string, partition int, from int64, fn func(kafkago.Message) error) error {
	first, end, err := partitionBounds(ctx, brokers, topic, partition)
	if err != nil {
		return err
	}
	if from < first {
		from = first
	}
	if from >= end {
		return nil
	}

	conn, err := kafkago.DialLeader(ctx, "tcp", brokers[0], topic, partition)
	if err != nil {
		return fmt.Errorf("dial %s/%d: %w", topic, partition, err)
	}
	defer conn.Close()
	if _, err := conn.Seek(from, kafkago.SeekAbsolute); err != nil {
		return fmt.Errorf("seek %s/%d to %d: %w", topic, partition, from, err)
	}
	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		msg, err := conn.ReadMessage(10e6)
		if err != nil {
			return fmt.Errorf("read %s/%d: %w", topic, partition, err)
		}
		if err := fn(msg); err != nil {
			return err
		}
		if msg.Offset+1 >= end {
			return nil
		}
	}
}

// listPartitions returns every partition of every non-internal topic, sorted
// for stable output.
func listPartitions(ctx context.Context, brokers []string) ([]kafkago.Partition, error) {
	if len(brokers) == 0 {
		return nil, errors.New("no brokers given")
	}
	conn, err := (&kafkago.Dialer{}).DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", brokers[0], err)
	}
	defer conn.Close()
	all, err := conn.ReadPartitions()
	if err != nil {
		return nil, fmt.Errorf("list partitions: %w", err)
	}
	out := all[:0]
	for _, p := range all {
		if strings.HasPrefix(p.Topic, "_") {
			continue // __consumer_offsets, _schemas, ...
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Topic != out[j].Topic {
			return out[i].Topic < out[j].Topic
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func partitionBounds(ctx context.Context, brokers []string, topic string, partition int) (first, end int64, err error) {
	conn, err := kafkago.DialLeader(ctx, "tcp", brokers[0], topic, partition)
	if err != nil {
		return 0, 0, fmt.Errorf("dial %s/%d: %w", topic, partition, err)
	}
	defer conn.Close()
	first, end, err = conn.ReadOffsets()
	if err != nil {
		return 0, 0, fmt.Errorf("offsets %s/%d: %w", topic, partition, err)
	}
	return first, end, nil
}

// printRaw reports a message the event codec can't decode, instead of
// skipping it: on a tap, a poison message is exactly what you want to see.
func printRaw(topic string, msg kafkago.Message, decodeErr error) error {
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"topic":        topic,
		"partition":    msg.Partition,
		"offset":       msg.Offset,
		"decode_error": decodeErr.Error(),
		"raw":          string(msg.Value),
		"headers":      headerMap(msg.Headers),
	})
}

func headerMap(hs []kafkago.Header) map[string]string {
	if len(hs) == 0 {
		return nil
	}
	m := make(map[string]string, len(hs))
	for _, h := range hs {
		m[h.Key] = string(h.Value)
	}
	return m
}
