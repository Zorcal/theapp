// Package git provides utility functions for interacting with git.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ProjectRoot returns the git root of the project.
func ProjectRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cmd: %w", err)
	}

	return strings.TrimSpace(out.String()), nil
}
