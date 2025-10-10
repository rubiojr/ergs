package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/urfave/cli/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Define styles using lipgloss
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			Background(lipgloss.Color("235")).
			Padding(0, 1).
			Margin(0, 0, 1, 0)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")).
			Margin(1, 0, 1, 0)

	blockStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1).
			Margin(0, 0, 1, 2)

	metaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)

	summaryStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("32")).
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("32")).
			Padding(0, 1).
			Margin(0, 0, 1, 0)

	noDataStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true).
			Margin(1, 0)

	urlStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Margin(1, 0, 0, 0)
)

// TodayCommand creates the today command
func TodayCommand() *cli.Command {
	return &cli.Command{
		Name:  "today",
		Usage: "Show all blocks stored today, grouped by datasource type",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "limit",
				Usage: "Maximum number of blocks per datasource (0 for no limit)",
				Value: 50,
			},
			&cli.BoolFlag{
				Name:  "no-pager",
				Usage: "Disable pager and output directly to terminal",
				Value: false,
			},
			&cli.StringSliceFlag{
				Name:  "datasource",
				Usage: "Filter by specific datasource(s). Can be used multiple times",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return showTodayBlocks(c.String("config"), c.Int("limit"), c.Bool("no-pager"), c.StringSlice("datasource"))
		},
	}
}

