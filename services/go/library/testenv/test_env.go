package testenv

import (
	"context"
	"sync"
	"testing"
)

// TestEnv manages the lifecycle of infrastructure dependencies for a specific test package.
// It leverages lazy initialization to ensure resources are only provisioned when requested.
type TestEnv struct {
	cfg         *config
	postgres    *Postgres
	kafka       *Kafka
	packageName string
	pgOnce      sync.Once
	kafkaOnce   sync.Once
}

// New creates a new environment manager for the given package.
// The packageName is used to scope resources.
func New(packageName string, opts ...Option) *TestEnv {
	cfg := newConfig(opts...)
	return &TestEnv{
		cfg:         cfg,
		postgres:    nil,
		kafka:       nil,
		packageName: packageName,
		pgOnce:      sync.Once{},
		kafkaOnce:   sync.Once{},
	}
}

// GetPostgres returns an initialized PostgreSQL environment.
func (te *TestEnv) GetPostgres(t *testing.T) *Postgres {
	t.Helper()
	te.pgOnce.Do(func() {
		ctx := context.Background()
		var err error
		te.postgres, err = NewPostgres(ctx, te.packageName, te.cfg.postgresImage, te.cfg.migrationTableName)
		if err != nil {
			t.Fatalf("failed to start test db: %v", err)
		}
	})
	return te.postgres
}

// GetKafka returns an initialized Kafka environment.
func (te *TestEnv) GetKafka(t *testing.T) *Kafka {
	t.Helper()
	te.kafkaOnce.Do(func() {
		ctx := context.Background()
		var err error
		te.kafka, err = NewKafka(ctx, te.cfg.kafkaImage)
		if err != nil {
			t.Fatalf("failed to start test kafka: %v", err)
		}
	})
	return te.kafka
}

// Cleanup performs teardown of all initialized services.
func (te *TestEnv) Cleanup() {
	if te.postgres != nil && te.postgres.Cleanup != nil {
		te.postgres.Cleanup()
	}
	if te.kafka != nil && te.kafka.Cleanup != nil {
		te.kafka.Cleanup()
	}
}
