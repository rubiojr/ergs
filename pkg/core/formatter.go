package core

import "fmt"

// FormatMetadata formats a metadata map into a pretty-printed string
func FormatMetadata(metadata map[string]interface{}) string {
	if len(metadata) == 0 {
		return ""
	}

	metadataInfo := "\n  Metadata:"
	for key, value := range metadata {
		switch v := value.(type) {
		case string:
			if len(v) > 100 {
				v = v[:97] + "..."
			}
			metadataInfo += fmt.Sprintf("\n    %s: %s", key, v)
		case bool:
			metadataInfo += fmt.Sprintf("\n    %s: %v", key, v)
		case int, int64, float64:
			metadataInfo += fmt.Sprintf("\n    %s: %v", key, v)
		default:
			valueStr := fmt.Sprintf("%v", v)
			if len(valueStr) > 100 {
				valueStr = valueStr[:97] + "..."
			}
			metadataInfo += fmt.Sprintf("\n    %s: %s", key, valueStr)
		}
	}

	return metadataInfo
}
