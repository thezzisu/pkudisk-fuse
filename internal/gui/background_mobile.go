//go:build android || ios

package gui

import "context"

// Mobile hosts own process lifetime; they never detach or launch a desktop UI.
func RunMode(ctx context.Context, opts Options, _, _ bool) error { return Run(ctx, opts) }
