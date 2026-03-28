package testenv

// config holds the internal settings for the test environment.
// It is not exported to encourage the use of Functional Options (NewConfig).
type config struct {
	// postgresImage specifies the Docker image version to be used for the
	// PostgreSQL container. Defaults to "postgres:18.3-trixie".
	postgresImage string
	// migrationTableName specifies the name of the migration version table.
	// If set, this table will be excluded from data cleanup (truncation).
	migrationTableName string
	// kafkaImage specifies the Docker image version to be used for the
	// Kafka container. Defaults to "confluentinc/confluent-local:7.5.0".
	kafkaImage string
}

// newConfig initializes a config struct with sensible defaults and applies
// any provided functional options.
//
// Defaults:
//   - postgresImage: "postgres:18.3-trixie"
//   - kafkaImage: "apache/kafka:4.1.2"
func newConfig(opts ...Option) *config {
	c := &config{
		postgresImage:      "postgres:18.3-trixie",
		migrationTableName: "",
		kafkaImage:         "apache/kafka:4.1.2",
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Option defines a functional configuration type used to modify the behavior
// of the test environment setup.
type Option func(*config)

// WithPostgresImage overrides the default PostgreSQL Docker image.
// Use this to test against specific database versions (e.g., alpine or older releases).
func WithPostgresImage(image string) Option {
	return func(c *config) {
		c.postgresImage = image
	}
}

// WithMigrationTableName specifies the name of the migration version table (e.g., "goose_db_version").
// This table will be excluded from the database cleanup process (truncation).
func WithMigrationTableName(name string) Option {
	return func(c *config) {
		c.migrationTableName = name
	}
}

// WithKafkaImage overrides the default Kafka Docker image.
func WithKafkaImage(image string) Option {
	return func(c *config) {
		c.kafkaImage = image
	}
}
