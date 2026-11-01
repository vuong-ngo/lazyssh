package main

import (
	"fmt"
	"os"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("lazyssh version %s (commit %s, built at %s)\n", version, commit, date)
		return
	}

	fmt.Println("lazyssh: The simple, fast, and keyboard-driven terminal UI for managing SSH connections.")
	fmt.Println("Project is initialized. See documentation in docs/ for architecture and specs.")
}
