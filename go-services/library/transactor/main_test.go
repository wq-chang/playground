package transactor_test

import (
	"os"
	"testing"

	"go-services/library/testenv"
)

var te *testenv.TestEnv

func TestMain(m *testing.M) {
	te = testenv.New("library_transactor")
	code := m.Run()

	te.Cleanup()

	os.Exit(code)
}
