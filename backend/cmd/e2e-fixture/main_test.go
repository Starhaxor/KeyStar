package main

import "testing"

func TestValidateDedicatedDatabaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		databaseURL string
		wantError   bool
	}{
		{
			name:        "dedicated e2e database",
			databaseURL: "postgres://keystar_test:secret@localhost:5432/keystar_test?sslmode=disable",
		},
		{
			name:        "developer database",
			databaseURL: "postgres://postgres:secret@localhost:5432/keystar?sslmode=disable",
			wantError:   true,
		},
		{
			name:        "similarly named database",
			databaseURL: "postgres://postgres:secret@localhost:5432/keystar_test_backup?sslmode=disable",
			wantError:   true,
		},
		{
			name:        "missing database name",
			databaseURL: "postgres://postgres:secret@localhost:5432/?sslmode=disable",
			wantError:   true,
		},
		{
			name:        "invalid URL",
			databaseURL: "://not-a-postgres-url",
			wantError:   true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateDedicatedDatabaseURL(test.databaseURL)
			if test.wantError && err == nil {
				t.Fatal("validateDedicatedDatabaseURL() error = nil, want an error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("validateDedicatedDatabaseURL() error = %v", err)
			}
		})
	}
}
