package github

import (
	"fmt"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

type EventBlock struct {
	id         string
	text       string
	createdAt  time.Time
	source     string
	metadata   map[string]interface{}
	eventType  string
	actorLogin string
	repoName   string
	repoURL    string
	repoDesc   string
	language   string
	stars      int
	forks      int
	public     bool
	payload    string
}

func NewEventBlock(id, eventType, actorLogin, repoName, repoURL, repoDesc, language string, stars, forks int, createdAt time.Time, public bool, payload string) *EventBlock {
	text := fmt.Sprintf("event_type=%s actor_login=%s repo_name=%s repo_desc=%s language=%s repo_url=%s stars=%d forks=%d public=%t",
		eventType, actorLogin, repoName, repoDesc, language, repoURL, stars, forks, public)

	metadata := map[string]interface{}{
		"event_type":  eventType,
		"actor_login": actorLogin,
		"repo_name":   repoName,
		"repo_url":    repoURL,
		"repo_desc":   repoDesc,
		"language":    language,
		"stars":       stars,
		"forks":       forks,
		"public":      public,
		"payload":     payload,
	}

	return &EventBlock{
		id:         id,
		text:       text,
		createdAt:  createdAt,
		source:     "github",
		metadata:   metadata,
		eventType:  eventType,
		actorLogin: actorLogin,
		repoName:   repoName,
		repoURL:    repoURL,
		repoDesc:   repoDesc,
		language:   language,
		stars:      stars,
		forks:      forks,
		public:     public,
		payload:    payload,
	}
}

func (e *EventBlock) ID() string {
	return e.id
}

func (e *EventBlock) Text() string {
	return e.text
}

func (e *EventBlock) CreatedAt() time.Time {
	return e.createdAt
}

func (e *EventBlock) Source() string {
	return e.source
}

func (e *EventBlock) Metadata() map[string]interface{} {
	return e.metadata
}

func (e *EventBlock) Type() string {
	return "github"
}

func (e *EventBlock) EventType() string {
	return e.eventType
}

func (e *EventBlock) ActorLogin() string {
	return e.actorLogin
}

func (e *EventBlock) RepoName() string {
	return e.repoName
}

func (e *EventBlock) RepoURL() string {
	return e.repoURL
}

func (e *EventBlock) RepoDescription() string {
	return e.repoDesc
}

func (e *EventBlock) Language() string {
	return e.language
}

func (e *EventBlock) Stars() int {
	return e.stars
}

func (e *EventBlock) Forks() int {
	return e.forks
}

func (e *EventBlock) IsPublic() bool {
	return e.public
}

func (e *EventBlock) Payload() string {
	return e.payload
}

func (e *EventBlock) PrettyText() string {
	visibility := "public"
	if !e.public {
		visibility = "private"
	}

	languageInfo := ""
	if e.language != "" {
		languageInfo = fmt.Sprintf(" [%s]", e.language)
	}

	repoInfo := ""
	if e.repoName != "" {
		repoInfo = fmt.Sprintf("\n  Repository: %s%s ⭐ %d 🍴 %d (%s)",
			e.repoName, languageInfo, e.stars, e.forks, visibility)
		if e.repoDesc != "" {
			repoInfo += fmt.Sprintf("\n  Description: %s", e.repoDesc)
		}
		if e.repoURL != "" {
			repoInfo += fmt.Sprintf("\n  URL: %s", e.repoURL)
		}
	}

	// Format metadata using utility function
	metadataInfo := core.FormatMetadata(e.metadata)

	return fmt.Sprintf("🐙 GitHub %s by %s\n  ID: %s\n  Time: %s%s%s",
		e.eventType, e.actorLogin, e.id, e.createdAt.Format("2006-01-02 15:04:05"), repoInfo, metadataInfo)
}

// Summary returns a concise one-line summary of the GitHub event.
func (e *EventBlock) Summary() string {
	repoInfo := ""
	if e.repoName != "" {
		repoInfo = fmt.Sprintf(" on %s", e.repoName)
	}
	return fmt.Sprintf("🐙 %s by %s%s", e.eventType, e.actorLogin, repoInfo)
}

// Factory creates a new EventBlock from a GenericBlock and source.
// This method is part of the core.Block interface and enables reconstruction
// from database data without requiring separate factory objects.
func (e *EventBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	metadata := genericBlock.Metadata()
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

	return &EventBlock{
		id:         genericBlock.ID(),
		text:       genericBlock.Text(),
		createdAt:  genericBlock.CreatedAt(),
		source:     source,
		metadata:   metadata,
		eventType:  eventType,
		actorLogin: actorLogin,
		repoName:   repoName,
		repoURL:    repoURL,
		repoDesc:   repoDesc,
		language:   language,
		stars:      stars,
		forks:      forks,
		public:     public,
		payload:    payload,
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
