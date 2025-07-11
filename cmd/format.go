package cmd

import (
	"fmt"
	"sort"
	"time"
)

// formatNumber formats a number with K/M suffixes for readability
func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	} else if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	} else {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
}

// formatTime formats a time relative to now or as an absolute date
func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	// If it's within the last day, show relative time
	if diff < 24*time.Hour {
		if diff < time.Hour {
			minutes := int(diff.Minutes())
			if minutes < 1 {
				return "just now"
			}
			return fmt.Sprintf("%d minutes ago", minutes)
		}
		hours := int(diff.Hours())
		return fmt.Sprintf("%d hours ago", hours)
	}

	// If it's within the last week, show days ago
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	}

	// Otherwise show the date
	if t.Year() == now.Year() {
		return t.Format("Jan 2, 15:04")
	}
	return t.Format("Jan 2, 2006")
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1f hours", d.Hours())
	} else if d < 30*24*time.Hour {
		return fmt.Sprintf("%.1f days", d.Hours()/24)
	} else if d < 365*24*time.Hour {
		return fmt.Sprintf("%.1f months", d.Hours()/(24*30))
	} else {
		return fmt.Sprintf("%.1f years", d.Hours()/(24*365))
	}
}

// formatStats formats storage statistics for display
func formatStats(stats map[string]interface{}) {
	// Print summary
	fmt.Printf("📊 Storage Statistics\n")
	fmt.Printf("═══════════════════════\n\n")

	totalBlocks, _ := stats["total_blocks"].(int)
	totalDatasources, _ := stats["total_datasources"].(int)

	fmt.Printf("Total blocks: %s\n", formatNumber(totalBlocks))
	fmt.Printf("Total datasources: %d\n\n", totalDatasources)

	if totalDatasources == 0 {
		fmt.Printf("No datasources configured yet.\n")
		return
	}

	// Get datasource names and sort them
	var datasourceNames []string
	for key := range stats {
		if key != "total_blocks" && key != "total_datasources" {
			datasourceNames = append(datasourceNames, key)
		}
	}
	sort.Strings(datasourceNames)

	// Print each datasource
	fmt.Printf("Datasource Details:\n")
	fmt.Printf("───────────────────\n")

	for i, name := range datasourceNames {
		if i > 0 {
			fmt.Printf("\n")
		}

		dsStats, ok := stats[name].(map[string]interface{})
		if !ok {
			fmt.Printf("❌ %s: No data available\n", name)
			continue
		}

		fmt.Printf("📁 %s\n", name)

		if totalBlocks, ok := dsStats["total_blocks"].(int); ok {
			fmt.Printf("   Blocks: %s", formatNumber(totalBlocks))

			// Calculate percentage
			if totalBlocks > 0 {
				percentage := float64(totalBlocks) / float64(stats["total_blocks"].(int)) * 100
				fmt.Printf(" (%.1f%%)", percentage)
			}
			fmt.Printf("\n")
		}

		if oldestBlock, ok := dsStats["oldest_block"].(time.Time); ok {
			fmt.Printf("   Oldest: %s\n", formatTime(oldestBlock))
		}

		if newestBlock, ok := dsStats["newest_block"].(time.Time); ok {
			fmt.Printf("   Newest: %s\n", formatTime(newestBlock))

			// Calculate time span
			if oldestBlock, ok := dsStats["oldest_block"].(time.Time); ok {
				duration := newestBlock.Sub(oldestBlock)
				fmt.Printf("   Span:   %s\n", formatDuration(duration))
			}
		}
	}
}
