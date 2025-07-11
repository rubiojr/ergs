package main

import (
	"context"
	"log"
	"os"

	"github.com/rubiojr/ergs/cmd"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:  "ergs",
		Usage: "A generic data fetching and indexing tool",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug logging",
				Value: false,
			},
			&cli.StringFlag{
				Name:  "config",
				Usage: "Configuration file path",
				Value: config.GetDefaultConfigPath(),
			},
		},
		Commands: []*cli.Command{
			cmd.InitCommand(),
			cmd.DatasourceCommand(),
			cmd.FetchCommand(),
			cmd.SearchCommand(),
			cmd.ListCommand(),
			cmd.ServeCommand(),
			cmd.StatsCommand(),
			cmd.OptimizeCommand(),
			cmd.VersionCommand(),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
