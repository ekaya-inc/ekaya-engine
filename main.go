package main

import (
	"fmt"
	"os"

	"github.com/ekaya-inc/ekaya-engine/internal/app"
	"github.com/ekaya-inc/ekaya-engine/internal/cli"
)

// Version is set at build time via ldflags
var Version = "dev"

func main() {
	if err := cli.Run(os.Args[1:], Version, func() error {
		return app.Run(Version)
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
