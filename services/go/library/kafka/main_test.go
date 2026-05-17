//go:build integration

package kafka_test

import (
	"fmt"
	"os"
	"testing"

	"go-services/library/testenv"
)

var (
	te        *testenv.TestEnv
	testKafka *testenv.Kafka
)

func TestMain(m *testing.M) {
	te = testenv.New("library_transactor")

	var err error
	testKafka, err = testenv.SetupKafka(te)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to set up kafka: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	te.Cleanup()

	os.Exit(code)
}
