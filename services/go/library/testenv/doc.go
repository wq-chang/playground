// Package testenv provides utilities for managing ephemeral and reusable integration test environments.
//
// It simplifies the setup and teardown of infrastructure dependencies—such as databases,
// message queues, and cloud emulators—ensuring that tests run in isolated, reproducible contexts.
//
// # Goals
//
//   - **Speed**: leveraging reusable containers (via Testcontainers) to minimize startup overhead across test suites.
//   - **Isolation**: providing scoped resources (e.g., unique database schemas) to prevent state leakage between tests.
//   - **Simplicity**: offering a clean API to provision complex infrastructure with minimal boilerplate.
//
// # Supported Services
//
//   - **PostgreSQL**: Managed via `SetupPostgres`. Supports schema-level isolation for parallel test execution.
//   - **Kafka**: Managed via `SetupKafka`.
//
// # Usage
//
// To use testenv, keep container-backed tests in integration-tagged test files
// and provision the infrastructure from an integration-only TestMain. This
// keeps ordinary unit-test runs container-free while still allowing explicit
// integration runs to share infrastructure.
//
//	var (
//		te *testenv.TestEnv
//		pg *testenv.Postgres
//	)
//
//	//go:build integration
//
//	func TestMain(m *testing.M) {
//		// Initialize the environment with a unique name (used as a DB schema)
//		te = testenv.New("my_package")
//
//		var err error
//		pg, err = testenv.SetupPostgres(te)
//		if err != nil {
//			fmt.Fprintf(os.Stderr, "failed to set up postgres: %v\n", err)
//			os.Exit(1)
//		}
//
//		code := m.Run()
//
//		// Teardown all initialized services
//		te.Cleanup()
//
//		os.Exit(code)
//	}
//
//	func TestMyFeature(t *testing.T) {
//		pool := pg.Pool
//
//		// Run tests using 'pool'...
//	}
//
// Run these tests explicitly with `go test -tags=integration ./...`
//
// # Environment Variables
//
//   - `KEEP_TEST_DB`: If set to "true", the created PostgreSQL schema will NOT be dropped after the test cleanup function is called.
//     This allows developers to manually inspect the database state for debugging purposes.
package testenv
