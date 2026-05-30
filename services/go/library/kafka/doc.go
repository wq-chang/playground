// Package kafka provides a thin application-facing wrapper around franz-go for
// Kafka producers and consumers.
//
// The package centers on New, which creates a shared Client exposing both
// Consumer and Producer capabilities on top of a single franz-go client. That
// shared client owns the broker connections, topic subscriptions, and manual
// offset commit coordination; calling Client.Close shuts down both sides.
//
// # Consuming
//
// Register topic handlers up front with WithSubscription or WithTopic, or add
// them later with Consumer.AddSubscription or Consumer.AddTopic. Consumer.Run
// polls records, routes them by topic, and processes each topic-partition
// sequentially while still allowing different partitions to run concurrently.
// WithWorkers sets the global record-processing concurrency limit across those
// per-partition runners.
//
// Delivery guarantees are controlled per subscription:
//
//   - AckModeAtLeastOnce commits after a record is handled successfully, which
//     can replay a record after failures but avoids acknowledging work that did
//     not finish.
//   - AckModeAtMostOnce commits before the handler runs, which avoids replay at
//     the cost of potentially losing a record if processing fails afterward.
//
// # Failure handling
//
// Subscription.FailurePolicy controls retries and what happens after retries
// are exhausted. A subscription can pause the topic and leave the failed record
// uncommitted, commit past the record, or publish it to a dead-letter topic and
// commit only after the dead-letter publish succeeds.
//
// # Producing
//
// Use Client.Producer to send records asynchronously with Produce or
// synchronously with ProduceSync. Producer and consumer capabilities share the
// same underlying Kafka client, so applications only need one package-level
// setup path for authentication, broker discovery, and franz-go options.
//
// # Example
//
//	client, err := kafka.New(
//		[]string{"localhost:9092"},
//		"user-events",
//		kafka.WithTopic("users.created", handleUserCreated),
//		kafka.WithWorkers(32),
//	)
//	if err != nil {
//		return err
//	}
//	defer client.Close()
//
//	go func() {
//		_ = client.Consumer.Run(ctx)
//	}()
//
//	return client.Producer.ProduceSync(ctx, &kgo.Record{
//		Topic: "users.created",
//		Key:   []byte("123"),
//		Value: []byte(`{"id":"123"}`),
//	})
package kafka
