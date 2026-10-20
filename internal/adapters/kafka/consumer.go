package kafka

import (
	"context"
	"log"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

const KAFKA_URL = "kafka:9092"

type Handler interface {
	Handle(ctx context.Context, record *kgo.Record) error
}

func RunKafkaConsumer(ctx context.Context, topic string, consumerGroupId string, handler Handler, maxConcurrent int) {
	sem := make(chan struct{}, maxConcurrent)
	kafkaClient, err := kgo.NewClient(
		kgo.SeedBrokers(KAFKA_URL),

		kgo.ConsumerGroup(consumerGroupId),
		kgo.ConsumeTopics(topic),
		//kgo.DisableAutoCommit(),
		kgo.AutoCommitMarks(),
	)
	if err != nil {
		log.Fatalf("Unable to connect to Kafka: %v", err)
	}
	defer kafkaClient.Close()

	var wg sync.WaitGroup

	for {
		fetches := kafkaClient.PollFetches(ctx)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			log.Printf("[INFO] Kafka consumer (topic '%s') graceful shutdown - ok", topic)
			break
		}

		fetches.EachError(func(t string, p int32, err error) {
			log.Printf("[ERR] Fetch error topic %s: %v", t, err)
		})

		fetches.EachRecord(func(rec *kgo.Record) {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:

			}

			wg.Go(func() {
				defer func() { <-sem }()

				err := handler.Handle(context.Background(), rec)

				if err != nil {
					log.Printf("[ERR] Kafka consumer (topic '%s') record handling failed: %v", topic, err)
					// produce to retry topic/dead letter queue in production
					// If produce succeeds, we consider this record "handled".
				}
				kafkaClient.MarkCommitRecords(rec)
			})
		})
	}

	wg.Wait()
}
