package testenv

import (
	"context"
	"fmt"
	"sync"
)

type serviceEntry struct {
	service   any
	err       error
	cleanup   func()
	ready     chan struct{}
	configKey string
}

// TestEnv manages the lifecycle of infrastructure dependencies for a specific
// test package. Services are cached by service kind so repeated setup calls can
// reuse the same initialized dependency.
//
//nolint:govet // Keeping packageName and the shared service cache on TestEnv keeps setup orchestration straightforward.
type TestEnv struct {
	packageName string
	services    map[string]*serviceEntry
	mu          sync.Mutex
}

// New creates a new environment manager for the given package. The package name
// is used to scope package-specific resources such as PostgreSQL schemas.
func New(packageName string) *TestEnv {
	return &TestEnv{
		packageName: packageName,
		services:    make(map[string]*serviceEntry),
		mu:          sync.Mutex{},
	}
}

func (te *TestEnv) setupService(
	serviceName,
	configKey string,
	setup func(context.Context) (any, func(), error),
) (any, error) {
	te.mu.Lock()
	if entry, ok := te.services[serviceName]; ok {
		if entry.configKey != configKey {
			te.mu.Unlock()
			return nil, fmt.Errorf("%s already initialized with different options", serviceName)
		}

		ready := entry.ready
		te.mu.Unlock()

		<-ready
		if entry.err != nil {
			return nil, entry.err
		}

		return entry.service, nil
	}

	entry := &serviceEntry{
		service:   nil,
		err:       nil,
		cleanup:   nil,
		ready:     make(chan struct{}),
		configKey: configKey,
	}
	te.services[serviceName] = entry
	te.mu.Unlock()

	service, cleanup, err := setup(context.Background())

	te.mu.Lock()
	entry.service = service
	entry.cleanup = cleanup
	entry.err = err
	close(entry.ready)
	if err != nil {
		delete(te.services, serviceName)
	}
	te.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return service, nil
}

func (te *TestEnv) takeServices() []*serviceEntry {
	te.mu.Lock()
	defer te.mu.Unlock()

	entries := make([]*serviceEntry, 0, len(te.services))
	for _, entry := range te.services {
		entries = append(entries, entry)
	}
	te.services = make(map[string]*serviceEntry)

	return entries
}

// Cleanup performs teardown of all initialized services.
func (te *TestEnv) Cleanup() {
	for _, entry := range te.takeServices() {
		<-entry.ready
		if entry.cleanup != nil {
			entry.cleanup()
		}
	}
}
