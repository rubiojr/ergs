package core

import (
	"fmt"
	"strings"
)

// FormatMetadata formats a metadata map into a pretty-printed string
func FormatMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}

	var metadataInfo strings.Builder
	metadataInfo.WriteString("\n  Metadata:")
	for key, value := range metadata {
		switch v := value.(type) {
		case string:
			if len(v) > 100 {
				v = v[:97] + "..."
			}
			fmt.Fprintf(&metadataInfo, "\n    %s: %s", key, v)
		case bool:
			fmt.Fprintf(&metadataInfo, "\n    %s: %v", key, v)
		case int, int64, float64:
			fmt.Fprintf(&metadataInfo, "\n    %s: %v", key, v)
		default:
			valueStr := fmt.Sprintf("%v", v)
			if len(valueStr) > 100 {
				valueStr = valueStr[:97] + "..."
			}
			fmt.Fprintf(&metadataInfo, "\n    %s: %s", key, valueStr)
		}
	}

	return metadataInfo.String()
}
