package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// VersionCommand creates the version command
func VersionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Show version information",
		Action: func(ctx context.Context, c *cli.Command) error {
			fmt.Println("ergs version 1.0.0")
			return nil
		},
	}
}
