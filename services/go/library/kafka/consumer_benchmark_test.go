//go:build integration

package kafka_test

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/kafka"
	"go-services/library/require"
)

func benchSetup(b *testing.B, n int) (topic string, cleanup func()) {
	b.Helper()

	topic = fmt.Sprintf("bench-%d", time.Now().UnixNano())
	err := testKafka.CreateTopic(context.Background(), topic)
	require.NoError(b, err, "create topic")

	producer, err := kgo.NewClient(
		kgo.SeedBrokers(testKafka.PlainBrokers...),
		kgo.DefaultProduceTopic(topic),
	)
	require.NoError(b, err, "create producer")

	ctx := context.Background()
	for i := range n {
		producer.Produce(ctx, &kgo.Record{
			Topic: topic,
			Value: fmt.Appendf(nil, "msg-%d", i),
		}, nil)
	}
	for producer.BufferedProduceRecords() > 0 {
		err = producer.Flush(ctx)
		require.NoError(b, err, "flush")
	}
	producer.Close()

	// Warm broker fetch path and group coordinator to eliminate first-run
	// variance caused by cold container state.
	warmer, err := kgo.NewClient(
		kgo.SeedBrokers(testKafka.PlainBrokers...),
		kgo.ConsumerGroup(fmt.Sprintf("warmup-%d", time.Now().UnixNano())),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		b.Fatalf("warmup client: %v", err)
	}
	warmCtx, warmCancel := context.WithTimeout(ctx, 10*time.Second)
	warmer.PollRecords(warmCtx, 500)
	warmer.Close()
	warmCancel()

	return topic, func() {} // topics cleaned up by container teardown
}

type memSnapshot struct {
	totalAlloc uint64
	numGC      uint32
}

func takeMemSnapshot() memSnapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return memSnapshot{ms.TotalAlloc, ms.NumGC}
}

func (m memSnapshot) reportDelta(b *testing.B, end memSnapshot, nRecords int) {
	b.ReportMetric(float64(end.totalAlloc-m.totalAlloc)/float64(nRecords), "B/record")
	b.ReportMetric(float64(end.numGC-m.numGC), "gc-cycles")
}

// BenchmarkRawKgo measures raw kgo.Client throughput (no Consumer wrapper).
func BenchmarkRawKgo(b *testing.B) {
	const nRecords = 1_000_000
	topic, cleanup := benchSetup(b, nRecords)
	defer cleanup()

	for b.Loop() {
		var processed atomic.Int64

		client, err := kgo.NewClient(
			kgo.SeedBrokers(testKafka.PlainBrokers...),
			kgo.ConsumerGroup(fmt.Sprintf("raw-%d", time.Now().UnixNano())),
			kgo.ConsumeTopics(topic),
			kgo.DisableAutoCommit(),
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		)
		require.NoError(b, err, "new kgo client")

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

		memBefore := takeMemSnapshot()
		b.ResetTimer()
		start := time.Now()

		for processed.Load() < nRecords {
			fetches := client.PollRecords(ctx, -1)
			if err := fetches.Err(); err != nil {
				if ctx.Err() != nil {
					break
				}
				b.Fatalf("poll error: %v", err)
			}
			for _, r := range fetches.Records() {
				_ = r.Value
				processed.Add(1)
			}
		}

		elapsed := time.Since(start)
		memAfter := takeMemSnapshot()

		b.ReportMetric(float64(nRecords)/elapsed.Seconds(), "records/s")
		b.ReportMetric(float64(elapsed.Microseconds())/float64(nRecords), "µs/record")
		memBefore.reportDelta(b, memAfter, nRecords)

		cancel()
		client.Close()
	}
}

// BenchmarkConsumer measures our Consumer wrapper throughput.
func BenchmarkConsumer(b *testing.B) {
	const nRecords = 1_000_000
	topic, cleanup := benchSetup(b, nRecords)
	defer cleanup()

	for b.Loop() {
		var processed atomic.Int64

		client, err := kafka.New(
			testKafka.PlainBrokers,
			fmt.Sprintf("consumer-%d", time.Now().UnixNano()),
			kafka.WithWorkers(8),
			kafka.WithFetchMaxRecords(-1),
		)
		require.NoError(b, err, "new consumer")

		err = client.Consumer.AddTopic(topic, func(_ context.Context, r *kgo.Record) error {
			_ = r.Value
			processed.Add(1)
			return nil
		})
		require.NoError(b, err, "add topic")

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

		memBefore := takeMemSnapshot()
		b.ResetTimer()
		start := time.Now()

		go func() {
			if err := client.Consumer.Run(ctx); err != nil && ctx.Err() == nil {
				b.Errorf("consumer run: %v", err)
			}
		}()

		for processed.Load() < nRecords && ctx.Err() == nil {
			time.Sleep(10 * time.Millisecond)
		}

		elapsed := time.Since(start)
		memAfter := takeMemSnapshot()

		b.ReportMetric(float64(nRecords)/elapsed.Seconds(), "records/s")
		b.ReportMetric(float64(elapsed.Microseconds())/float64(nRecords), "µs/record")
		memBefore.reportDelta(b, memAfter, nRecords)

		cancel()
		client.Close()
	}
}
