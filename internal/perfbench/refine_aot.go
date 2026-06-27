// Package perfbench contains end-to-end performance benchmark workflows.
package perfbench

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const refineAOTScript = "scripts/refine-aot-bench.sh"

// RunRefineAOT runs the Refine AOT benchmark command.
func RunRefineAOT(ctx context.Context, args []string) error {
	command, err := refineAOTCommand(args)
	if err != nil {
		return err
	}
	if command == "help" {
		if _, err := os.Stdout.WriteString(RefineAOTUsage()); err != nil {
			return fmt.Errorf("write refine-aot usage: %w", err)
		}
		return nil
	}
	return runRefineAOTScript(ctx, command)
}

// RefineAOTUsage returns command help for the Refine AOT workflow.
func RefineAOTUsage() string {
	return `Usage: go run ./cmd/perfcontract refine-aot <prepare|smoke|perf|stress|baseline|down>

Environment:
  REFINE_AOT_DIR       Workspace for cloned source, dist, cache, and bundle. Default: tmp/refine-aot.
  REFINE_SOURCE_DIR    Existing Refine project to build instead of cloning an example.
  REFINE_EXAMPLE_PATH  Example path inside refinedev/refine. Default: examples/app-crm-minimal.
  REFINE_EXAMPLE_REF   Git ref for refinedev/refine. Default: @refinedev/core@5.0.12.
  REFINE_FIXTURE_PACKAGES
                      Extra npm packages imported by the benchmark fixture entry.
  REFINE_BUILD_MODE    Build mode: vite or package. Default: vite.
  REFINE_PACKAGE_MANAGER
                      Package manager command, or auto. Default: auto.
  BENCH_GOARCH         Linux runtime architecture for the Docker image. Default: amd64.
  SPACK_RUNTIME_BENCH_IMAGE
                      Runtime image used by direct and AOT containers. Default: spack-k6-bench:local.
  SPACK_COMPILER_BENCH_IMAGE
                      Compiler image used to produce the .spack bundle in Docker. Default: spack-compiler-bench:local.
  SPACK_RUNTIME_BENCH_BUILD
                      Build the local runtime image before running. Default: true.
  SPACK_COMPILER_BENCH_BUILD
                      Build the local compiler image before running. Default: true.
  K6_VUS               k6 virtual users. Default: 64, smoke defaults to 1.
  K6_DURATION          k6 duration. Default: 30s, smoke defaults to 5s.
  ACCEPT_ENCODING      Accept-Encoding header for k6. Default: br,gzip.
`
}

func refineAOTCommand(args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("too many refine-aot arguments: %s\n%s", strings.Join(args[1:], " "), RefineAOTUsage())
	}
	command := "smoke"
	if len(args) == 1 {
		command = strings.TrimSpace(args[0])
	}
	switch command {
	case "", "smoke":
		return "smoke", nil
	case "prepare", "perf", "stress", "baseline", "down":
		return command, nil
	case "-h", "--help", "help":
		return "help", nil
	default:
		return "", fmt.Errorf("unknown refine-aot command %q\n%s", command, RefineAOTUsage())
	}
}

func runRefineAOTScript(ctx context.Context, command string) error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	script := filepath.Join(root, refineAOTScript)
	if _, statErr := os.Stat(script); statErr != nil {
		return fmt.Errorf("stat refine-aot script: %w", statErr)
	}

	cmd, err := refineAOTScriptCommand(ctx, command)
	if err != nil {
		return err
	}
	cmd.Dir = root
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run refine-aot script: %w", err)
	}
	return nil
}

func refineAOTScriptCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	switch command {
	case "prepare":
		return exec.CommandContext(ctx, "bash", "scripts/refine-aot-bench.sh", "prepare"), nil
	case "smoke":
		return exec.CommandContext(ctx, "bash", "scripts/refine-aot-bench.sh", "smoke"), nil
	case "perf":
		return exec.CommandContext(ctx, "bash", "scripts/refine-aot-bench.sh", "perf"), nil
	case "stress":
		return exec.CommandContext(ctx, "bash", "scripts/refine-aot-bench.sh", "stress"), nil
	case "baseline":
		return exec.CommandContext(ctx, "bash", "scripts/refine-aot-bench.sh", "baseline"), nil
	case "down":
		return exec.CommandContext(ctx, "bash", "scripts/refine-aot-bench.sh", "down"), nil
	default:
		return nil, fmt.Errorf("unsupported refine-aot command %q", command)
	}
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, refineAOTScript)) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find repository root")
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
