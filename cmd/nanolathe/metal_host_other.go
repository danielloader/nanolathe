//go:build !darwin

package main

import "fmt"

func runMetal(opts Options, cs *contentSet) error {
	return fmt.Errorf("nanolathe: the Metal renderer requires macOS")
}
