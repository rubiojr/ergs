package hackernews

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

type ItemBlock struct {
	id          string
	text        string
	createdAt   time.Time
	source      string
	metadata    map[string]interface{}
	itemType    string
	title       string
	itemText    string
	url         string
	author      string
	score       int
	descendants int
	parentID    int
	pollID      int
	deleted     bool
	dead        bool
}

func NewItemBlock(id, text string, createdAt time.Time, source string, metadata map[string]interface{},
	itemType, title, itemText, url, author string, score, descendants, parentID, pollID int, deleted, dead bool) *ItemBlock {
	return &ItemBlock{
		id:          id,
		text:        text,
		createdAt:   createdAt,
		source:      source,
		metadata:    metadata,
		itemType:    itemType,
		title:       title,
		itemText:    itemText,
		url:         url,
		author:      author,
		score:       score,
		descendants: descendants,
		parentID:    parentID,
		pollID:      pollID,
		deleted:     deleted,
		dead:        dead,
	}
}

func (i *ItemBlock) ID() string {
	return i.id
}

func (i *ItemBlock) Text() string {
	return i.text
}

func (i *ItemBlock) CreatedAt() time.Time {
	return i.createdAt
}

func (i *ItemBlock) Source() string {
	return i.source
}

func (i *ItemBlock) Metadata() map[string]interface{} {
	return i.metadata
}

func (i *ItemBlock) ItemType() string {
	return i.itemType
}

func (i *ItemBlock) Title() string {
	return i.title
}

func (i *ItemBlock) ItemText() string {
	return i.itemText
}

func (i *ItemBlock) URL() string {
	return i.url
}

func (i *ItemBlock) Author() string {
	return i.author
}

func (i *ItemBlock) Score() int {
	return i.score
}

func (i *ItemBlock) Descendants() int {
	return i.descendants
}

func (i *ItemBlock) ParentID() int {
	return i.parentID
}

func (i *ItemBlock) PollID() int {
	return i.pollID
}

func (i *ItemBlock) IsDeleted() bool {
	return i.deleted
}

func (i *ItemBlock) IsDead() bool {
	return i.dead
}

func (i *ItemBlock) PrettyText() string {
	var parts []string

	// Item type emoji
	var emoji string
	switch i.itemType {
	case "story":
		emoji = "📰"
	case "comment":
		emoji = "💬"
	case "job":
		emoji = "💼"
	case "poll":
		emoji = "📊"
	case "pollopt":
		emoji = "☑️"
	default:
		emoji = "📄"
	}

	// Main title line
	mainLine := fmt.Sprintf("%s HackerNews %s", emoji, strings.Title(i.itemType))
	if i.author != "" {
		mainLine += fmt.Sprintf(" by %s", i.author)
	}
	parts = append(parts, mainLine)

	// ID and time
	parts = append(parts, fmt.Sprintf("  ID: %s", i.id))
	parts = append(parts, fmt.Sprintf("  Time: %s", i.createdAt.Format("2006-01-02 15:04:05")))

	// Title (for stories, jobs, polls)
	if i.title != "" {
		parts = append(parts, fmt.Sprintf("  Title: %s", i.title))
	}

	// URL (for stories with links)
	if i.url != "" {
		parts = append(parts, fmt.Sprintf("  URL: %s", i.url))
	}

	// Text content (HTML decoded and truncated for readability)
	if i.itemText != "" {
		decodedText := html.UnescapeString(i.itemText)
		// Remove HTML tags for display
		decodedText = strings.ReplaceAll(decodedText, "<p>", "\n")
		decodedText = strings.ReplaceAll(decodedText, "</p>", "")
		decodedText = strings.ReplaceAll(decodedText, "<br>", "\n")
		decodedText = strings.ReplaceAll(decodedText, "<i>", "")
		decodedText = strings.ReplaceAll(decodedText, "</i>", "")
		decodedText = strings.ReplaceAll(decodedText, "<pre>", "\n")
		decodedText = strings.ReplaceAll(decodedText, "</pre>", "\n")
		decodedText = strings.ReplaceAll(decodedText, "<code>", "")
		decodedText = strings.ReplaceAll(decodedText, "</code>", "")

		// Truncate long text
		if len(decodedText) > 200 {
			decodedText = decodedText[:200] + "..."
		}

		// Clean up whitespace
		decodedText = strings.TrimSpace(decodedText)
		lines := strings.Split(decodedText, "\n")
		var cleanLines []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				cleanLines = append(cleanLines, "    "+line)
			}
		}
		if len(cleanLines) > 0 {
			parts = append(parts, "  Text:")
			parts = append(parts, cleanLines...)
		}
	}

	// Score and comments (for stories and polls)
	if i.itemType == "story" || i.itemType == "poll" {
		scoreLine := fmt.Sprintf("  Score: %d", i.score)
		if i.descendants > 0 {
			scoreLine += fmt.Sprintf(", Comments: %d", i.descendants)
		}
		parts = append(parts, scoreLine)
	}

	// Parent reference (for comments and poll options)
	if i.parentID > 0 {
		parts = append(parts, fmt.Sprintf("  Parent: hn-%d", i.parentID))
	}

	// Poll reference (for poll options)
	if i.pollID > 0 {
		parts = append(parts, fmt.Sprintf("  Poll: hn-%d", i.pollID))
	}

	// Format metadata using utility function
	metadataInfo := core.FormatMetadata(i.metadata)
	if metadataInfo != "" {
		parts = append(parts, metadataInfo)
	}

	return strings.Join(parts, "\n")
}

// Factory creates a new ItemBlock from a GenericBlock and source.
// This method is part of the core.Block interface and enables reconstruction
// from database data without requiring separate factory objects.
func (i *ItemBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	metadata := genericBlock.Metadata()
	itemType := getStringFromMetadata(metadata, "item_type", "story")
	title := getStringFromMetadata(metadata, "title", "")
	url := getStringFromMetadata(metadata, "url", "")
	author := getStringFromMetadata(metadata, "author", "")
	score := getIntFromMetadata(metadata, "score", 0)
	descendants := getIntFromMetadata(metadata, "descendants", 0)
	parentID := getIntFromMetadata(metadata, "parent_id", 0)
	pollID := getIntFromMetadata(metadata, "poll_id", 0)
	deleted := getBoolFromMetadata(metadata, "deleted", false)
	dead := getBoolFromMetadata(metadata, "dead", false)

	return &ItemBlock{
		id:          genericBlock.ID(),
		text:        genericBlock.Text(),
		createdAt:   genericBlock.CreatedAt(),
		source:      source,
		metadata:    metadata,
		itemType:    itemType,
		title:       title,
		itemText:    "", // Item text not stored in search text, would need to extract
		url:         url,
		author:      author,
		score:       score,
		descendants: descendants,
		parentID:    parentID,
		pollID:      pollID,
		deleted:     deleted,
		dead:        dead,
	}
}

// Helper functions for safe metadata extraction
func getStringFromMetadata(metadata map[string]interface{}, key, defaultValue string) string {
	if value, exists := metadata[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

func getIntFromMetadata(metadata map[string]interface{}, key string, defaultValue int) int {
	if value, exists := metadata[key]; exists {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return defaultValue
}

func getBoolFromMetadata(metadata map[string]interface{}, key string, defaultValue bool) bool {
	if value, exists := metadata[key]; exists {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return defaultValue
}
