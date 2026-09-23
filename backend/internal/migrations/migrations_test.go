package migrations

import "testing"

func TestEmbeddedMigrationsAreOrdered(t *testing.T) {
	migrations, err := load()
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("migration count = %d, want 1", len(migrations))
	}
	if migrations[0].version != 1 || migrations[0].name != "000001_initial.sql" {
		t.Fatalf("migration = %#v", migrations[0])
	}
	if migrations[0].sql == "" {
		t.Fatal("migration SQL is empty")
	}
}
