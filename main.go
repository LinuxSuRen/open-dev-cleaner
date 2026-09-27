// Open Dev Cleaner — a cross-platform (Windows/macOS/Linux) open source
// disk cleaner for developers: caches, container images, build outputs,
// with a risk-graded embedded web UI.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/LinuxSuRen/open-dev-cleaner/internal/cli"
)

//go:embed web
var webFiles embed.FS

// Injected at build time via -ldflags.
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	cli.Version = version
	cli.Commit = commit
	cli.BuildDate = buildDate
	if err := cli.Run(os.Args[1:], webFiles); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
