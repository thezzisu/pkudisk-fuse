//go:build linux || darwin

package desktop

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"time"
)

func openCommand(name, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, url)
	// Helpers and browsers must never echo the ticket to our logs or inherit
	// the caller's terminal input. Arguments are passed without a shell.
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		return errors.New("default browser launch failed")
	}
	return nil
}
