package cmd

import (
	"context"
	"fmt"
	"os"

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
	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Configuration file already exists at %s\n", configPath)
		return nil
	}

	cfg, err := config.GetDefaultConfig()
	if err != nil {
		return fmt.Errorf("getting default config: %w", err)
	}
	if err := cfg.SaveTemplateConfig(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Configuration initialized at %s\n", configPath)
	return nil
}
