package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, v string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(v+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	t.Setenv("JWT_SECRET", "from-env")
	t.Setenv("JWT_SECRET_FILE", write("jwt", "from-file"))
	t.Setenv("POSTGRES_DSN", "postgres://split_svc@postgres:5432/billsplit?sslmode=disable")
	t.Setenv("POSTGRES_PASSWORD_FILE", write("db", "s3cr3t/pass"))
	t.Setenv("REDIS_PASSWORD_FILE", write("redis", "r3dis"))

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.JWT.Secret != "from-file" {
		t.Errorf("jwt = %q, _FILE must win", c.JWT.Secret)
	}
	if c.Redis.Password != "r3dis" {
		t.Errorf("redis = %q", c.Redis.Password)
	}
	if !strings.Contains(c.Postgres.DSN, "split_svc:s3cr3t%2Fpass@postgres:5432") {
		t.Errorf("dsn = %q", c.Postgres.DSN)
	}

	t.Setenv("INTERNAL_TOKEN_FILE", filepath.Join(dir, "missing"))
	if _, err := Load(); err == nil {
		t.Error("missing secret file must fail fast")
	}
}
