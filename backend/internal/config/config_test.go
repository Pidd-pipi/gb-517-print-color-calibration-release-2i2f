package config

import "testing"

func TestLoadRejectsUnknownDatabaseDriver(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "unknown")
	if _, err := Load(); err == nil {
		t.Fatal("expected unsupported database driver to fail")
	}
}

func TestLoadAcceptsSQLiteForLocalSmoke(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", ":memory:")
	t.Setenv("JWT_SECRET", "this-is-long-enough-for-tests")
	if _, err := Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
}
