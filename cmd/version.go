package cmd

import (
	"context"
	"fmt"

	"github.com/rubiojr/ergs/pkg/version"
	"github.com/urfave/cli/v3"
)

// VersionCommand creates the version command
func VersionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Show version information",
		Action: func(ctx context.Context, c *cli.Command) error {
			fmt.Println(version.BuildVersion())
			return nil
		},
	}
}
