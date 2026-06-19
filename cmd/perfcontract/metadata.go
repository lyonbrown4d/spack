package main

import (
	"os"
	"runtime"
	"strings"
	"time"
)

func collectMetadata() metadata {
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	return metadata{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Commit:      firstNonEmpty(os.Getenv("GITHUB_SHA"), os.Getenv("GIT_COMMIT")),
		GoVersion:   runtime.Version(),
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		NumCPU:      runtime.NumCPU(),
		Hostname:    host,
		Environment: map[string]string{
			"CGO_ENABLED": os.Getenv("CGO_ENABLED"),
			"GOFLAGS":     os.Getenv("GOFLAGS"),
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
