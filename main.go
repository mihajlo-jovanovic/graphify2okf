package main

import "os"

func main() {
	if err := newRootCommand(os.Stdout, os.Stderr).Execute(); err != nil {
		os.Exit(1)
	}
}
