//go:build integration

package kafka_test

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-services/library/assert"
	"go-services/library/kafka"
	"go-services/library/require"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestKafkaProducerConsumer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	topic := fmt.Sprintf("test-topic-%d", time.Now().UnixNano())
	err := testKafka.CreateTopic(ctx, topic)
	require.NoError(t, err, "failed to create test topic")

	message := "hello kafka"

	tests := map[string]struct {
		brokers []string
		auth    bool
	}{
		"no-auth": {testKafka.PlainBrokers, false},
		"auth":    {testKafka.AuthBrokers, true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			consumedChan := make(chan string, 1)
			handler := func(ctx context.Context, record *kgo.Record) error {
				consumedChan <- string(record.Value)
				return nil
			}

			opts := []kafka.Option{
				kafka.WithSubscription(kafka.Subscription{
					Topic:         topic,
					Handler:       handler,
					BatchHandler:  nil,
					AckMode:       kafka.AckModeAtLeastOnce,
					FailurePolicy: kafka.FailurePolicy{},
				}),
				kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
			}
			if tt.auth {
				opts = append(opts, kafka.WithAuth(testKafka.Username, testKafka.Password, kafka.AuthMechanismScram512))
			}

			client, err := kafka.New(tt.brokers, "test-group-"+name, opts...)
			require.NoError(t, err, "failed to create kafka client")
			defer client.Close()

			err = client.Producer.ProduceSync(ctx, &kgo.Record{
				Topic: topic,
				Value: []byte(message),
			})
			require.NoError(t, err, "failed to produce message")

			go func() {
				if err := client.Consumer.Run(ctx); err != nil {
					if ctx.Err() == nil {
						t.Errorf("consumer run failed: %v", err)
					}
				}
			}()

			select {
			case val := <-consumedChan:
				assert.Equal(t, val, message, "kafka message")
			case <-ctx.Done():
				t.Fatal("timed out waiting for message")
			}
		})
	}
}

