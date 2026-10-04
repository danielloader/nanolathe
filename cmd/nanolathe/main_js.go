//go:build js && wasm

package main

import "os"

func main() {
	opts, code, ok := mainOptions(os.Args[1:], os.Stdout)
	if !ok {
		if code != 0 {
			os.Exit(code)
		}
		return
	}
	// The browser's explicit stress diagnostic reuses the existing authored
	// three-army fixture (DESIGN_BROWSER_HOST §3).
	opts.LiveScene = os.Getenv("NANOLATHE_BROWSER_SCENE")
	if code := runOptions(opts, os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func browserStageBattle(opts Options, shell *gameShell) error {
	if opts.LiveScene == "" {
		return nil
	}
	scene, err := parseLiveScene(opts.LiveScene)
	if err != nil {
		return err
	}
	_, err = stageLiveScene(opts, shell, scene)
	return err
}
