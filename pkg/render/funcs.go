package render

import (
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
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
	Render(block core.Block) template.HTML
	CanRender(block core.Block) bool
	GetDatasourceType() string
}

type TemplateData struct {
	Block    core.Block
	Metadata map[string]interface{}
	Links    []string
}

var globalRenderers []BlockRenderer

func RegisterRenderer(renderer BlockRenderer) {
	if renderer == nil {
		return
	}
	globalRenderers = append(globalRenderers, renderer)
}

func GetRegisteredRenderers() []BlockRenderer {
	out := make([]BlockRenderer, len(globalRenderers))
	copy(out, globalRenderers)
	return out
}

func ExtractLinks(text string) []string {
	var links []string
	words := strings.Fields(text)
	for _, w := range words {
		if strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://") {
			clean := strings.TrimRight(w, ".,!?;:)]}")
			links = append(links, clean)
		}
	}
	return links
}

func FormatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		m := int(diff.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case diff < 24*time.Hour:
		h := int(diff.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case diff < 7*24*time.Hour:
		d := int(diff.Hours() / 24)
		if d == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", d)
	default:
		return t.Format("Jan 2, 2006")
	}
}

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
			if v, ok := val.(string); ok && v == "" {
				return def
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
					if strings.TrimSpace(v) == "" {
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
					if strings.TrimSpace(v) != "" {
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
		"slice":     func(args ...string) []string { return args },

		// Metadata filter
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

		// Pretty JSON / structural helpers
		"prettyJSON": func(v interface{}) string {
			var data interface{}
			switch t := v.(type) {
			case string:
				if err := json.Unmarshal([]byte(t), &data); err != nil {
					return t
				}
			default:
				data = t
			}
			b, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				return fmt.Sprintf("%v", v)
			}
			return string(b)
		},
		"isMap": func(v interface{}) bool {
			if v == nil {
				return false
			}
			_, ok := v.(map[string]interface{})
			return ok
		},
		"isSlice": func(v interface{}) bool {
			if v == nil {
				return false
			}
			_, ok := v.([]interface{})
			return ok
		},
		"asMap": func(v interface{}) map[string]interface{} {
			if m, ok := v.(map[string]interface{}); ok {
				return m
			}
			return map[string]interface{}{}
		},
		"asSlice": func(v interface{}) []interface{} {
			if s, ok := v.([]interface{}); ok {
				return s
			}
			return []interface{}{}
		},
		"sortedKeys": func(m map[string]interface{}) []string {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return keys
		},

		// ---- State / numeric diff helpers for Home Assistant renderer ----
		"isNumber": func(v interface{}) bool {
			switch v.(type) {
			case int, int8, int16, int32, int64,
				uint, uint8, uint16, uint32, uint64,
				float32, float64:
				return true
			case string:
				if _, err := parseFloatLoose(v); err == nil {
					return true
				}
				return false
			default:
				return false
			}
		},
		"toFloat": func(v interface{}) float64 {
			f, _ := parseFloatLoose(v)
			return f
		},
		"stateDiff": func(oldV, newV interface{}) string {
			oldS := fmt.Sprintf("%v", oldV)
			newS := fmt.Sprintf("%v", newV)
			if oldS == "" {
				return newS
			}
			if oldS == newS {
				return newS
			}
			return fmt.Sprintf("%s → %s", oldS, newS)
		},
		"stateChangeClass": func(oldV, newV interface{}) string {
			fOld, errOld := parseFloatLoose(oldV)
			fNew, errNew := parseFloatLoose(newV)
			if errOld != nil || errNew != nil {
				// Non-numeric: equality vs change
				if fmt.Sprintf("%v", oldV) == fmt.Sprintf("%v", newV) {
					return "ha-diff-same"
				}
				return "ha-diff-changed"
			}
			switch {
			case fNew > fOld:
				return "ha-diff-up"
			case fNew < fOld:
				return "ha-diff-down"
			default:
				return "ha-diff-same"
			}
		},
		"stateIcon": func(domain, state string) string {
			s := strings.ToLower(state)
			switch strings.ToLower(domain) {
			case "light":
				if s == "on" {
					return "💡"
				}
				return "⚪"
			case "switch":
				if s == "on" {
					return "🔘"
				}
				return "⚪"
			case "binary_sensor":
				if s == "on" || s == "detected" || s == "motion" {
					return "🟢"
				}
				return "⚫"
			case "climate":
				return "🌡️"
			case "alarm_control_panel":
				if s == "triggered" {
					return "🚨"
				}
				if s == "armed_away" || s == "armed_home" {
					return "🔒"
				}
				return "🔓"
			}
			// Generic on/off fallback
			if s == "on" {
				return "🟢"
			}
			if s == "off" {
				return "⚫"
			}
			return ""
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
	case string:
		f, _ := parseFloatLoose(t)
		return f
	default:
		return 0
	}
}

// parseFloatLoose tries to parse numerics from interface or string (including strings with units stripped).
func parseFloatLoose(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint16:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case string:
		s := strings.TrimSpace(val)
		// Strip common unit suffixes (very lightweight heuristic)
		s = strings.TrimRight(s, "°CFfwW%")
		if f, err := json.Number(s).Float64(); err == nil {
			return f, nil
		}
		// Fallback manual parse
		var f float64
		_, err := fmt.Sscanf(s, "%f", &f)
		return f, err
	default:
		return 0, fmt.Errorf("not numeric")
	}
}