func TestKafkaConsumerAddSubscription(t *testing.T) {
	t.Run("after construction before run", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		topic := fmt.Sprintf("test-add-topic-before-run-%d", time.Now().UnixNano())
		err := testKafka.CreateTopic(ctx, topic)
		require.NoError(t, err, "failed to create test topic")

		consumedChan := make(chan string, 1)
		handler := func(ctx context.Context, record *kgo.Record) error {
			consumedChan <- string(record.Value)
			return nil
		}

		client, err := kafka.New(
			testKafka.PlainBrokers,
			fmt.Sprintf("test-group-before-run-%d", time.Now().UnixNano()),
			kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
		)
		require.NoError(t, err, "failed to create kafka client")
		defer client.Close()

		err = client.Consumer.AddSubscription(kafka.Subscription{
			Topic:         topic,
			Handler:       handler,
			BatchHandler:  nil,
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		})
		require.NoError(t, err, "failed to add topic before run")

		go func() {
			if runErr := client.Consumer.Run(ctx); runErr != nil && ctx.Err() == nil {
				t.Errorf("consumer run failed: %v", runErr)
			}
		}()

		err = client.Producer.ProduceSync(ctx, &kgo.Record{
			Topic: topic,
			Value: []byte("before-run"),
		})
		require.NoError(t, err, "failed to produce message")

		select {
		case val := <-consumedChan:
			assert.Equal(t, val, "before-run", "kafka message")
		case <-ctx.Done():
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("while run is active", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		topic := fmt.Sprintf("test-add-topic-during-run-%d", time.Now().UnixNano())
		err := testKafka.CreateTopic(ctx, topic)
		require.NoError(t, err, "failed to create test topic")

		consumedChan := make(chan string, 1)
		handler := func(ctx context.Context, record *kgo.Record) error {
			consumedChan <- string(record.Value)
			return nil
		}

		client, err := kafka.New(
			testKafka.PlainBrokers,
			fmt.Sprintf("test-group-during-run-%d", time.Now().UnixNano()),
			kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
		)
		require.NoError(t, err, "failed to create kafka client")
		defer client.Close()

		go func() {
			if runErr := client.Consumer.Run(ctx); runErr != nil && ctx.Err() == nil {
				t.Errorf("consumer run failed: %v", runErr)
			}
		}()

		time.Sleep(500 * time.Millisecond)

		err = client.Consumer.AddSubscription(kafka.Subscription{
			Topic:         topic,
			Handler:       handler,
			BatchHandler:  nil,
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		})
		require.NoError(t, err, "failed to add topic during run")

		err = client.Producer.ProduceSync(ctx, &kgo.Record{
			Topic: topic,
			Value: []byte("during-run"),
		})
		require.NoError(t, err, "failed to produce message")

		select {
		case val := <-consumedChan:
			assert.Equal(t, val, "during-run", "kafka message")
		case <-ctx.Done():
			t.Fatal("timed out waiting for message")
		}
	})
}

func TestKafkaConsumerDLQOnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mainTopic := fmt.Sprintf("test-dlq-main-%d", time.Now().UnixNano())
	dlqTopic := fmt.Sprintf("test-dlq-topic-%d", time.Now().UnixNano())

	require.NoError(t, testKafka.CreateTopic(ctx, mainTopic), "failed to create main topic")
	require.NoError(t, testKafka.CreateTopic(ctx, dlqTopic), "failed to create dlq topic")

	dlqRecords := make(chan *kgo.Record, 1)
	client, err := kafka.New(
		testKafka.PlainBrokers,
		fmt.Sprintf("test-group-dlq-%d", time.Now().UnixNano()),
		kafka.WithSubscription(kafka.Subscription{
			Topic: mainTopic,
			Handler: func(context.Context, *kgo.Record) error {
				return fmt.Errorf("boom")
			},
			BatchHandler: nil,
			AckMode:      kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{
				MaxAttempts:  0,
				RetryBackoff: 0,
				DLQ:          &kafka.DLQConfig{Topic: dlqTopic},
				OnExhausted:  kafka.ExhaustedActionUnspecified,
			},
		}),
		kafka.WithSubscription(kafka.Subscription{
			Topic: dlqTopic,
			Handler: func(ctx context.Context, record *kgo.Record) error {
				dlqRecords <- record
				return nil
			},
			BatchHandler:  nil,
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		}),
		kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
	)
	require.NoError(t, err, "failed to create kafka client")
	defer client.Close()

	runErrs := make(chan error, 1)
	go func() {
		runErrs <- client.Consumer.Run(ctx)
	}()

	require.NoError(t, client.Producer.ProduceSync(ctx, &kgo.Record{
		Topic: mainTopic,
		Value: []byte("failed-message"),
	}), "failed to produce main topic message")

	select {
	case record := <-dlqRecords:
		assert.Equal(t, string(record.Value), "failed-message", "dlq payload")
		headerValue, ok := headerValue(record.Headers, "dlq-original-topic")
		assert.True(t, ok, "dlq metadata header should exist")
		assert.Equal(t, headerValue, mainTopic, "dlq original topic")
	case err := <-runErrs:
		if err != nil && ctx.Err() == nil {
			t.Fatalf("consumer should continue after dlq publish: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for dlq message")
	}
}

func headerValue(headers []kgo.RecordHeader, key string) (string, bool) {
	for _, header := range headers {
		if header.Key == key {
			return string(header.Value), true
		}
	}
	return "", false
}

func TestKafkaConsumerStopPausesTopicOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	failingTopic := fmt.Sprintf("test-stop-topic-%d", time.Now().UnixNano())
	healthyTopic := fmt.Sprintf("test-healthy-topic-%d", time.Now().UnixNano())
	require.NoError(t, testKafka.CreateTopic(ctx, failingTopic), "failed to create failing topic")
	require.NoError(t, testKafka.CreateTopic(ctx, healthyTopic), "failed to create healthy topic")

	var failingCalls atomic.Int32
	failingSeen := make(chan struct{}, 1)
	healthyValues := make(chan string, 2)

	client, err := kafka.New(
		testKafka.PlainBrokers,
		fmt.Sprintf("test-group-stop-%d", time.Now().UnixNano()),
		kafka.WithSubscription(kafka.Subscription{
			Topic: failingTopic,
			Handler: func(context.Context, *kgo.Record) error {
				if failingCalls.Add(1) == 1 {
					failingSeen <- struct{}{}
				}
				return fmt.Errorf("boom")
			},
			BatchHandler:  nil,
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		}),
		kafka.WithSubscription(kafka.Subscription{
			Topic: healthyTopic,
			Handler: func(ctx context.Context, record *kgo.Record) error {
				healthyValues <- string(record.Value)
				return nil
			},
			BatchHandler:  nil,
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		}),
		kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
	)
	require.NoError(t, err, "failed to create kafka client")
	defer client.Close()

	runErrs := make(chan error, 1)
	go func() {
		runErrs <- client.Consumer.Run(ctx)
	}()

	require.NoError(t, client.Producer.ProduceSync(ctx, &kgo.Record{
		Topic: failingTopic,
		Value: []byte("failed-message-1"),
	}), "failed to produce main topic message")

	select {
	case <-failingSeen:
	case <-ctx.Done():
		t.Fatal("timed out waiting for failing topic to be handled")
	}

	require.NoError(t, client.Producer.ProduceSync(ctx, &kgo.Record{
		Topic: healthyTopic,
		Value: []byte("healthy-message"),
	}), "failed to produce healthy topic message")
	require.NoError(t, client.Producer.ProduceSync(ctx, &kgo.Record{
		Topic: failingTopic,
		Value: []byte("failed-message-2"),
	}), "failed to produce second failing topic message")

	select {
	case value := <-healthyValues:
		assert.Equal(t, value, "healthy-message", "healthy topic should continue consuming")
	case err := <-runErrs:
		if err != nil && ctx.Err() == nil {
			t.Fatalf("consumer should keep running after pausing one topic: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for healthy topic message")
	}

	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, failingCalls.Load(), 1, "failing topic should be paused after the first exhausted failure")

	select {
	case err := <-runErrs:
		if err != nil && ctx.Err() == nil {
			t.Fatalf("consumer should keep running after pausing one topic: %v", err)
		}
	default:
	}
}

