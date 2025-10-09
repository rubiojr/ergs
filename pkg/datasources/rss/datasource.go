package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("rss", prototype)
}

type Config struct {
	URLs     []string `toml:"urls"`
	MaxItems int      `toml:"max_items"`
}

func (c *Config) Validate() error {
	if len(c.URLs) == 0 {
		return fmt.Errorf("at least one RSS URL must be configured")
	}
	if c.MaxItems <= 0 {
		c.MaxItems = 50
	}
	if c.MaxItems > 200 {
		c.MaxItems = 200
	}
	return nil
}

type Datasource struct {
	config       *Config
	client       *http.Client
	instanceName string
}

// RSS Feed structures
type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Channel Channel  `xml:"channel"`
}

type Atom struct {
	XMLName xml.Name   `xml:"feed"`
	Title   string     `xml:"title"`
	Entries []AtomItem `xml:"entry"`
}

type Channel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []RSSItem `xml:"item"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
	Author      string `xml:"author"`
	Category    string `xml:"category"`
}

type AtomItem struct {
	Title   string     `xml:"title"`
	Link    AtomLink   `xml:"link"`
	Summary string     `xml:"summary"`
	Content string     `xml:"content"`
	ID      string     `xml:"id"`
	Updated string     `xml:"updated"`
	Author  AtomAuthor `xml:"author"`
}

type AtomLink struct {
	Href string `xml:"href,attr"`
}

type AtomAuthor struct {
	Name string `xml:"name"`
}

func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var rssConfig *Config
	if config == nil {
		rssConfig = &Config{
			URLs: []string{
				"https://feeds.arstechnica.com/arstechnica/index",
				"https://www.phoronix.com/rss.php",
			},
			MaxItems: 50,
		}
	} else {
		var ok bool
		rssConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for RSS datasource")
		}
	}

	if err := rssConfig.Validate(); err != nil {
		return nil, err
	}

	return &Datasource{
		config:       rssConfig,
		client:       &http.Client{Timeout: 30 * time.Second},
		instanceName: instanceName,
	}, nil
}

func (d *Datasource) Type() string {
	return "rss"
}

func (d *Datasource) Name() string {
	return d.instanceName
}

func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"feed_title":  "TEXT",
		"feed_url":    "TEXT",
		"title":       "TEXT",
		"link":        "TEXT",
		"description": "TEXT",
		"author":      "TEXT",
		"category":    "TEXT",
		"guid":        "TEXT",
		"published":   "TEXT",
	}
}

func (d *Datasource) BlockPrototype() core.Block {
	return &ItemBlock{}
}

func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

func (d *Datasource) SetConfig(config interface{}) error {
	if cfg, ok := config.(*Config); ok {
		if err := cfg.Validate(); err != nil {
			return err
		}
		d.config = cfg
		return nil
	}
	return fmt.Errorf("invalid config type for RSS datasource")
}

func (d *Datasource) GetConfig() interface{} {
	return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	log.Printf("Fetching RSS feeds from %d URLs", len(d.config.URLs))

	totalFetched := 0
	itemsPerFeed := d.config.MaxItems / len(d.config.URLs)
	if itemsPerFeed == 0 {
		itemsPerFeed = 1
	}

	for _, feedURL := range d.config.URLs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("Fetching RSS feed: %s", feedURL)

		items, feedTitle, err := d.fetchFeed(ctx, feedURL)
		if err != nil {
			log.Printf("Failed to fetch feed %s: %v", feedURL, err)
			continue
		}

		log.Printf("Fetched %d items from %s", len(items), feedURL)

		// Limit items per feed
		if len(items) > itemsPerFeed {
			items = items[:itemsPerFeed]
		}

		for _, item := range items {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			block := d.convertItemToBlock(item, feedTitle, feedURL)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case blockCh <- block:
				totalFetched++
			}
		}

		// Add small delay between feeds to be respectful
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Fetched %d RSS items total", totalFetched)
	return nil
}

func (d *Datasource) fetchFeed(ctx context.Context, feedURL string) ([]FeedItem, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("User-Agent", "ergs/1.0 RSS Reader")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Try to parse as RSS first
	decoder := xml.NewDecoder(resp.Body)
	var rss RSS
	if err := decoder.Decode(&rss); err != nil {
		// Reset and try Atom
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
		req, err = http.NewRequestWithContext(ctx, "GET", feedURL, nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("User-Agent", "ergs/1.0 RSS Reader")

		resp, err = d.client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				fmt.Printf("Warning: failed to close response body: %v\n", err)
			}
		}()

		decoder = xml.NewDecoder(resp.Body)
		var atom Atom
		if err := decoder.Decode(&atom); err != nil {
			return nil, "", fmt.Errorf("failed to parse as RSS or Atom: %w", err)
		}

		// Convert Atom to common format
		items := make([]FeedItem, len(atom.Entries))
		for i, entry := range atom.Entries {
			items[i] = FeedItem{
				Title:       entry.Title,
				Link:        entry.Link.Href,
				Description: entry.Summary,
				Content:     entry.Content,
				PubDate:     entry.Updated,
				GUID:        entry.ID,
				Author:      entry.Author.Name,
			}
		}
		return items, atom.Title, nil
	}

	// Convert RSS to common format
	items := make([]FeedItem, len(rss.Channel.Items))
	for i, item := range rss.Channel.Items {
		items[i] = FeedItem{
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			PubDate:     item.PubDate,
			GUID:        item.GUID,
			Author:      item.Author,
			Category:    item.Category,
		}
	}

	return items, rss.Channel.Title, nil
}

// Common feed item structure
type FeedItem struct {
	Title       string
	Link        string
	Description string
	Content     string
	PubDate     string
	GUID        string
	Author      string
	Category    string
}

func (d *Datasource) convertItemToBlock(item FeedItem, feedTitle, feedURL string) core.Block {
	// Create unique ID
	// Build an ID namespaced by the datasource instance name so multiple RSS datasources
	// (e.g. rss, rss_teammates) don't collide when a feed item GUID/link is the same.
	rawID := item.GUID
	if rawID == "" {
		rawID = item.Link
	}
	// Fall back to current time if both GUID and Link are empty (very rare / malformed feed)
	if rawID == "" {
		rawID = time.Now().UTC().Format(time.RFC3339Nano)
	}
	itemID := fmt.Sprintf("%s-%s", d.instanceName, rawID)

	// Parse publish date
	var createdAt time.Time
	if item.PubDate != "" {
		// Try common date formats
		formats := []string{
			time.RFC1123Z, // RSS standard
			time.RFC1123,  // RSS without timezone
			time.RFC3339,  // Atom standard
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05",
			"Mon, 02 Jan 2006 15:04:05 -0700",
		}

		for _, format := range formats {
			if t, err := time.Parse(format, item.PubDate); err == nil {
				createdAt = t.UTC()
				break
			}
		}
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	// Clean up description/content for search text
	description := item.Description
	if item.Content != "" && len(item.Content) > len(description) {
		description = item.Content
	}

	// Decode HTML entities and remove basic HTML tags
	description = html.UnescapeString(description)
	description = strings.ReplaceAll(description, "<br>", " ")
	description = strings.ReplaceAll(description, "<br/>", " ")
	description = strings.ReplaceAll(description, "<p>", " ")
	description = strings.ReplaceAll(description, "</p>", " ")
	// Simple tag removal (not comprehensive but covers basics)
	for strings.Contains(description, "<") && strings.Contains(description, ">") {
		start := strings.Index(description, "<")
		end := strings.Index(description[start:], ">")
		if end != -1 {
			description = description[:start] + " " + description[start+end+1:]
		} else {
			break
		}
	}
	description = strings.TrimSpace(description)

	// Build search text
	text := fmt.Sprintf("title=%s description=%s author=%s category=%s feed=%s url=%s",
		item.Title, description, item.Author, item.Category, feedTitle, item.Link)

	metadata := map[string]interface{}{
		"feed_title":  feedTitle,
		"feed_url":    feedURL,
		"title":       item.Title,
		"link":        item.Link,
		"description": description,
		"author":      item.Author,
		"category":    item.Category,
		"guid":        item.GUID,
		"published":   item.PubDate,
	}

	return NewItemBlock(
		itemID,
		text,
		createdAt,
		d.instanceName, // use instance name (was "rss") so each configured RSS datasource stores in its own DB
		metadata,
		feedTitle,
		feedURL,
		item.Title,
		item.Link,
		description,
		item.Author,
		item.Category,
		item.GUID,
		item.PubDate,
	)
}

func (d *Datasource) Close() error {
	return nil
}

func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
