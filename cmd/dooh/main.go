package main

import (
	"errors"
	"fmt"
	"os"

	"dooh/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdout); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			fmt.Fprintln(os.Stderr, exitErr.Message)
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
