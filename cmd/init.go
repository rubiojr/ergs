package cmd

import (
	"context"
	"fmt"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/urfave/cli/v3"
)

// InitCommand creates the init command
func InitCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize configuration",
		Action: func(ctx context.Context, c *cli.Command) error {
			return initConfig(c.String("config"))
		},
	}
}

// initConfig initializes the configuration file
func initConfig(configPath string) error {
	cfg := config.GetDefaultConfig()
	if err := cfg.SaveTemplateConfig(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Configuration initialized at %s\n", configPath)
	return nil
}
