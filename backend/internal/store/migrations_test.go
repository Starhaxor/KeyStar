package store

import (
	"strings"
	"testing"

	"github.com/starloader/backend/migrations"
)

func TestVersionedMigrationsIncludeSharedRateLimits(t *testing.T) {
	last := versionedMigrations[len(versionedMigrations)-1]
	if last.version != 19 || last.up != "000019_admin_bootstrap_state.up.sql" {
		t.Fatalf("latest migration = %#v, want admin bootstrap state migration 19", last)
	}
}

func TestConsoleLifecycleMigrationReplacesChecksBeforeRewritingStatuses(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		drop        string
		statusWrite string
	}{
		{name: "up products", file: "000016_console_lifecycle.up.sql", drop: "alter table products drop constraint", statusWrite: "update products set status = 'inactive'"},
		{name: "up plans", file: "000016_console_lifecycle.up.sql", drop: "alter table plans drop constraint", statusWrite: "update plans set status = 'inactive'"},
		{name: "down products", file: "000016_console_lifecycle.down.sql", drop: "alter table products drop constraint", statusWrite: "update products set status = 'disabled'"},
		{name: "down plans", file: "000016_console_lifecycle.down.sql", drop: "alter table plans drop constraint", statusWrite: "update plans set status = 'disabled'"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql, err := migrations.Files.ReadFile(test.file)
			if err != nil {
				t.Fatalf("read migration: %v", err)
			}
			dropAt := strings.Index(string(sql), test.drop)
			writeAt := strings.Index(string(sql), test.statusWrite)
			if dropAt < 0 || writeAt < 0 || dropAt > writeAt {
				t.Fatalf("migration %s rewrites statuses before removing the incompatible CHECK constraint", test.file)
			}
		})
	}
}
