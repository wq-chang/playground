package kafka

// AckMode determines how the consumer acknowledges records.
type AckMode int

const (
	// AckModeAtLeastOnce ensures records are processed at least once.
	// Records are committed after the handler finishes successfully.
	// If the handler fails, the record might be processed again.
	AckModeAtLeastOnce AckMode = iota

	// AckModeAtMostOnce ensures records are processed at most once.
	// Records are committed before the handler is called.
	// If the handler fails, the record might be lost.
	AckModeAtMostOnce
)
