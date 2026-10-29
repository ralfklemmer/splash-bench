package check

import (
	"context"
	"fmt"
	"os/exec"
)

// ExecRunner runs local programs; it is the CommandRunner for real use.
type ExecRunner struct{}

func (ExecRunner) Output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}
