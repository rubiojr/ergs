package chromium

import (
	"fmt"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

type VisitBlock struct {
	id        string
	text      string
	createdAt time.Time
	source    string
	metadata  map[string]interface{}
	url       string
	title     string
	visitDate time.Time
}

func NewVisitBlock(id, url, title string, visitDate time.Time) *VisitBlock {
	text := fmt.Sprintf("url=%s title=%s", url, title)

	metadata := map[string]interface{}{
		"url":        url,
		"title":      title,
		"visit_date": visitDate.Format("2006-01-02 15:04:05"),
		"source":     "chromium",
	}

	return &VisitBlock{
		id:        id,
		text:      text,
		createdAt: visitDate,
		source:    "chromium",
		metadata:  metadata,
		url:       url,
		title:     title,
		visitDate: visitDate,
	}
}

func NewVisitBlockWithSource(id, url, title string, visitDate time.Time, source string) *VisitBlock {
	text := fmt.Sprintf("url=%s title=%s", url, title)

	metadata := map[string]interface{}{
		"url":        url,
		"title":      title,
		"visit_date": visitDate.Format("2006-01-02 15:04:05"),
		"source":     source,
	}

	return &VisitBlock{
		id:        id,
		text:      text,
		createdAt: visitDate,
		source:    source,
		metadata:  metadata,
		url:       url,
		title:     title,
		visitDate: visitDate,
	}
}

func (v *VisitBlock) ID() string {
	return v.id
}

func (v *VisitBlock) Text() string {
	return v.text
}

func (v *VisitBlock) CreatedAt() time.Time {
	return v.createdAt
}

func (v *VisitBlock) Source() string {
	return v.source
}

func (v *VisitBlock) Metadata() map[string]interface{} {
	return v.metadata
}

func (v *VisitBlock) Type() string {
	return "chromium"
}

func (v *VisitBlock) URL() string {
	return v.url
}

func (v *VisitBlock) Title() string {
	return v.title
}

func (v *VisitBlock) VisitDate() time.Time {
	return v.visitDate
}

func (v *VisitBlock) PrettyText() string {
	titleInfo := ""
	if v.title != "" {
		titleInfo = fmt.Sprintf("\n  Title: %s", v.title)
	}

	metadataInfo := core.FormatMetadata(v.metadata)

	return fmt.Sprintf("🌐 Chromium Visit\n  ID: %s\n  Time: %s\n  URL: %s%s%s",
		v.id, v.visitDate.Format("2006-01-02 15:04:05"), v.url, titleInfo, metadataInfo)
}

// Summary returns a concise one-line summary of the Chromium visit.
func (v *VisitBlock) Summary() string {
	title := v.title
	if title == "" {
		title = v.url
	}
	return fmt.Sprintf("🌐 %s", title)
}

// Factory creates a new VisitBlock from a GenericBlock and source.
// This method is part of the core.Block interface and enables reconstruction
// from database data without requiring separate factory objects.
func (v *VisitBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	metadata := genericBlock.Metadata()
	url := getStringFromMetadata(metadata, "url", "")
	title := getStringFromMetadata(metadata, "title", "")
	visitDate := genericBlock.CreatedAt()

	return NewVisitBlockWithSource(
		genericBlock.ID(),
		url,
		title,
		visitDate,
		source,
	)
}

// Helper function for safe metadata extraction
func getStringFromMetadata(metadata map[string]interface{}, key, defaultValue string) string {
	if value, exists := metadata[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}
