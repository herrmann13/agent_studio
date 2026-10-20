package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/herrmann13/agent_studio/internal/tui"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			fmt.Println("Usage: agstudio\n\nOpen the Agent Studio terminal interface.")
			return
		case "--version", "-v", "version":
			fmt.Println(currentVersion())
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

func currentVersion() string {
	if version != "dev" {
		return version
	}
	buildInfo, ok := debug.ReadBuildInfo()
	if ok && buildInfo.Main.Version != "" && buildInfo.Main.Version != "(devel)" {
		return buildInfo.Main.Version
	}
	return version
}