// showTodayBlocks displays all blocks stored today grouped by datasource type
func showTodayBlocks(configPath string, limit int, noPager bool, datasourceFilter []string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	registry := core.GetGlobalRegistry()

	if err := createDatasourcesFromConfig(registry, cfg); err != nil {
		return fmt.Errorf("creating datasources: %w", err)
	}
	defer func() {
		if err := registry.Close(); err != nil {
			fmt.Printf("Warning: failed to close registry: %v\n", err)
		}
	}()

	configuredDatasources := cfg.ListDatasources()
	storageManager, err := storage.NewManager(cfg.StorageDir, configuredDatasources...)
	if err != nil {
		return fmt.Errorf("creating storage manager: %w", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	if err := initializeDatasourceStorage(registry, storageManager); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	// Get start of today in local timezone
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Get blocks from all datasources since start of today
	allResults, err := getAllBlocksSince(storageManager, startOfDay, limit, datasourceFilter)
	if err != nil {
		return fmt.Errorf("getting today's blocks: %w", err)
	}

	// Generate the formatted output
	output := formatTodayOutput(allResults, startOfDay, datasourceFilter)

	// Display output with or without pager
	if noPager || !isTerminal() {
		fmt.Print(output)
	} else {
		return displayWithPager(output)
	}

	return nil
}

// getAllBlocksSince gets blocks from all datasources since the given time
func getAllBlocksSince(storageManager *storage.Manager, since time.Time, limit int, datasourceFilter []string) (map[string][]core.Block, error) {
	results := make(map[string][]core.Block)

	// Convert filter slice to map for efficient lookup
	filterMap := make(map[string]bool)
	for _, ds := range datasourceFilter {
		filterMap[ds] = true
	}
	hasFilter := len(datasourceFilter) > 0

	// Get all storage instances and query each one
	stats, err := storageManager.GetStats()
	if err != nil {
		return nil, fmt.Errorf("getting storage stats: %w", err)
	}

	for datasourceName := range stats {
		if datasourceName == "total_blocks" || datasourceName == "total_datasources" {
			continue
		}

		// Apply datasource filter if specified
		if hasFilter && !filterMap[datasourceName] {
			continue
		}

		storage, err := storageManager.GetStorage(datasourceName)
		if err != nil {
			return nil, fmt.Errorf("getting storage for %s: %w", datasourceName, err)
		}

		blocks, err := storage.GetBlocksSince(since)
		if err != nil {
			return nil, fmt.Errorf("getting blocks from %s: %w", datasourceName, err)
		}

		if len(blocks) > 0 {
			// Apply limit if specified
			if limit > 0 && len(blocks) > limit {
				blocks = blocks[:limit]
			}
			results[datasourceName] = blocks
		}
	}

	return results, nil
}

// formatTodayOutput creates the formatted output for today's blocks
func formatTodayOutput(results map[string][]core.Block, startOfDay time.Time, datasourceFilter []string) string {
	var output strings.Builder

	// Title
	today := startOfDay.Format("Monday, January 2, 2006")
	title := fmt.Sprintf("📅 Today's Blocks - %s", today)
	output.WriteString(titleStyle.Render(title))
	output.WriteString("\n")

	if len(results) == 0 {
		message := "No blocks found for today"
		if len(datasourceFilter) > 0 {
			if len(datasourceFilter) == 1 {
				message += fmt.Sprintf(" for datasource '%s'", datasourceFilter[0])
			} else {
				message += fmt.Sprintf(" for datasources: %s", strings.Join(datasourceFilter, ", "))
			}
		}
		message += "."
		output.WriteString(noDataStyle.Render(message))
		output.WriteString("\n")
		return output.String()
	}

	// Group by datasource type
	typeGroups := groupByDatasourceType(results)

	// Calculate totals for summary
	totalBlocks := 0
	totalDatasources := len(results)
	for _, blocks := range results {
		totalBlocks += len(blocks)
	}

	// Summary
	summary := fmt.Sprintf("📊 Summary: %d blocks across %d datasources, %d types",
		totalBlocks, totalDatasources, len(typeGroups))
	if len(datasourceFilter) > 0 {
		if len(datasourceFilter) == 1 {
			summary += fmt.Sprintf(" (filtered to: %s)", datasourceFilter[0])
		} else {
			summary += fmt.Sprintf(" (filtered to: %s)", strings.Join(datasourceFilter, ", "))
		}
	}
	output.WriteString(summaryStyle.Render(summary))
	output.WriteString("\n")

	// Sort types for consistent output
	var sortedTypes []string
	for dsType := range typeGroups {
		sortedTypes = append(sortedTypes, dsType)
	}
	sort.Strings(sortedTypes)

	// Output each datasource type
	for _, dsType := range sortedTypes {
		datasources := typeGroups[dsType]

		// Calculate total blocks for this type
		totalBlocksForType := 0
		for _, blocks := range datasources {
			totalBlocksForType += len(blocks)
		}

		// Type header
		typeHeader := fmt.Sprintf("🔧 %s (%d blocks)", cases.Title(language.English).String(dsType), totalBlocksForType)
		output.WriteString(headerStyle.Render(typeHeader))
		output.WriteString("\n")

		// Sort datasource names within type
		var sortedNames []string
		for name := range datasources {
			sortedNames = append(sortedNames, name)
		}
		sort.Strings(sortedNames)

		// Output each datasource within the type
		for _, name := range sortedNames {
			blocks := datasources[name]

			// Output blocks for this datasource
			for j, block := range blocks {
				blockContent := formatBlock(block, j+1)
				output.WriteString(blockContent)
				output.WriteString("\n")
			}
		}
	}

	return output.String()
}

// groupByDatasourceType groups results by datasource type
func groupByDatasourceType(results map[string][]core.Block) map[string]map[string][]core.Block {
	typeGroups := make(map[string]map[string][]core.Block)

	for datasourceName, blocks := range results {
		if len(blocks) == 0 {
			continue
		}

		// Get datasource type from the first block
		var dsType string
		if genericBlock, ok := blocks[0].(*core.GenericBlock); ok {
			dsType = genericBlock.DSType()
		}
		if dsType == "" {
			dsType = "unknown"
		}

		if typeGroups[dsType] == nil {
			typeGroups[dsType] = make(map[string][]core.Block)
		}
		typeGroups[dsType][datasourceName] = blocks
	}

	return typeGroups
}

// formatBlock formats a single block for display
func formatBlock(block core.Block, index int) string {
	var content strings.Builder

	// Block number and time
	timeStr := block.CreatedAt().Format("15:04:05")
	header := fmt.Sprintf("#%d - %s", index, timeStr)
	content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")).Render(header))
	content.WriteString("\n\n")

	// Use the block's Summary method for compact display
	summary := block.Summary()

	// Clean up summary by removing "title=" prefixes
	summary = strings.ReplaceAll(summary, "title=", "")

	content.WriteString(summary)

	// Extract and add URL with proper spacing if available
	metadata := block.Metadata()
	url := extractURL(metadata)
	if url != "" {
		urlText := fmt.Sprintf("🔗 %s", url)
		content.WriteString("\n" + urlStyle.Render(urlText))
	}

	// Add metadata footer
	if len(metadata) > 0 {
		content.WriteString("\n\n")
		metaInfo := fmt.Sprintf("ID: %s | Source: %s", block.ID(), block.Source())
		content.WriteString(metaStyle.Render(metaInfo))
	}

	return blockStyle.Render(content.String())
}

// extractURL extracts URL from block metadata
func extractURL(metadata map[string]interface{}) string {
	// Check common URL field names
	urlFields := []string{"url", "link", "repo_url"}

	for _, field := range urlFields {
		if value, exists := metadata[field]; exists {
			if urlStr, ok := value.(string); ok && urlStr != "" {
				return urlStr
			}
		}
	}

	return ""
}

// isTerminal checks if stdout is a terminal
func isTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// displayWithPager displays content using a pager
func displayWithPager(content string) error {
	// Try to find a suitable pager
	pagerCmd := os.Getenv("PAGER")
	if pagerCmd == "" {
		// Try common pagers in order of preference
		pagers := []string{"less", "more", "cat"}
		for _, pager := range pagers {
			if _, err := exec.LookPath(pager); err == nil {
				pagerCmd = pager
				break
			}
		}
	}

	if pagerCmd == "" {
		// No pager found, output directly
		fmt.Print(content)
		return nil
	}

	// Set up less with good defaults if it's available
	args := []string{}
	if strings.Contains(pagerCmd, "less") {
		args = []string{"-R", "-S", "-F", "-X"}
	}

	cmd := exec.Command(pagerCmd, args...)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
