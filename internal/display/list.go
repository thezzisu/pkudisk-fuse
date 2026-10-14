package display

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type ListOptions[T any] struct {
	JSON    bool
	Indent  string
	Headers []string
	Numbers []int
	// Row returns nil for records rendered outside the table, such as a URL.
	Row          func(T) []string
	EmptyMessage string
	// JSONValue preserves a command's envelope while receiving a nonnil slice.
	JSONValue func([]T) any
}

// RenderList gives every list command the same empty-array JSON contract and
// terminal-safe table renderer. Presentation callbacks never run for JSON.
func RenderList[T any](out io.Writer, values []T, options ListOptions[T]) error {
	if values == nil {
		values = []T{}
	}
	if options.JSON {
		var value any = values
		if options.JSONValue != nil {
			value = options.JSONValue(values)
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", options.Indent)
		return encoder.Encode(value)
	}
	if len(values) == 0 && options.EmptyMessage != "" {
		_, err := fmt.Fprintln(out, options.EmptyMessage)
		return err
	}
	if options.Row == nil {
		return errors.New("list row formatter is required")
	}
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		if row := options.Row(value); row != nil {
			rows = append(rows, row)
		}
	}
	if len(values) > 0 && len(rows) == 0 {
		return nil
	}
	return Table(out, options.Headers, rows, options.Numbers...)
}
