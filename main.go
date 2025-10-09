package main

import (
	"context"
	"log"
	"os"

	"github.com/rubiojr/ergs/cmd"
	"github.com/rubiojr/ergs/pkg/config"
	_ "github.com/rubiojr/ergs/pkg/datasources/importer"
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
				Value: getDefaultConfigPathOrExit(),
			},
		},
		Commands: []*cli.Command{
			cmd.InitCommand(),
			cmd.DatasourceCommand(),
			cmd.FetchCommand(),
			cmd.FirehoseCommand(),
			cmd.SearchCommand(),
			cmd.ListCommand(),
			cmd.TodayCommand(),
			cmd.ServeCommand(),
			cmd.WebCommand(),
			cmd.ImporterCommand(),
			cmd.StatsCommand(),
			cmd.OptimizeCommand(),
			cmd.MigrateCommand(),
			cmd.VersionCommand(),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func getDefaultConfigPathOrExit() string {
	path, err := config.GetDefaultConfigPath()
	if err != nil {
		log.Fatalf("Failed to get default config path: %v", err)
	}
	return path
}
