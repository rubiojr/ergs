package components

import (
	"strings"

	"github.com/rubiojr/ergs/cmd/web/components/types"
)

// ExtractDatasource attempts to determine the datasource name for a given
// web block. Firehose and unified block rendering rely on showing the
// originating datasource even when working from a flattened global slice.
//
// It looks for a "datasource" entry in the block's metadata map and returns
// it if it's a non-empty string. Otherwise an empty string is returned.
//
// This helper centralizes the logic so the templ file can call it directly
// (reducing inline template logic that previously triggered unused variable
// artifacts in generated code).
func ExtractDatasource(block types.WebBlock) string {
	if block.Metadata == nil {
		return ""
	}
	raw, ok := block.Metadata["datasource"]
	if !ok || raw == nil {
		return ""
	}
	// Accept common types, most often string
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		// Fallback: attempt stringification
		s := strings.TrimSpace(toStringApprox(v))
		return s
	}
}

// toStringApprox provides a conservative string conversion for non-string
// metadata values. We intentionally avoid fmt.Sprintf("%v", ...) for very large
// or complex structures to reduce unexpected allocations or leaking internal
// representations. Extend minimally as needed.
func toStringApprox(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		// Last resort – keep it simple; if this becomes noisy we can tighten.
		return ""
	}
}

// EnsureDatasourcePresent mutates the provided block's metadata map to include
// the datasource key if missing and returns whether it was added. This can be
// used before rendering to guarantee consistent template expectations.
func EnsureDatasourcePresent(block *types.WebBlock, datasource string) bool {
	if block == nil {
		return false
	}
	if block.Metadata == nil {
		block.Metadata = make(map[string]interface{}, 1)
	}
	if _, exists := block.Metadata["datasource"]; !exists && datasource != "" {
		block.Metadata["datasource"] = datasource
		return true
	}
	return false
}
