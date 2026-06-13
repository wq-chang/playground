package kafka

import "go-services/library/kafka/ktype"

// AckMode determines how the consumer acknowledges records.
type AckMode = ktype.AckMode

const (
	// AckModeAtLeastOnce ensures records are processed at least once.
	AckModeAtLeastOnce = ktype.AckModeAtLeastOnce

	// AckModeAtMostOnce ensures records are processed at most once.
	AckModeAtMostOnce = ktype.AckModeAtMostOnce
)
