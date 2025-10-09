package hackernews

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("hackernews", prototype)
}

const (
	baseURL = "https://hacker-news.firebaseio.com/v0"
)

type Config struct {
	FetchTop      bool `toml:"fetch_top"`
	FetchNew      bool `toml:"fetch_new"`
	FetchAsk      bool `toml:"fetch_ask"`
	FetchShow     bool `toml:"fetch_show"`
	FetchJobs     bool `toml:"fetch_jobs"`
	MaxItems      int  `toml:"max_items"`
	FetchComments bool `toml:"fetch_comments"`
}

func (c *Config) Validate() error {
	if c.MaxItems <= 0 {
		c.MaxItems = 100
	}
	if c.MaxItems > 500 {
		c.MaxItems = 500
	}
	return nil
}

type Datasource struct {
	config       *Config
	client       *http.Client
	instanceName string
}

type HNItem struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	Kids        []int  `json:"kids"`
	Parent      int    `json:"parent"`
	Poll        int    `json:"poll"`
	Parts       []int  `json:"parts"`
	Deleted     bool   `json:"deleted"`
	Dead        bool   `json:"dead"`
}

func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var hnConfig *Config
	if config == nil {
		hnConfig = &Config{
			FetchTop:      true,
			FetchNew:      false,
			FetchAsk:      false,
			FetchShow:     false,
			FetchJobs:     false,
			MaxItems:      100,
			FetchComments: false,
		}
	} else {
		var ok bool
		hnConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for HackerNews datasource")
		}
	}

	if err := hnConfig.Validate(); err != nil {
		return nil, err
	}

	return &Datasource{
		config:       hnConfig,
		client:       &http.Client{Timeout: 30 * time.Second},
		instanceName: instanceName,
	}, nil
}

func (d *Datasource) Type() string {
	return "hackernews"
}

func (d *Datasource) Name() string {
	return d.instanceName
}

func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"item_type":   "TEXT",
		"title":       "TEXT",
		"url":         "TEXT",
		"author":      "TEXT",
		"score":       "INTEGER",
		"descendants": "INTEGER",
		"parent_id":   "INTEGER",
		"poll_id":     "INTEGER",
		"deleted":     "BOOLEAN",
		"dead":        "BOOLEAN",
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
	return fmt.Errorf("invalid config type for HackerNews datasource")
}

func (d *Datasource) GetConfig() interface{} {
	return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	log.Printf("Fetching HackerNews items")

	var itemIDs []int
	totalFetched := 0

	if d.config.FetchTop {
		ids, err := d.fetchStoryIDs(ctx, "topstories")
		if err != nil {
			return fmt.Errorf("fetching top stories: %w", err)
		}
		itemIDs = append(itemIDs, ids...)
	}

	if d.config.FetchNew {
		ids, err := d.fetchStoryIDs(ctx, "newstories")
		if err != nil {
			return fmt.Errorf("fetching new stories: %w", err)
		}
		itemIDs = append(itemIDs, ids...)
	}

	if d.config.FetchAsk {
		ids, err := d.fetchStoryIDs(ctx, "askstories")
		if err != nil {
			return fmt.Errorf("fetching ask stories: %w", err)
		}
		itemIDs = append(itemIDs, ids...)
	}

	if d.config.FetchShow {
		ids, err := d.fetchStoryIDs(ctx, "showstories")
		if err != nil {
			return fmt.Errorf("fetching show stories: %w", err)
		}
		itemIDs = append(itemIDs, ids...)
	}

	if d.config.FetchJobs {
		ids, err := d.fetchStoryIDs(ctx, "jobstories")
		if err != nil {
			return fmt.Errorf("fetching job stories: %w", err)
		}
		itemIDs = append(itemIDs, ids...)
	}

	// Remove duplicates and limit to max_items
	itemIDs = d.deduplicateAndLimit(itemIDs)

	log.Printf("Processing %d HackerNews items", len(itemIDs))

	for _, itemID := range itemIDs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		item, err := d.fetchItem(ctx, itemID)
		if err != nil {
			log.Printf("Failed to fetch item %d: %v", itemID, err)
			continue
		}

		if item.Deleted || item.Dead {
			continue
		}

		block := d.convertItemToBlock(item)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case blockCh <- block:
			totalFetched++
		}

		// If fetch_comments is enabled and this item has comments
		if d.config.FetchComments && len(item.Kids) > 0 {
			// Fetch top-level comments (limit to first 5 to avoid too much data)
			commentLimit := 5
			if len(item.Kids) < commentLimit {
				commentLimit = len(item.Kids)
			}

			for i := 0; i < commentLimit; i++ {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				comment, err := d.fetchItem(ctx, item.Kids[i])
				if err != nil {
					log.Printf("Failed to fetch comment %d: %v", item.Kids[i], err)
					continue
				}

				if comment.Deleted || comment.Dead {
					continue
				}

				commentBlock := d.convertItemToBlock(comment)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case blockCh <- commentBlock:
					totalFetched++
				}
			}
		}

		// Add small delay to be respectful to the API
		time.Sleep(50 * time.Millisecond)
	}

	log.Printf("Fetched %d HackerNews items", totalFetched)
	return nil
}

func (d *Datasource) fetchStoryIDs(ctx context.Context, endpoint string) ([]int, error) {
	url := fmt.Sprintf("%s/%s.json", baseURL, endpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, err
	}

	return ids, nil
}

func (d *Datasource) fetchItem(ctx context.Context, itemID int) (*HNItem, error) {
	url := fmt.Sprintf("%s/item/%d.json", baseURL, itemID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var item HNItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, err
	}

	return &item, nil
}

func (d *Datasource) convertItemToBlock(item *HNItem) core.Block {
	itemID := fmt.Sprintf("hn-%d", item.ID)
	createdAt := time.Unix(item.Time, 0).UTC()

	// Build text content for search
	text := ""
	if item.Title != "" {
		text += fmt.Sprintf("title=%s ", item.Title)
	}
	if item.Text != "" {
		text += fmt.Sprintf("text=%s ", item.Text)
	}
	if item.URL != "" {
		text += fmt.Sprintf("url=%s ", item.URL)
	}
	if item.By != "" {
		text += fmt.Sprintf("author=%s ", item.By)
	}
	text += fmt.Sprintf("type=%s score=%d descendants=%d", item.Type, item.Score, item.Descendants)

	metadata := map[string]interface{}{
		"item_type":   item.Type,
		"title":       item.Title,
		"url":         item.URL,
		"author":      item.By,
		"score":       item.Score,
		"descendants": item.Descendants,
		"parent_id":   item.Parent,
		"poll_id":     item.Poll,
		"deleted":     item.Deleted,
		"dead":        item.Dead,
	}

	return NewItemBlock(
		itemID,
		text,
		createdAt,
		d.instanceName,
		metadata,
		item.Type,
		item.Title,
		item.Text,
		item.URL,
		item.By,
		item.Score,
		item.Descendants,
		item.Parent,
		item.Poll,
		item.Deleted,
		item.Dead,
	)
}

func (d *Datasource) deduplicateAndLimit(ids []int) []int {
	seen := make(map[int]bool)
	var result []int

	for _, id := range ids {
		if !seen[id] && len(result) < d.config.MaxItems {
			seen[id] = true
			result = append(result, id)
		}
	}

	return result
}

func (d *Datasource) Close() error {
	return nil
}

func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
