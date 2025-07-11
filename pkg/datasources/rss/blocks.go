package rss

import (
	"fmt"
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
	feedTitle   string
	feedURL     string
	title       string
	link        string
	description string
	author      string
	category    string
	guid        string
	published   string
}

func NewItemBlock(id, text string, createdAt time.Time, source string, metadata map[string]interface{},
	feedTitle, feedURL, title, link, description, author, category, guid, published string) *ItemBlock {
	return &ItemBlock{
		id:          id,
		text:        text,
		createdAt:   createdAt,
		source:      source,
		metadata:    metadata,
		feedTitle:   feedTitle,
		feedURL:     feedURL,
		title:       title,
		link:        link,
		description: description,
		author:      author,
		category:    category,
		guid:        guid,
		published:   published,
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

func (i *ItemBlock) FeedTitle() string {
	return i.feedTitle
}

func (i *ItemBlock) FeedURL() string {
	return i.feedURL
}

func (i *ItemBlock) Title() string {
	return i.title
}

func (i *ItemBlock) Link() string {
	return i.link
}

func (i *ItemBlock) Description() string {
	return i.description
}

func (i *ItemBlock) Author() string {
	return i.author
}

func (i *ItemBlock) Category() string {
	return i.category
}

func (i *ItemBlock) GUID() string {
	return i.guid
}

func (i *ItemBlock) Published() string {
	return i.published
}

func (i *ItemBlock) PrettyText() string {
	var parts []string

	// Main title line with RSS icon
	mainLine := fmt.Sprintf("📰 RSS: %s", i.title)
	parts = append(parts, mainLine)

	// ID and time
	parts = append(parts, fmt.Sprintf("  ID: %s", i.id))
	parts = append(parts, fmt.Sprintf("  Time: %s", i.createdAt.Format("2006-01-02 15:04:05")))

	// Feed information
	if i.feedTitle != "" {
		parts = append(parts, fmt.Sprintf("  Feed: %s", i.feedTitle))
	}
	if i.feedURL != "" {
		parts = append(parts, fmt.Sprintf("  Feed URL: %s", i.feedURL))
	}

	// Article link
	if i.link != "" {
		parts = append(parts, fmt.Sprintf("  Link: %s", i.link))
	}

	// Author and category
	if i.author != "" {
		parts = append(parts, fmt.Sprintf("  Author: %s", i.author))
	}
	if i.category != "" {
		parts = append(parts, fmt.Sprintf("  Category: %s", i.category))
	}

	// Description (truncated for readability)
	if i.description != "" {
		desc := i.description
		if len(desc) > 300 {
			desc = desc[:300] + "..."
		}

		// Clean up whitespace and format nicely
		lines := strings.Split(desc, "\n")
		var cleanLines []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				cleanLines = append(cleanLines, "    "+line)
			}
		}
		if len(cleanLines) > 0 {
			parts = append(parts, "  Description:")
			parts = append(parts, cleanLines...)
		}
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
	feedTitle := getStringFromMetadata(metadata, "feed_title", "")
	feedURL := getStringFromMetadata(metadata, "feed_url", "")
	title := getStringFromMetadata(metadata, "title", "")
	link := getStringFromMetadata(metadata, "link", "")
	description := getStringFromMetadata(metadata, "description", "")
	author := getStringFromMetadata(metadata, "author", "")
	category := getStringFromMetadata(metadata, "category", "")
	guid := getStringFromMetadata(metadata, "guid", "")
	published := getStringFromMetadata(metadata, "published", "")

	return &ItemBlock{
		id:          genericBlock.ID(),
		text:        genericBlock.Text(),
		createdAt:   genericBlock.CreatedAt(),
		source:      source,
		metadata:    metadata,
		feedTitle:   feedTitle,
		feedURL:     feedURL,
		title:       title,
		link:        link,
		description: description,
		author:      author,
		category:    category,
		guid:        guid,
		published:   published,
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
