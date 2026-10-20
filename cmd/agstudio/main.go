package main

import (
	"fmt"
	"os"

	"agent-studio/internal/tui"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			fmt.Println("Usage: agstudio\n\nOpen the Agent Studio terminal interface.")
			return
		case "--version", "-v", "version":
			fmt.Println(version)
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown option: %s\n", os.Args[1])
			os.Exit(2)
		}
	}

	if err := tui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "agstudio:", err)
		os.Exit(1)
	}
}
