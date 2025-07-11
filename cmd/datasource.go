package cmd

import (
	"context"
	"fmt"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/urfave/cli/v3"
)

// DatasourceCommand creates the datasource command with subcommands
func DatasourceCommand() *cli.Command {
	return &cli.Command{
		Name:  "datasource",
		Usage: "Manage datasources",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List datasources",
				Action: func(ctx context.Context, c *cli.Command) error {
					return listDatasources(c.String("config"))
				},
			},
			{
				Name:  "remove",
				Usage: "Remove a datasource",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Usage:    "Datasource name",
						Required: true,
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return removeDatasource(c.String("config"), c.String("name"))
				},
			},
		},
	}
}

// listDatasources lists all configured datasources
func listDatasources(configPath string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	datasources := cfg.ListDatasources()
	if len(datasources) == 0 {
		fmt.Println("No datasources configured")
		return nil
	}

	fmt.Println("Configured datasources:")
	for _, name := range datasources {
		dsType, _, err := cfg.GetDatasourceConfig(name)
		if err != nil {
			fmt.Printf("  %s: error loading config: %v\n", name, err)
			continue
		}
		interval := cfg.GetDatasourceInterval(name)
		fmt.Printf("  %s (%s) - interval: %v\n", name, dsType, interval)
	}

	return nil
}

// removeDatasource removes a datasource from the configuration
func removeDatasource(configPath, name string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	cfg.RemoveDatasource(name)

	if err := cfg.SaveConfig(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Removed datasource '%s'\n", name)
	return nil
}