// partitionTracker records the highest message sequence observed per partition.
type partitionTracker struct {
	last map[int32]int
	mu   sync.Mutex
}

func newPartitionTracker() *partitionTracker {
	return &partitionTracker{last: make(map[int32]int), mu: sync.Mutex{}}
}

func (t *partitionTracker) record(partition int32, seq int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if seq > t.last[partition] {
		t.last[partition] = seq
	}
}

func (t *partitionTracker) lastOf(partition int32) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.last[partition]
}

// maxOf returns the highest sequence seen on any partition, or -1 if none.
func (t *partitionTracker) maxOf() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	max := -1
	for _, s := range t.last {
		if s > max {
			max = s
		}
	}
	return max
}

// rebalanceMember is one consumer in the shared consumer group.
type rebalanceMember struct {
	client *kafka.Client
	runErr chan error
	seen   *partitionTracker
	label  string
}

// startRebalanceMember starts a member consuming topic in group. It records
// every delivered message's sequence into its own tracker.
func startRebalanceMember(t *testing.T, group, topic, label string) *rebalanceMember {
	t.Helper()

	seen := newPartitionTracker()
	client, err := kafka.New(
		testKafka.PlainBrokers,
		group,
		kafka.WithSubscription(kafka.Subscription{
			Topic: topic,
			Handler: func(_ context.Context, record *kgo.Record) error {
				seq, err := strconv.Atoi(string(record.Value))
				if err != nil {
					return fmt.Errorf("parse seq %q: %w", record.Value, err)
				}
				seen.record(record.Partition, seq)
				return nil
			},
			BatchHandler:  nil,
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		}),
		kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
	)
	require.NoError(t, err, "failed to create rebalance member client")
	t.Cleanup(client.Close)

	m := &rebalanceMember{label: label, client: client, runErr: make(chan error, 1), seen: seen}
	go func() { m.runErr <- client.Consumer.Run(context.Background()) }()
	return m
}

