package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	Brokers          = "kafka:9092"
	ConsumerGroup    = "topic-relay-worker"
	SourceTopic      = "new_sct.default"
	DestinationTopic = "sct.cleared"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client, err := kgo.NewClient(
		kgo.SeedBrokers(Brokers),
		kgo.ConsumerGroup(ConsumerGroup),
		kgo.ConsumeTopics(SourceTopic),
		kgo.AutoCommitMarks(),
	)
	if err != nil {
		log.Fatalf("unable to create kafka client: %v", err)
	}
	defer client.Close()

	log.Printf("Relaying messages from '%s' to '%s'...", SourceTopic, DestinationTopic)

	for {
		fetches := client.PollFetches(ctx)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			break
		}

		fetches.EachError(func(t string, p int32, err error) {
			log.Printf("[ERR] fetch error on topic %s: %v", t, err)
		})

		fetches.EachRecord(func(record *kgo.Record) {
			outRecord := &kgo.Record{
				Topic: DestinationTopic,
				Key:   record.Key,
				Value: record.Value,
			}

			client.Produce(ctx, outRecord, func(r *kgo.Record, err error) {
				if err != nil {
					log.Printf("[ERR] failed to produce message key %s: %v", string(record.Key), err)
					// In production, you would exit the process or route to a DLQ here
					return
				}

				client.MarkCommitRecords(record)
			})
		})
	}

	log.Println("Shutdown signal received. Flushing pending records...")
	if err := client.Flush(context.Background()); err != nil {
		log.Printf("[ERR] flush failed during shutdown: %v", err)
	}
	log.Println("Exited cleanly")
}
