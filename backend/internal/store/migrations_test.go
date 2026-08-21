package store

import "testing"

func TestVersionedMigrationsIncludeModerationSchema(t *testing.T) {
	last := versionedMigrations[len(versionedMigrations)-1]
	if last.version != 15 || last.up != "000015_moderation.up.sql" {
		t.Fatalf("latest migration = %#v, want moderation migration 15", last)
	}
}
