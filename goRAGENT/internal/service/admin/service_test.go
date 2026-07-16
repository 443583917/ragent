package admin

import (
	"os"
	"testing"

	"goRAGENT/pkg/snowflake"
)

func TestMain(m *testing.M) {
	// Initialize snowflake for tests that generate IDs
	_ = snowflake.Init(0)
	os.Exit(m.Run())
}
