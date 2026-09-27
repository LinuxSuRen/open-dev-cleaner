package scan

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// runCmd executes an external CLI honoring Env.NoExternal with a timeout.
// Returns trimmed stdout. A missing binary returns an error.
func runCmd(ctx context.Context, env *Env, timeout time.Duration, name string, args ...string) (string, error) {
	if env == nil || env.NoExternal {
		return "", errNoExternal
	}
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		return stdout.String(), err
	}
	return stdout.String(), nil
}

type constErr string

func (e constErr) Error() string { return string(e) }

const errNoExternal = constErr("external commands disabled")

// DefaultProviders returns every built-in provider in display order.
func DefaultProviders() []Provider {
	return []Provider{
		&DockerProvider{},
		&GoProvider{},
		&NodeProvider{},
		&JavaProvider{},
		&PythonProvider{},
		&RustProvider{},
		&WorkspaceProvider{},
		&OllamaProvider{},
		&EditorProvider{},
		&AppCacheProvider{},
		&BrowserProvider{},
		&OSProvider{},
	}
}
