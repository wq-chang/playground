package user_test

import (
	"os"
	"testing"

	"go-services/backend/migrations"
	"go-services/library/testenv"
)

var te *testenv.TestEnv

func TestMain(m *testing.M) {
	te = testenv.New("backend_user", testenv.WithMigrationTableName(migrations.MigrationTableName))
	code := m.Run()

	te.Cleanup()

	os.Exit(code)
}
