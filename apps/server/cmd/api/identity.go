package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

// executableGenerator retains the renderer while replacing its asset-only identity.
type executableGenerator struct {
	generator siteGenerator
	identity  string
}

// Identity identifies the entire executable, including Go orchestration and embedded resources.
func (g *executableGenerator) Identity() string { return g.identity }

// Generate delegates content rendering without changing its validation or cancellation rules.
func (g *executableGenerator) Generate(ctx context.Context, raw []byte) (map[string][]byte, error) {
	return g.generator.Generate(ctx, raw)
}

// freezeExecutableIdentity binds production generation to the complete running binary.
func freezeExecutableIdentity(generator siteGenerator) (siteGenerator, error) {
	if generator == nil {
		return nil, errors.New("site generator is required")
	}
	path, err := os.Executable()
	if err != nil {
		return nil, errors.New("running executable unavailable")
	}
	identity, err := hashExecutable(path)
	if err != nil {
		return nil, err
	}
	return &executableGenerator{generator: generator, identity: identity}, nil
}

// hashExecutable streams the executable instead of retaining another binary-sized allocation.
func hashExecutable(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("running executable could not be read")
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", errors.New("running executable could not be hashed")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
