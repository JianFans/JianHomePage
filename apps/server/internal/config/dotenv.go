package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// LoadEnvironment reads only the working directory's .env and lets explicitly
// provided process variables override file values without mutating the environment.
func LoadEnvironment() (Config, error) {
	return loadEnvironment(".env", os.LookupEnv)
}

// loadEnvironment keeps absent files optional while failing closed on malformed
// or unreadable files. Parser errors are redacted because they may contain secrets.
func loadEnvironment(path string, lookup func(string) (string, bool)) (Config, error) {
	values, err := godotenv.Read(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("%w: cannot read or parse .env", ErrInvalidConfig)
	}
	return Load(func(key string) string {
		if value, exists := lookup(key); exists {
			return value
		}
		return values[key]
	})
}
