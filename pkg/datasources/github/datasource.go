package github

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/go-github/v73/github"
	"github.com/rubiojr/ergs/pkg/core"
	"golang.org/x/oauth2"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("github", prototype)
}

// BlockFactory implements the BlockFactory interface for GitHub
type BlockFactory struct{}

func (f *BlockFactory) CreateFromGeneric(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) core.Block {
	eventType := getStringFromMetadata(metadata, "event_type", "UnknownEvent")
	actorLogin := getStringFromMetadata(metadata, "actor_login", "unknown")
	repoName := getStringFromMetadata(metadata, "repo_name", "")
	repoURL := getStringFromMetadata(metadata, "repo_url", "")
	repoDesc := getStringFromMetadata(metadata, "repo_desc", "")
	language := getStringFromMetadata(metadata, "language", "")
	stars := getIntFromMetadata(metadata, "stars", 0)
	forks := getIntFromMetadata(metadata, "forks", 0)
	public := getBoolFromMetadata(metadata, "public", true)
	payload := getStringFromMetadata(metadata, "payload", "")

	// Use instance source (passed in) for proper data isolation
	return NewEventBlock(id, eventType, actorLogin, repoName, repoURL, repoDesc, language, stars, forks, createdAt, public, payload, source)
}

// Helper functions for safe metadata extraction

type Config struct {
	Token    string `toml:"token"`
	Language string `toml:"language"`
}

func (c *Config) Validate() error {
	return nil
}

type Datasource struct {
	config       *Config
	client       *github.Client
	instanceName string
}

func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var ghConfig *Config
	if config == nil {
		ghConfig = &Config{}
	} else {
		var ok bool
		ghConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for GitHub datasource")
		}
	}

	var client *github.Client
	if ghConfig.Token != "" {

		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: ghConfig.Token},
		)
		tc := oauth2.NewClient(context.Background(), ts)
		client = github.NewClient(tc)
	} else {

		client = github.NewClient(nil)
	}

	return &Datasource{
		config:       ghConfig,
		client:       client,
		instanceName: instanceName,
	}, nil
}

func (d *Datasource) Type() string {
	return "github"
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

		// Recreate the GitHub client with the new config
		var client *github.Client
		if cfg.Token != "" {

			ts := oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: cfg.Token},
			)
			tc := oauth2.NewClient(context.Background(), ts)
			client = github.NewClient(tc)
		} else {

			client = github.NewClient(nil)
		}
		d.client = client

		return cfg.Validate()
	}
	return fmt.Errorf("invalid config type for GitHub datasource")
}

func (d *Datasource) GetConfig() interface{} {
	return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	log.Printf("Fetching GitHub events")

	opts := &github.ListOptions{
		Page:    1,
		PerPage: 100,
	}

	eventCount := 0
	pageCount := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pageCount++
		events, resp, err := d.client.Activity.ListEvents(ctx, opts)
		if err != nil {
			return fmt.Errorf("fetching events from GitHub: %w", err)
		}

		log.Printf("Processing GitHub events page %d with %d events", pageCount, len(events))

		for _, event := range events {
			if event.CreatedAt == nil {
				continue
			}

			block, err := d.convertEventToBlock(ctx, event)
			if err != nil {
				log.Printf("Failed to convert event: %v", err)
				continue
			}

			if d.config.Language != "" && !strings.EqualFold(d.getLanguageFromBlock(block), d.config.Language) {
				continue
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case blockCh <- block:
				eventCount++
			}
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage

		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Fetched %d GitHub events across %d pages", eventCount, pageCount)
	return nil
}

func (d *Datasource) convertEventToBlock(ctx context.Context, event *github.Event) (core.Block, error) {
	if event.Actor == nil {
		return nil, fmt.Errorf("event missing actor")
	}

	// Use simple event ID form; instance name already isolates storage (separate DB per instance)
	eventID := fmt.Sprintf("event-%s", event.GetID())

	// Get payload as JSON string
	payloadJSON := ""
	if event.RawPayload != nil {
		if payloadBytes, err := json.Marshal(event.RawPayload); err == nil {
			payloadJSON = string(payloadBytes)
		}
	}

	// For events without repo, create a simplified block
	if event.Repo == nil {
		block := NewEventBlock(
			eventID,
			event.GetType(),
			event.Actor.GetLogin(),
			"",
			"",
			"",
			"",
			0,
			0,
			event.CreatedAt.Time.UTC(),
			event.GetPublic(),
			payloadJSON,
			d.instanceName,
		)
		return block, nil
	}

	repoName := event.Repo.GetName()
	if repoName == "" {
		return nil, fmt.Errorf("event missing repository name")
	}

	// For events with repo, try to get repository details
	repo, err := d.getRepositoryDetails(ctx, repoName)
	if err != nil {
		log.Printf("Warning: could not get repository details for %s: %v", repoName, err)
		// Create block with basic repo info from event
		block := NewEventBlock(
			eventID,
			event.GetType(),
			event.Actor.GetLogin(),
			repoName,
			event.Repo.GetURL(),
			"",
			"",
			0,
			0,
			event.CreatedAt.Time.UTC(),
			event.GetPublic(),
			payloadJSON,
			d.instanceName,
		)
		return block, nil
	}

	block := NewEventBlock(
		eventID,
		event.GetType(),
		event.Actor.GetLogin(),
		repo.GetFullName(),
		repo.GetHTMLURL(),
		repo.GetDescription(),
		repo.GetLanguage(),
		repo.GetStargazersCount(),
		repo.GetForksCount(),
		event.CreatedAt.Time.UTC(),
		event.GetPublic(),
		payloadJSON,
		d.instanceName,
	)

	return block, nil
}

func (d *Datasource) getRepositoryDetails(ctx context.Context, repoName string) (*github.Repository, error) {
	parts := strings.Split(repoName, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository name format: %s", repoName)
	}

	owner, name := parts[0], parts[1]
	repo, _, err := d.client.Repositories.Get(ctx, owner, name)
	if err != nil {
		return nil, fmt.Errorf("fetching repository %s: %w", repoName, err)
	}

	return repo, nil
}

func (d *Datasource) getLanguageFromBlock(block core.Block) string {
	metadata := block.Metadata()
	if language, exists := metadata["language"]; exists {
		if langStr, ok := language.(string); ok {
			return langStr
		}
	}
	return ""
}

func (d *Datasource) Close() error {
	return nil
}

func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
