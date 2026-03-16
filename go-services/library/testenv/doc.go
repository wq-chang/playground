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
//   - **PostgreSQL**: Managed via `GetPostgres`. Supports schema-level isolation for parallel test execution.
//
// # Usage
//
// To use testenv, initialize a package-level TestEnv in your TestMain. This allows
// infrastructure to be shared across all tests in the package while maintaining isolation.
//
//	var te *testenv.TestEnv
//
//	func TestMain(m *testing.M) {
//		// Initialize the environment with a unique name (used as a DB schema)
//		te = testenv.New("my_package")
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
//		// Lazily initializes the PG container/schema on first call
//		pg := te.GetPostgres(t)
//		pool := pg.Pool
//
//		// Run tests using 'pool'...
//	}
//
// # Environment Variables
//
//   - `KEEP_TEST_DB`: If set to "true", the created PostgreSQL schema will NOT be dropped after the test cleanup function is called.
//     This allows developers to manually inspect the database state for debugging purposes.
package testenv
