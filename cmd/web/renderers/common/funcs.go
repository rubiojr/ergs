package common

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// BlockRenderer defines the interface for rendering different types of blocks
type BlockRenderer interface {
	// Render takes a block and returns formatted HTML for display
	Render(block core.Block) template.HTML

	// CanRender checks if this renderer can handle the given block type
	CanRender(block core.Block) bool

	// GetDatasourceType returns the datasource type this renderer handles
	GetDatasourceType() string
}

// Global registry for auto-registration
var globalRenderers []BlockRenderer

// RegisterRenderer adds a renderer to the global registry
func RegisterRenderer(renderer BlockRenderer) {
	globalRenderers = append(globalRenderers, renderer)
}

// GetRegisteredRenderers returns all registered renderers
func GetRegisteredRenderers() []BlockRenderer {
	return globalRenderers
}

// TemplateData holds data passed to block templates
type TemplateData struct {
	Block    core.Block
	Metadata map[string]interface{}
	Links    []string
}

// ExtractLinks finds URLs in text
func ExtractLinks(text string) []string {
	var links []string
	words := strings.Fields(text)

	for _, word := range words {
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			links = append(links, word)
		}
	}

	return links
}

// FormatTime formats a time for display
func FormatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		minutes := int(diff.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}

// GetTemplateFuncs returns common template functions used across renderers
func GetTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		// Time formatting
		"formatTime": FormatTime,

		// Text processing
		"extractLinks": ExtractLinks,
		"htmlEscape":   template.HTMLEscapeString,
		"safeHTML":     func(s string) template.HTML { return template.HTML(s) },
		"truncate": func(s string, length int) string {
			if len(s) <= length {
				return s
			}
			return s[:length] + "..."
		},

		// JSON parsing
		"parseJSON": func(s string) interface{} {
			var result interface{}
			if err := json.Unmarshal([]byte(s), &result); err != nil {
				return nil
			}
			return result
		},
		"list": func() []interface{} {
			return []interface{}{}
		},

		// Template logic
		"default": func(def, val interface{}) interface{} {
			if val == nil || val == "" {
				return def
			}
			return val
		},
		"index":  func(m map[string]interface{}, key string) interface{} { return m[key] },
		"printf": fmt.Sprintf,
		"ne":     func(a, b interface{}) bool { return a != b },
		"eq":     func(a, b interface{}) bool { return a == b },
		"and": func(args ...interface{}) bool {
			for _, arg := range args {
				if arg == nil || arg == "" || arg == false {
					return false
				}
			}
			return len(args) > 0
		},
		"or": func(args ...interface{}) bool {
			for _, arg := range args {
				if arg != nil && arg != "" && arg != false {
					return true
				}
			}
			return false
		},
		"gt": func(a, b interface{}) bool {
			return compareNumbers(a, b) > 0
		},
		"lt": func(a, b interface{}) bool {
			return compareNumbers(a, b) < 0
		},

		// String helpers
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"title":     cases.Title(language.English).String,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"replace": func(s, old, new string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"split": strings.Split,
		"trim":  strings.TrimSpace,
		"join":  strings.Join,

		// Slice helpers
		"slice": func(args ...string) []string {
			return args
		},

		// Metadata filtering helper
		"filterMetadata": func(metadata map[string]interface{}, excludeFields []string) map[string]interface{} {
			filtered := make(map[string]interface{})
			excludeSet := make(map[string]bool)
			for _, field := range excludeFields {
				excludeSet[field] = true
			}

			for key, value := range metadata {
				if !excludeSet[key] && value != nil && value != "" && fmt.Sprintf("%v", value) != "0" {
					filtered[key] = value
				}
			}
			return filtered
		},
	}
}

// compareNumbers compares two numeric values, returning -1, 0, or 1
func compareNumbers(a, b interface{}) int {
	aVal := getNumericValue(a)
	bVal := getNumericValue(b)

	if aVal < bVal {
		return -1
	} else if aVal > bVal {
		return 1
	}
	return 0
}

// getNumericValue extracts numeric value from interface{}
func getNumericValue(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	case float32:
		return float64(val)
	default:
		return 0
	}
}
