package render

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

// BlockRenderer defines the interface for rendering different types of blocks.
// Implementations decide if they can render a block (usually by datasource type)
// and produce trusted HTML (already sanitized / escaped as needed).
type BlockRenderer interface {
	// Render returns formatted HTML for the block
	Render(block core.Block) template.HTML
	// CanRender returns true if this renderer can handle the block
	CanRender(block core.Block) bool
	// GetDatasourceType returns the datasource *type* (e.g. "github") this renderer handles
	// or "" for generic handlers (like the default renderer).
	GetDatasourceType() string
}

// TemplateData holds data passed to block templates.
type TemplateData struct {
	Block    core.Block
	Metadata map[string]interface{}
	Links    []string
}

// Global auto-registration slice (populated via init() in renderer implementations).
var globalRenderers []BlockRenderer

// RegisterRenderer adds a renderer to the global registry (called from init()).
func RegisterRenderer(renderer BlockRenderer) {
	if renderer == nil {
		return
	}
	globalRenderers = append(globalRenderers, renderer)
}

// GetRegisteredRenderers returns all registered renderers (copy for safety).
func GetRegisteredRenderers() []BlockRenderer {
	out := make([]BlockRenderer, len(globalRenderers))
	copy(out, globalRenderers)
	return out
}

// ExtractLinks performs a lightweight URL extraction.
// Kept intentionally simple; renderers may apply richer parsing if needed.
func ExtractLinks(text string) []string {
	var links []string
	words := strings.Fields(text)
	for _, w := range words {
		if strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://") {
			// Trim common trailing punctuation
			clean := strings.TrimRight(w, ".,!?;:)]}")
			links = append(links, clean)
		}
	}
	return links
}

// FormatTime returns a human-friendly relative or absolute time string.
func FormatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hrs := int(diff.Hours())
		if hrs == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hrs)
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

// GetTemplateFuncs returns a function map used by render templates.
func GetTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		// Time
		"formatTime": FormatTime,

		// Text / links
		"extractLinks": ExtractLinks,
		"htmlEscape":   template.HTMLEscapeString,
		"safeHTML":     func(s string) template.HTML { return template.HTML(s) },

		"truncate": func(s string, length int) string {
			if len(s) <= length {
				return s
			}
			if length <= 3 {
				return s[:length]
			}
			return s[:length-3] + "..."
		},

		// JSON
		"parseJSON": func(s string) interface{} {
			var result interface{}
			if err := json.Unmarshal([]byte(s), &result); err != nil {
				return nil
			}
			return result
		},
		"list": func() []interface{} { return []interface{}{} },

		// Logic helpers
		"default": func(def, val interface{}) interface{} {
			if val == nil {
				return def
			}
			switch v := val.(type) {
			case string:
				if v == "" {
					return def
				}
			}
			return val
		},
		"index": func(m map[string]interface{}, key string) interface{} {
			if m == nil {
				return nil
			}
			return m[key]
		},
		"printf": fmt.Sprintf,
		"ne":     func(a, b interface{}) bool { return a != b },
		"eq":     func(a, b interface{}) bool { return a == b },

		"and": func(args ...interface{}) bool {
			for _, a := range args {
				if a == nil {
					return false
				}
				switch v := a.(type) {
				case bool:
					if !v {
						return false
					}
				case string:
					if v == "" {
						return false
					}
				}
			}
			return len(args) > 0
		},
		"or": func(args ...interface{}) bool {
			for _, a := range args {
				if a == nil {
					continue
				}
				switch v := a.(type) {
				case bool:
					if v {
						return true
					}
				case string:
					if v != "" {
						return true
					}
				default:
					return true
				}
			}
			return false
		},
		"gt": func(a, b interface{}) bool { return compareNumbers(a, b) > 0 },
		"lt": func(a, b interface{}) bool { return compareNumbers(a, b) < 0 },

		// String helpers
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"title":     cases.Title(language.English).String,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"replace":   strings.ReplaceAll,
		"split":     strings.Split,
		"trim":      strings.TrimSpace,
		"join":      strings.Join,

		// Slice builder
		"slice": func(args ...string) []string { return args },

		// Metadata filter (exclude fields and remove empty-ish values)
		"filterMetadata": func(metadata map[string]interface{}, excludeFields []string) map[string]interface{} {
			if len(metadata) == 0 {
				return map[string]interface{}{}
			}
			ex := make(map[string]struct{}, len(excludeFields))
			for _, f := range excludeFields {
				ex[f] = struct{}{}
			}
			out := make(map[string]interface{})
			for k, v := range metadata {
				if _, skip := ex[k]; skip {
					continue
				}
				if v == nil {
					continue
				}
				if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
					continue
				}
				if s := fmt.Sprintf("%v", v); s == "0" || s == "" {
					continue
				}
				out[k] = v
			}
			return out
		},
	}
}

// compareNumbers compares two numeric-ish values returning -1 / 0 / 1.
func compareNumbers(a, b interface{}) int {
	av := getNumericValue(a)
	bv := getNumericValue(b)
	switch {
	case av < bv:
		return -1
	case av > bv:
		return 1
	default:
		return 0
	}
}

// getNumericValue extracts a float64 value for comparison; non-numeric -> 0.
func getNumericValue(v interface{}) float64 {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int8:
		return float64(t)
	case int16:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case uint:
		return float64(t)
	case uint8:
		return float64(t)
	case uint16:
		return float64(t)
	case uint32:
		return float64(t)
	case uint64:
		return float64(t)
	case float32:
		return float64(t)
	case float64:
		return t
	default:
		return 0
	}
}
