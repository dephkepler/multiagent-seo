//go:build integration

package handlers_test

import (
	"context"
	"os"
	"testing"

	"multiagent-seo/internal/testsupport"
)

var baseConnStr string

func TestMain(m *testing.M) {
	ctx := context.Background()
	conn, terminate, err := testsupport.StartPostgres(ctx)
	if err != nil {
		panic(err)
	}
	baseConnStr = conn
	code := m.Run()
	terminate()
	os.Exit(code)
}
