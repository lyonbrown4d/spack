// Package main defines SPACK build automation tasks.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/goyek/goyek/v3"
	"golang.org/x/mod/semver"
)

const (
	releasePolicyOutputEnv = "SPACK_RELEASE_POLICY_OUTPUT"
	releaseTagEnv          = "SPACK_RELEASE_TAG"
	maxDockerTagLength     = 128
)

var dockerTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

type releasePolicyResult struct {
	CurrentTag      string
	HighestStable   string
	Prerelease      bool
	PublishFloating bool
}

var releasePolicyTask = goyek.Define(goyek.Task{
	Name:  "release-policy",
	Usage: "validate a release tag and decide whether stable floating tags may advance",
	Action: func(a *goyek.A) {
		result, err := loadReleasePolicy(a.Context(), os.Getenv(releaseTagEnv))
		if err != nil {
			a.Fatal(err)
		}
		if err := writeReleasePolicy(result); err != nil {
			a.Fatal(err)
		}
	},
})

func loadReleasePolicy(ctx context.Context, currentTag string) (releasePolicyResult, error) {
	cmd := exec.CommandContext(ctx, "git", "tag", "--list", "v*")
	output, err := cmd.Output()
	if err != nil {
		return releasePolicyResult{}, fmt.Errorf("list repository release tags: %w", err)
	}
	return evaluateReleasePolicy(currentTag, strings.Fields(string(output)))
}

func evaluateReleasePolicy(currentTag string, repositoryTags []string) (releasePolicyResult, error) {
	if !isStrictSemver(currentTag) {
		return releasePolicyResult{}, fmt.Errorf("release tag is not strict semantic version: %q", currentTag)
	}
	result := releasePolicyResult{
		CurrentTag: currentTag,
		Prerelease: semver.Prerelease(currentTag) != "",
	}
	for _, tag := range repositoryTags {
		if !isStrictSemver(tag) || semver.Prerelease(tag) != "" {
			continue
		}
		if result.HighestStable == "" || semver.Compare(tag, result.HighestStable) > 0 {
			result.HighestStable = tag
		}
	}
	result.PublishFloating = !result.Prerelease &&
		(result.HighestStable == "" || semver.Compare(currentTag, result.HighestStable) >= 0)
	return result, nil
}

func isStrictSemver(tag string) bool {
	return tag != "" &&
		!strings.ContainsRune(tag, '+') &&
		len(tag) <= maxDockerTagLength &&
		dockerTagPattern.MatchString(tag) &&
		semver.IsValid(tag) &&
		semver.Canonical(tag) == tag
}

func writeReleasePolicy(result releasePolicyResult) error {
	output := fmt.Sprintf(
		"release-tag=%s\nprerelease=%t\npublish-floating=%t\nhighest-stable=%s\n",
		result.CurrentTag,
		result.Prerelease,
		result.PublishFloating,
		result.HighestStable,
	)
	outputPath := os.Getenv(releasePolicyOutputEnv)
	if outputPath == "" {
		outputPath = os.Getenv("GITHUB_OUTPUT")
	}
	if outputPath == "" {
		if _, err := fmt.Fprint(os.Stdout, output); err != nil {
			return fmt.Errorf("write release policy to stdout: %w", err)
		}
		return nil
	}
	file, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open release policy output: %w", err)
	}
	if _, err := file.WriteString(output); err != nil {
		return errors.Join(fmt.Errorf("write release policy output: %w", err), file.Close())
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close release policy output: %w", err)
	}
	return nil
}
