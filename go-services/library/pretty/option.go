package pretty

import "strings"

type config struct {
	sensitiveFields map[string]struct{}
	maxDepth        int
	maxSliceItems   int
	maxMapItems     int
	maxStructFields int
	maxBytes        int
}

func newConfig(opts ...Option) *config {
	c := &config{
		maxDepth:        4,
		maxSliceItems:   10,
		maxMapItems:     10,
		maxStructFields: 20,
		maxBytes:        512,
		sensitiveFields: make(map[string]struct{}),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// NewConfig creates a reusable configuration object
func NewConfig(opts ...Option) *config {
	return newConfig(opts...)
}

// Option is a function that configures our settings
type Option func(*config)

// Setting functions that the user can call
func WithMaxDepth(d int) Option          { return func(c *config) { c.maxDepth = d } }
func WithSliceLimit(l int) Option        { return func(c *config) { c.maxSliceItems = l } }
func WithMapLimit(l int) Option          { return func(c *config) { c.maxMapItems = l } }
func WithBytesLimit(l int) Option        { return func(c *config) { c.maxBytes = l } }
func WithStructFieldsLimit(l int) Option { return func(c *config) { c.maxStructFields = l } }

func WithSensitiveFields(fields ...string) Option {
	return func(c *config) {
		for _, f := range fields {
			c.sensitiveFields[strings.ToLower(f)] = struct{}{}
		}
	}
}
