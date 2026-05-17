//go:build integration

package transactor_test

import (
	"fmt"
	"os"
	"testing"

	"go-services/library/testenv"
)

var (
	te *testenv.TestEnv
	pg *testenv.Postgres
)

func TestMain(m *testing.M) {
	te = testenv.New("library_transactor")

	var err error
	pg, err = testenv.SetupPostgres(te)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to set up postgres: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	te.Cleanup()

	os.Exit(code)
}
