package codeberg

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("codeberg", prototype)
}

// BlockFactory implements the BlockFactory interface for Codeberg
type BlockFactory struct{}

func (f *BlockFactory) CreateFromGeneric(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) core.Block {
	eventType := getStringFromMetadata(metadata, "event_type", "RepositoryEvent")
	actorLogin := getStringFromMetadata(metadata, "actor_login", "unknown")
	repoName := getStringFromMetadata(metadata, "repo_name", "")
	repoURL := getStringFromMetadata(metadata, "repo_url", "")
	repoDesc := getStringFromMetadata(metadata, "repo_desc", "")
	language := getStringFromMetadata(metadata, "language", "")
	stars := getIntFromMetadata(metadata, "stars", 0)
	forks := getIntFromMetadata(metadata, "forks", 0)
	public := getBoolFromMetadata(metadata, "public", true)
	payload := getStringFromMetadata(metadata, "payload", "")

	return NewEventBlock(id, eventType, actorLogin, repoName, repoURL, repoDesc, language, stars, forks, createdAt, public, payload, source)
}

type ForgejoRepository struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	FullName    string    `json:"full_name"`
	Description string    `json:"description"`
	HTMLURL     string    `json:"html_url"`
	Language    string    `json:"language"`
	Stars       int       `json:"stars_count"`
	Forks       int       `json:"forks_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Private     bool      `json:"private"`
	Owner       struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type SearchResponse struct {
	OK   bool                `json:"ok"`
	Data []ForgejoRepository `json:"data"`
}

type Config struct {
	Token    string `toml:"token"`
	Language string `toml:"language"`
	Pages    int    `toml:"pages"`
}

func (c *Config) Validate() error {
	return nil
}

type Datasource struct {
	config       *Config
	client       *http.Client
	baseURL      string
	instanceName string
}

func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var cbConfig *Config
	if config == nil {
		cbConfig = &Config{}
	} else {
		var ok bool
		cbConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for Codeberg datasource")
		}
	}

	return &Datasource{
		config:       cbConfig,
		client:       &http.Client{Timeout: 30 * time.Second},
		baseURL:      "https://codeberg.org/api/v1",
		instanceName: instanceName,
	}, nil
}

func (d *Datasource) Type() string {
	return "codeberg"
}

func (d *Datasource) Name() string {
	return d.instanceName
}

func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"event_type":       "TEXT",
		"actor_login":      "TEXT",
		"repo_name":        "TEXT",
		"repo_url":         "TEXT",
		"repo_description": "TEXT",
		"language":         "TEXT",
		"stars":            "INTEGER",
		"forks":            "INTEGER",
		"public":           "BOOLEAN",
		"payload":          "TEXT",
	}
}

func (d *Datasource) BlockPrototype() core.Block {
	return &EventBlock{}
}

func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

func (d *Datasource) SetConfig(config interface{}) error {
	if cfg, ok := config.(*Config); ok {

		d.config = cfg
		return cfg.Validate()
	}
	return fmt.Errorf("invalid config type for Codeberg datasource")
}

func (d *Datasource) GetConfig() interface{} {
	return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	log.Printf("Fetching Codeberg repositories")

	page := 1
	limit := 50
	repoCount := 0

	// Default to 10 pages if not configured
	maxPages := d.config.Pages
	if maxPages <= 0 {
		maxPages = 10
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		url := fmt.Sprintf("%s/repos/search?sort=updated&order=desc&page=%d&limit=%d", d.baseURL, page, limit)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}

		if d.config.Token != "" {
			req.Header.Set("Authorization", "token "+d.config.Token)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "ergs/1.0")

		resp, err := d.client.Do(req)
		if err != nil {
			return fmt.Errorf("making request: %w", err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				fmt.Printf("Warning: failed to close response body: %v\n", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("codeberg API returned status %d", resp.StatusCode)
		}

		var searchResp SearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}

		repos := searchResp.Data
		log.Printf("Processing Codeberg repositories page %d with %d repos", page, len(repos))

		if len(repos) == 0 {
			break
		}

		for _, repo := range repos {
			if d.config.Language != "" && !strings.EqualFold(repo.Language, d.config.Language) {
				continue
			}

			block := d.convertRepoToBlock(repo)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case blockCh <- block:
				repoCount++
			}
		}

		page++
		if page > maxPages {
			log.Printf("Reached maximum page limit (%d), stopping fetch", maxPages)
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Fetched %d Codeberg repositories across %d pages", repoCount, page-1)
	return nil
}

func (d *Datasource) convertRepoToBlock(repo ForgejoRepository) core.Block {
	// Namespace ID with instance name to prevent collisions across multiple Codeberg datasource instances
	eventID := fmt.Sprintf("repo-%d", repo.ID)

	block := NewEventBlock(
		eventID,
		"RepositoryEvent",
		repo.Owner.Login,
		repo.FullName,
		repo.HTMLURL,
		repo.Description,
		repo.Language,
		repo.Stars,
		repo.Forks,
		repo.UpdatedAt,
		!repo.Private,
		"",
		d.instanceName,
	)

	return block
}

func (d *Datasource) Close() error {
	return nil
}

func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
