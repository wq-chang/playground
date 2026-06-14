// Package consumer holds internal runtime types and collaborators for the
// kafka package's consumer implementation. It must not import its parent
// go-services/library/kafka package. It uses go-services/library/kafka/ktype
// for shared canonical types.
package consumer

import "go-services/library/kafka/ktype"

// Key is an alias for ktype.Key for convenience within the consumer package.
type Key = ktype.Key

// Subscription is an alias for ktype.Subscription.
type Subscription = ktype.Subscription

// AckMode is an alias for ktype.AckMode.
type AckMode = ktype.AckMode

// ExhaustedAction is an alias for ktype.ExhaustedAction.
type ExhaustedAction = ktype.ExhaustedAction

// DLQConfig is an alias for ktype.DLQConfig.
type DLQConfig = ktype.DLQConfig

// FailurePolicy is an alias for ktype.FailurePolicy.
type FailurePolicy = ktype.FailurePolicy

// PauseInfo is an alias for ktype.PauseInfo.
type PauseInfo = ktype.PauseInfo

// BatchResult is an alias for ktype.BatchResult.
type BatchResult = ktype.BatchResult

// Constants aliased from ktype for convenience within the consumer package.
const (
	AckModeAtLeastOnce = ktype.AckModeAtLeastOnce
	AckModeAtMostOnce  = ktype.AckModeAtMostOnce

	ExhaustedActionStop          = ktype.ExhaustedActionStop
	ExhaustedActionCommit        = ktype.ExhaustedActionCommit
	ExhaustedActionDLQThenCommit = ktype.ExhaustedActionDLQThenCommit
)
