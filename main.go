package main

import (
	"os"

	"github.com/gnomegl/teleslurp/internal/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
