package gui

import (
	"context"
)

func RunMode(ctx context.Context, opts Options, _, _ bool) error { return Run(ctx, opts) }
