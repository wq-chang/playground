package transactor

// config holds the internal settings for a Transactor instance.
// It is kept private to ensure that configuration can only be modified
// through the provided Option functions.
type config struct {
	timeoutSeconds int
}

// NewConfig initializes a config struct with default values and
// applies any provided options.
//
// Default values:
//   - timeoutSeconds: 30
func NewConfig(opts ...Option) *config {
	c := &config{
		timeoutSeconds: 30,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Option defines a function signature used to customize a Transactor's configuration.
type Option func(*config)

// WithTimeout sets the maximum duration in seconds that a transaction
// can run before being automatically cancelled.
func WithTimeout(t int) Option { return func(c *config) { c.timeoutSeconds = t } }
