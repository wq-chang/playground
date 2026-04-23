package kafka

// AuthMechanism represents the SASL authentication mechanism.
type AuthMechanism string

const (
	// AuthMechanismPlain uses SASL/PLAIN authentication.
	AuthMechanismPlain AuthMechanism = "PLAIN"
	// AuthMechanismScram256 uses SASL/SCRAM-SHA-256 authentication.
	AuthMechanismScram256 AuthMechanism = "SCRAM-SHA-256"
	// AuthMechanismScram512 uses SASL/SCRAM-SHA-512 authentication.
	AuthMechanismScram512 AuthMechanism = "SCRAM-SHA-512"
)
