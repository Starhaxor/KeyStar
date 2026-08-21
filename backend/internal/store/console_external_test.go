package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExternalConsoleUserList(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" { t.Skip("TEST_DATABASE_URL is not set") }
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil { t.Fatal(err) }
	defer pool.Close()
	repository := New(pool)
	application, err := repository.FindApplicationBySlug(context.Background(), "starloader")
	if err != nil { t.Fatal(err) }
	if _, _, err = repository.ListConsoleUsers(context.Background(), application.ID, 0, 20, "", ""); err != nil { t.Fatal(err) }
}
