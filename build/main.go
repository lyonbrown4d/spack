// Package main defines SPACK build automation tasks.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/goyek/goyek/v3"
	"github.com/goyek/x/boot"
)

const (
	distDir       = "dist"
	runtimeBinary = "spack-runtime"
	goLDFlags     = "-s -w -buildid="
)

var test = goyek.Define(goyek.Task{
	Name:  "test",
	Usage: "run the full Go test suite",
	Action: func(a *goyek.A) {
		run(a, goCommand("test", "./..."))
	},
})

var testLibvips = goyek.Define(goyek.Task{
	Name:  "test-libvips",
	Usage: "run libvips-tagged pipeline tests",
	Action: func(a *goyek.A) {
		run(a, goCommand("test", "-tags=spack_libvips", "./internal/pipeline").withEnv("CGO_ENABLED=1"))
	},
})

var lint = goyek.Define(goyek.Task{
	Name:  "lint",
	Usage: "run golangci-lint",
	Action: func(a *goyek.A) {
		run(a, command{name: "golangci-lint", args: []string{"run", "./..."}})
	},
})

var build = goyek.Define(goyek.Task{
	Name:  "build",
	Usage: "build the current platform runtime binary",
	Action: func(a *goyek.A) {
		if err := os.MkdirAll(distDir, 0o755); err != nil {
			a.Fatal(err)
		}
		run(a, goCommand(
			"build",
			"-trimpath",
			"-ldflags="+goLDFlags,
			"-o",
			filepath.Join(distDir, runtimeBinary),
			"./cmd/spack-runtime",
		))
	},
})

var perfcontract = goyek.Define(goyek.Task{
	Name:  "perfcontract",
	Usage: "show the Refine AOT performance workflow help",
	Action: func(a *goyek.A) {
		run(a, goCommand("run", "./cmd/perfcontract", "refine-aot", "help"))
	},
})

var refineAOTPrepare = goyek.Define(goyek.Task{
	Name:  "refine-aot-prepare",
	Usage: "prepare the Refine AOT benchmark fixture",
	Action: func(a *goyek.A) {
		run(a, goCommand("run", "./cmd/perfcontract", "refine-aot", "prepare"))
	},
})

var refineAOTSmoke = goyek.Define(goyek.Task{
	Name:  "refine-aot-smoke",
	Usage: "run the Refine AOT smoke benchmark",
	Action: func(a *goyek.A) {
		run(a, goCommand("run", "./cmd/perfcontract", "refine-aot", "smoke"))
	},
})

var refineAOTPerf = goyek.Define(goyek.Task{
	Name:  "refine-aot-perf",
	Usage: "run the Refine AOT performance benchmark",
	Action: func(a *goyek.A) {
		run(a, goCommand("run", "./cmd/perfcontract", "refine-aot", "perf"))
	},
})

var refineAOTStress = goyek.Define(goyek.Task{
	Name:  "refine-aot-stress",
	Usage: "run the Refine AOT stress benchmark",
	Action: func(a *goyek.A) {
		run(a, goCommand("run", "./cmd/perfcontract", "refine-aot", "stress"))
	},
})

var refineAOTBaseline = goyek.Define(goyek.Task{
	Name:  "refine-aot-baseline",
	Usage: "run the multi-round Refine AOT performance baseline",
	Action: func(a *goyek.A) {
		run(a, goCommand("run", "./cmd/perfcontract", "refine-aot", "baseline"))
	},
})
var refineAOTDown = goyek.Define(goyek.Task{
	Name:  "refine-aot-down",
	Usage: "stop and remove Refine AOT benchmark resources",
	Action: func(a *goyek.A) {
		run(a, goCommand("run", "./cmd/perfcontract", "refine-aot", "down"))
	},
})

var releaseLocal = goyek.Define(goyek.Task{
	Name:  "release-local",
	Usage: "publish the current tag with local GoReleaser and Docker image push workflow",
	Action: func(a *goyek.A) {
		run(a, command{name: "bash", args: []string{"scripts/release-local.sh"}})
	},
})

var releaseVerify = goyek.Define(goyek.Task{
	Name:  "release-verify",
	Usage: "verify published runtime and compiler images with black-box checks",
	Action: func(a *goyek.A) {
		run(a, command{name: "bash", args: []string{"scripts/release-verify.sh"}})
	},
})

var validate = goyek.Define(goyek.Task{
	Name:  "validate",
	Usage: "run the default local validation suite",
	Action: func(a *goyek.A) {
		run(a, goCommand("test", "./..."))
		run(a, goCommand("test", "-tags=spack_libvips", "./internal/pipeline").withEnv("CGO_ENABLED=1"))
		run(a, command{name: "golangci-lint", args: []string{"run", "./..."}})
	},
})

func main() {
	if err := chdirRepoRoot(); err != nil {
		panic(err)
	}
	trimGoRunSeparator()
	goyek.SetDefault(validate)
	boot.Main()
}

type command struct {
	name string
	args []string
	env  []string
}

func goCommand(args ...string) command {
	return command{name: "go", args: args}
}

func (c command) withEnv(env ...string) command {
	c.env = append(c.env, env...)
	return c
}

func run(a *goyek.A, c command) {
	a.Log("exec: " + c.String())
	cmd := exec.CommandContext(a.Context(), c.name, c.args...)
	cmd.Env = append(os.Environ(), c.env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		a.Fatal(err)
	}
}

func (c command) String() string {
	parts := make([]string, 0, len(c.env)+1+len(c.args))
	parts = append(parts, c.env...)
	parts = append(parts, c.name)
	parts = append(parts, c.args...)
	return strings.Join(parts, " ")
}

func fatalBuildStartup(err error) {
	if _, writeErr := fmt.Fprintf(os.Stderr, "build task startup failed: %v`n", err); writeErr != nil {
		os.Exit(1)
	}
	os.Exit(1)
}

func trimGoRunSeparator() {
	if len(os.Args) > 1 && os.Args[1] == "--" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}
}

func chdirRepoRoot() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "Taskfile.yml")) {
			return os.Chdir(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return errors.New("could not find repository root")
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