// combinedProgress returns the highest sequence any of the members observed
// for partition, i.e. 0 if none of them have consumed that partition at all.
func combinedProgress(members []*rebalanceMember, partition int32) int {
	max := 0
	for _, m := range members {
		if v := m.seen.lastOf(partition); v > max {
			max = v
		}
	}
	return max
}

// waitForPartitionProgress polls until every partition in expected has been
// consumed up to the expected sequence by some member of the group.
func waitForPartitionProgress(
	t *testing.T,
	members []*rebalanceMember,
	expected map[int32]int,
	timeout time.Duration,
	phase string,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	stuck := func() []int32 {
		var result []int32
		for p, want := range expected {
			if combinedProgress(members, p) < want {
				result = append(result, p)
			}
		}
		slices.Sort(result)
		return result
	}

	for {
		if s := stuck(); len(s) == 0 {
			return
		} else if time.Now().After(deadline) {
			parts := make([]int32, 0, len(expected))
			for p := range expected {
				parts = append(parts, p)
			}
			slices.Sort(parts)
			var got []string
			var perMember []string
			for _, p := range parts {
				got = append(got, fmt.Sprintf("p%d=%d/%d", p, combinedProgress(members, p), expected[p]))
			}
			for _, m := range members {
				var mparts []string
				for _, p := range parts {
					mparts = append(mparts, fmt.Sprintf("p%d=%d", p, m.seen.lastOf(p)))
				}
				perMember = append(perMember, fmt.Sprintf("%s[%s]", m.label, strings.Join(mparts, " ")))
			}
			t.Fatalf("%s: partitions stuck after %s: %v (combined %s; per member %s)",
				phase, timeout, s, strings.Join(got, ", "), strings.Join(perMember, " "))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// assertRebalanceMemberRunning fails if the member's Run ended before the
// assertion.
func assertRebalanceMemberRunning(t *testing.T, m *rebalanceMember, phase string) {
	t.Helper()
	select {
	case err := <-m.runErr:
		t.Fatalf("%s: member %s run ended: %v", phase, m.label, err)
	default:
	}
}

// waitForMemberProgress waits until the member has consumed at least one
// message, proving it received a partition assignment.
func waitForMemberProgress(t *testing.T, m *rebalanceMember, timeout time.Duration, phase string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for m.seen.maxOf() < 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%s: member %s consumed nothing within %s", phase, m.label, timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitForMemberCleanStop expects the member's Run to have returned nil after
// its client was closed (the external-close clean signal).
func waitForMemberCleanStop(t *testing.T, m *rebalanceMember, timeout time.Duration, phase string) {
	t.Helper()
	select {
	case err := <-m.runErr:
		require.NoError(t, err, "%s: member %s should stop cleanly on close", phase, m.label)
	case <-time.After(timeout):
		t.Fatalf("%s: member %s did not stop within %s", phase, m.label, timeout)
	}
}

// TestKafkaConsumerGroupRebalance drives real rebalances (partition revoke and
// reassignment) within one consumer group and verifies delivery keeps flowing
// to every partition. It is the end-to-end regression for revoke handling:
// a stale partition-level pause left by a revocation would strand a partition
// when it is reassigned, which the per-partition progress waits detect.
func TestKafkaConsumerGroupRebalance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	topic := fmt.Sprintf("test-rebalance-%d", time.Now().UnixNano())
	const partitions = 3
	require.NoError(t, testKafka.CreateTopic(ctx, topic, partitions), "failed to create rebalance topic")

	group := fmt.Sprintf("test-group-rebalance-%d", time.Now().UnixNano())

	// franz-go's default partitioner hashes the record key and ignores
	// Record.Partition; ManualPartitioner makes the explicit partition field
	// authoritative so the per-partition sequence expectations are exact.
	producer, err := kafka.New(
		testKafka.PlainBrokers,
		"rebalance-producer-"+group,
		kafka.WithKgoOptions(kgo.RecordPartitioner(kgo.ManualPartitioner())),
	)
	require.NoError(t, err, "failed to create producer client")
	defer producer.Close()

	// produce emits count sequential messages cycling across the partitions and
	// returns the highest sequence produced to each partition.
	produce := func(offset, count int) map[int32]int {
		expected := make(map[int32]int)
		for i := range count {
			seq := offset + i
			partition := int32(i % partitions)
			require.NoError(t, producer.Producer.ProduceSync(ctx, &kgo.Record{
				Topic:     topic,
				Partition: partition,
				Value:     []byte(strconv.Itoa(seq)),
			}), "failed to produce message %d", seq)
			expected[partition] = seq
		}
		return expected
	}

	// Phase 1: two members join and collectively consume all three partitions.
	a := startRebalanceMember(t, group, topic, "a")
	b := startRebalanceMember(t, group, topic, "b")
	initial := produce(0, 30)
	waitForPartitionProgress(t, []*rebalanceMember{a, b}, initial, 30*time.Second, "initial assignment")
	assertRebalanceMemberRunning(t, a, "phase 1")
	assertRebalanceMemberRunning(t, b, "phase 1")

	// Phase 2: steady-state delivery with two members.
	steady := produce(30, 30)
	waitForPartitionProgress(t, []*rebalanceMember{a, b}, steady, 30*time.Second, "two-member steady state")

	// Phase 3: a third member joins. With 3 partitions / 3 members the
	// coordinator must revoke a partition from an existing member and assign
	// it to the newcomer. Cooperative rebalancing assigns the newcomer in a
	// later round, and until it lands the previous owners still consume the
	// moving partition — so production must overlap with the wait, or the
	// newcomer's partition arrives empty. Produce batches until the newcomer
	// has consumed at least one message; once it owns a partition, only it can
	// advance that partition's combined progress.
	c := startRebalanceMember(t, group, topic, "c")
	all := []*rebalanceMember{a, b, c}
	next := 60
	deadline := time.Now().Add(30 * time.Second)
	for ; time.Now().Before(deadline) && c.seen.maxOf() < 0; next += 30 {
		expected := produce(next, 30)
		waitForPartitionProgress(t, all, expected, 20*time.Second, "phase 3 stream")
	}
	waitForMemberProgress(t, c, 5*time.Second, "joined member consumes its reassigned partition")
	assertRebalanceMemberRunning(t, a, "phase 3")
	assertRebalanceMemberRunning(t, b, "phase 3")

	// Phase 4: the third member leaves. Its partition is reassigned back —
	// with the sticky assignor, most likely to the member that originally
	// owned it. Every partition must keep progressing; a stale partition-level
	// pause from the revocation would strand the returning partition, which is
	// the regression this phase detects (fix: resume-on-revoke).
	c.client.Close()
	waitForMemberCleanStop(t, c, 10*time.Second, "third member leaves cleanly")
	afterLeave := produce(next, 30)
	waitForPartitionProgress(t, []*rebalanceMember{a, b}, afterLeave, 30*time.Second, "after third member leaves")

	// Phase 5: closing the remaining members must end their Run cleanly
	// (external-close clean signal).
	a.client.Close()
	b.client.Close()
	waitForMemberCleanStop(t, a, 10*time.Second, "member a close")
	waitForMemberCleanStop(t, b, 10*time.Second, "member b close")
}
