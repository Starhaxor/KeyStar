package store

import "testing"

func TestVersionedMigrationsIncludeModerationSchema(t *testing.T) {
	last := versionedMigrations[len(versionedMigrations)-1]
	if last.version != 16 || last.up != "000016_console_lifecycle.up.sql" {
		t.Fatalf("latest migration = %#v, want console lifecycle migration 16", last)
	}
}
