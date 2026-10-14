package main

import (
	"github.com/thezzisu/pkudisk-fuse/internal/display"
	"io"
)

func printTable(output io.Writer, headers []string, rows [][]string, numbers ...int) error {
	return display.Table(output, headers, rows, numbers...)
}
