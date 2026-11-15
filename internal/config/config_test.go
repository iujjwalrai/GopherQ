package config_test

import (
	"strings"
	"testing"

	"github.com/ujjwal/gopherq/internal/config"
)

// clearEnv makes every GopherQ variable empty for this test, so values
// from your own shell can't leak in. t.Setenv undoes this after the test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"PORT", "WORKERS", "DATABASE_URL"} {
		t.Setenv(name, "")
	}
}

func TestDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	want := config.Config{Port: 8080, Workers: 4, DatabaseURL: ""}
	if cfg != want {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestReadsValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("WORKERS", "16")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/gopherq")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}

	want := config.Config{Port: 9090, Workers: 16, DatabaseURL: "postgres://u:p@localhost:5432/gopherq"}
	if cfg != want {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestPortLimitsAreAllowed(t *testing.T) {
	for _, port := range []string{"1", "65535"} {
		t.Run(port, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("PORT", port)

			if _, err := config.Load(); err != nil {
				t.Fatalf("PORT=%s rejected: %v", port, err)
			}
		})
	}
}

func TestInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "PORT", value: "abc"},
		{name: "PORT", value: "0"},
		{name: "PORT", value: "65536"},
		{name: "PORT", value: " 9090"},
		{name: "WORKERS", value: "0"},
		{name: "WORKERS", value: "-1"},
		{name: "WORKERS", value: "four"},
	}

	for _, tc := range tests {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(tc.name, tc.value)

			_, err := config.Load()
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("error %q does not name %s", err, tc.name)
			}
		})
	}
}

func TestReportsEveryProblem(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "abc")
	t.Setenv("WORKERS", "0")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, `PORT="abc"`) || !strings.Contains(msg, `WORKERS="0"`) {
		t.Fatalf("error should mention both variables, got:\n%s", msg)
	}
}
