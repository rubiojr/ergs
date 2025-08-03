package hackernews

import (
	"fmt"
	"html"
	"strings"
	"time"
	"unicode"

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

func (i *ItemBlock) Type() string {
	return "hackernews"
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
	var lines []string

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

	// Main title line with item type
	var titleLine string
	if i.title != "" {
		titleLine = fmt.Sprintf("%s %s", emoji, i.title)
	} else {
		titleLine = fmt.Sprintf("%s HackerNews %s", emoji, title(i.itemType))
	}
	lines = append(lines, titleLine)

	// URL line (for stories with links)
	if i.url != "" {
		lines = append(lines, fmt.Sprintf("   🔗 %s", i.url))
	}

	// Build metadata line
	var metaLine []string
	if i.author != "" {
		metaLine = append(metaLine, fmt.Sprintf("by %s", i.author))
	}

	// Add score and comments for stories/polls
	if i.itemType == "story" || i.itemType == "poll" {
		if i.score > 0 {
			metaLine = append(metaLine, fmt.Sprintf("⬆️ %d", i.score))
		}
		if i.descendants > 0 {
			metaLine = append(metaLine, fmt.Sprintf("💬 %d", i.descendants))
		}
	}

	// Add timestamp
	timeStr := i.createdAt.Format("2006-01-02 15:04")
	metaLine = append(metaLine, timeStr)

	if len(metaLine) > 0 {
		lines = append(lines, fmt.Sprintf("   %s", strings.Join(metaLine, " • ")))
	}

	// Parent reference (for comments and poll options)
	if i.parentID > 0 {
		lines = append(lines, fmt.Sprintf("   ↳ Reply to hn-%d", i.parentID))
	}

	// Item text content with better formatting
	if i.itemText != "" {
		decodedText := i.cleanHTML(i.itemText)

		// Truncate if too long
		if len(decodedText) > 300 {
			decodedText = decodedText[:300] + "..."
		}

		decodedText = strings.TrimSpace(decodedText)
		if decodedText != "" {
			lines = append(lines, "") // Add spacing

			// Split into paragraphs and indent
			paragraphs := strings.Split(decodedText, "\n\n")
			for _, para := range paragraphs {
				para = strings.TrimSpace(para)
				if para != "" {
					// Wrap long lines
					wrapped := i.wrapText(para, 80)
					for _, line := range wrapped {
						lines = append(lines, fmt.Sprintf("   %s", line))
					}
				}
			}
		}
	}

	// Add ID as footer
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("   ID: %s", i.id))

	return strings.Join(lines, "\n")
}

// cleanHTML removes HTML tags and decodes entities
func (i *ItemBlock) cleanHTML(text string) string {
	// Decode HTML entities
	decoded := html.UnescapeString(text)

	// Replace common HTML tags with formatting
	decoded = strings.ReplaceAll(decoded, "<p>", "\n\n")
	decoded = strings.ReplaceAll(decoded, "</p>", "")
	decoded = strings.ReplaceAll(decoded, "<br>", "\n")
	decoded = strings.ReplaceAll(decoded, "<br/>", "\n")
	decoded = strings.ReplaceAll(decoded, "<br />", "\n")
	decoded = strings.ReplaceAll(decoded, "<i>", "_")
	decoded = strings.ReplaceAll(decoded, "</i>", "_")
	decoded = strings.ReplaceAll(decoded, "<em>", "_")
	decoded = strings.ReplaceAll(decoded, "</em>", "_")
	decoded = strings.ReplaceAll(decoded, "<b>", "**")
	decoded = strings.ReplaceAll(decoded, "</b>", "**")
	decoded = strings.ReplaceAll(decoded, "<strong>", "**")
	decoded = strings.ReplaceAll(decoded, "</strong>", "**")
	decoded = strings.ReplaceAll(decoded, "<pre>", "\n```\n")
	decoded = strings.ReplaceAll(decoded, "</pre>", "\n```\n")
	decoded = strings.ReplaceAll(decoded, "<code>", "`")
	decoded = strings.ReplaceAll(decoded, "</code>", "`")

	// Remove any remaining HTML tags
	for strings.Contains(decoded, "<") && strings.Contains(decoded, ">") {
		start := strings.Index(decoded, "<")
		end := strings.Index(decoded[start:], ">")
		if end > 0 {
			decoded = decoded[:start] + decoded[start+end+1:]
		} else {
			break
		}
	}

	return decoded
}

// wrapText wraps text to specified width
func (i *ItemBlock) wrapText(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{}
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		// If adding this word would exceed width, start new line
		if currentLine.Len() > 0 && currentLine.Len()+len(word)+1 > width {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
		}

		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

// Summary returns a concise one-line summary of the HackerNews item.
func (i *ItemBlock) Summary() string {
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

	title := i.title
	if title == "" {
		title = "Untitled"
	}

	return fmt.Sprintf("%s %s", emoji, title)
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

// title capitalizes the first letter of a string (replacement for deprecated strings.Title)
func title(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
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
