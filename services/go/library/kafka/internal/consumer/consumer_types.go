// services/go/library/kafka/internal/consumer/consumer_types.go
package consumer

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
)

// PartitionBatch groups all polled records for one topic-partition.
type PartitionBatch struct {
	Key          Key
	Records      []*kgo.Record
	Subscription Subscription
}

// FetchControlClient is the narrow interface Dispatcher needs for backpressure
// pause/resume on the Kafka client.
type FetchControlClient interface {
	PauseFetchPartitions(partitions map[string][]int32)
	ResumeFetchPartitions(partitions map[string][]int32)
}

// WorkerClient is the narrow interface WorkerRunner needs for commit, pause,
// and resume operations on the Kafka client.
type WorkerClient interface {
	CommitOffsetsSync(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error
	PauseFetchTopics(topics ...string)
	ResumeFetchPartitions(partitions map[string][]int32)
}
