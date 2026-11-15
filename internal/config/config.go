// Package config reads GopherQ's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
)

const (
	defaultPort    = 8080
	defaultWorkers = 4
)

// Config holds every setting GopherQ reads at startup.
type Config struct {
	// Port is the HTTP port to listen on. Env: PORT, default 8080.
	Port int

	// Workers is how many jobs run at the same time. Env: WORKERS,
	// default 4. Used from Phase 2.
	Workers int

	// DatabaseURL is the Postgres connection string. Env: DATABASE_URL,
	// default empty. Used from Phase 3. It contains a password, so never
	// log it.
	DatabaseURL string
}

// Load reads the configuration from the environment. A variable that is
// unset or empty gets its default. If any value is invalid, Load returns
// one error that lists every problem.
func Load() (Config, error) {
	var errs []error

	port, err := intFromEnv("PORT", defaultPort, 1, 65535)
	errs = append(errs, err)

	workers, err := intFromEnv("WORKERS", defaultWorkers, 1, math.MaxInt)
	errs = append(errs, err)

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}

	return Config{
		Port:        port,
		Workers:     workers,
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}, nil
}

// intFromEnv reads the variable called name as a whole number between lo
// and hi. It returns def when the variable is unset or empty.
func intFromEnv(name string, def, lo, hi int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return def, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("%s=%q: must be %s", name, raw, describeRange(lo, hi))
	}

	return n, nil
}

func describeRange(lo, hi int) string {
	if hi == math.MaxInt {
		return fmt.Sprintf("a number of at least %d", lo)
	}
	return fmt.Sprintf("a number between %d and %d", lo, hi)
}
