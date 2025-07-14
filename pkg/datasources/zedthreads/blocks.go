package zedthreads

import (
	"fmt"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

type ThreadBlock struct {
	id         string
	text       string
	createdAt  time.Time
	source     string
	metadata   map[string]interface{}
	summary    string
	updatedAt  time.Time
	messages   []Message
	model      *Model
	tokenUsage map[string]interface{}
}

type Message struct {
	ID       int       `json:"id"`
	Role     string    `json:"role"`
	Segments []Segment `json:"segments"`
	IsHidden bool      `json:"is_hidden"`
	Context  string    `json:"context"`
}

type Segment struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Model struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type ThreadData struct {
	Version    string                 `json:"version"`
	Summary    string                 `json:"summary"`
	UpdatedAt  string                 `json:"updated_at"`
	Messages   []Message              `json:"messages"`
	Model      *Model                 `json:"model"`
	TokenUsage map[string]interface{} `json:"cumulative_token_usage"`
}

func NewThreadBlock(id, summary string, updatedAt time.Time, threadData *ThreadData) *ThreadBlock {
	return NewThreadBlockWithSource(id, summary, updatedAt, threadData, "zedthreads")
}

func NewThreadBlockWithSource(id, summary string, updatedAt time.Time, threadData *ThreadData, source string) *ThreadBlock {
	// Extract text content from all messages for search
	var textParts []string
	textParts = append(textParts, summary)

	for _, msg := range threadData.Messages {
		if !msg.IsHidden {
			for _, segment := range msg.Segments {
				if segment.Type == "text" && segment.Text != "" {
					textParts = append(textParts, segment.Text)
				}
			}
		}
	}

	text := strings.Join(textParts, " ")

	// Count messages by role
	userMessages := 0
	assistantMessages := 0
	for _, msg := range threadData.Messages {
		if !msg.IsHidden {
			switch msg.Role {
			case "user":
				userMessages++
			case "assistant":
				assistantMessages++
			}
		}
	}

	metadata := map[string]interface{}{
		"summary":            summary,
		"updated_at":         updatedAt.Format("2006-01-02 15:04:05"),
		"model":              getModelString(threadData.Model),
		"version":            threadData.Version,
		"message_count":      len(threadData.Messages),
		"user_messages":      userMessages,
		"assistant_messages": assistantMessages,
		"source":             source,
	}

	// Add token usage if available
	if threadData.TokenUsage != nil {
		for key, value := range threadData.TokenUsage {
			metadata["token_"+key] = value
		}
	}

	return &ThreadBlock{
		id:         id,
		text:       text,
		createdAt:  updatedAt,
		source:     source,
		metadata:   metadata,
		summary:    summary,
		updatedAt:  updatedAt,
		messages:   threadData.Messages,
		model:      threadData.Model,
		tokenUsage: threadData.TokenUsage,
	}
}

func (t *ThreadBlock) ID() string {
	return t.id
}

func (t *ThreadBlock) Text() string {
	return t.text
}

func (t *ThreadBlock) CreatedAt() time.Time {
	return t.createdAt
}

func (t *ThreadBlock) Source() string {
	return t.source
}

func (t *ThreadBlock) Metadata() map[string]interface{} {
	return t.metadata
}

func (t *ThreadBlock) ThreadSummary() string {
	return t.summary
}

// Summary returns a concise one-line summary of the Zed thread.
func (t *ThreadBlock) Summary() string {
	summary := t.summary
	if summary == "" {
		summary = "Untitled thread"
	}

	messageCount := t.MessageCount()
	return fmt.Sprintf("💬 %s (%d msgs)", summary, messageCount)
}

func (t *ThreadBlock) UpdatedAt() time.Time {
	return t.updatedAt
}

func (t *ThreadBlock) Model() string {
	return getModelString(t.model)
}

func (t *ThreadBlock) Messages() []Message {
	return t.messages
}

func (t *ThreadBlock) MessageCount() int {
	if len(t.messages) > 0 {
		return len(t.messages)
	}
	// Fall back to metadata when messages are not loaded (e.g., from database)
	return getIntFromMetadata(t.metadata, "message_count", 0)
}

func (t *ThreadBlock) UserMessageCount() int {
	if len(t.messages) > 0 {
		count := 0
		for _, msg := range t.messages {
			if !msg.IsHidden && msg.Role == "user" {
				count++
			}
		}
		return count
	}
	// Fall back to metadata when messages are not loaded (e.g., from database)
	return getIntFromMetadata(t.metadata, "user_messages", 0)
}

func (t *ThreadBlock) AssistantMessageCount() int {
	if len(t.messages) > 0 {
		count := 0
		for _, msg := range t.messages {
			if !msg.IsHidden && msg.Role == "assistant" {
				count++
			}
		}
		return count
	}
	// Fall back to metadata when messages are not loaded (e.g., from database)
	return getIntFromMetadata(t.metadata, "assistant_messages", 0)
}

func (t *ThreadBlock) FirstUserMessage() string {
	for _, msg := range t.messages {
		if !msg.IsHidden && msg.Role == "user" && len(msg.Segments) > 0 {
			for _, segment := range msg.Segments {
				if segment.Type == "text" && segment.Text != "" {
					// Truncate long messages for display
					text := segment.Text
					if len(text) > 200 {
						text = text[:200] + "..."
					}
					return text
				}
			}
		}
	}
	return ""
}

func (t *ThreadBlock) PrettyText() string {
	messageInfo := fmt.Sprintf("%d messages (%d user, %d assistant)",
		t.MessageCount(), t.UserMessageCount(), t.AssistantMessageCount())

	modelInfo := ""
	modelStr := getModelString(t.model)
	if modelStr != "" {
		modelInfo = fmt.Sprintf("\n  Model: %s", modelStr)
	}

	firstMessage := t.FirstUserMessage()
	firstMessageInfo := ""
	if firstMessage != "" {
		firstMessageInfo = fmt.Sprintf("\n  First message: %s", firstMessage)
	}

	tokenInfo := ""
	if totalTokens, exists := t.tokenUsage["total"]; exists {
		tokenInfo = fmt.Sprintf("\n  Tokens: %v", totalTokens)
	}

	metadataInfo := core.FormatMetadata(t.metadata)

	return fmt.Sprintf("💬 Zed Thread\n  ID: %s\n  Summary: %s\n  Time: %s\n  %s%s%s%s%s",
		t.id, t.summary, t.updatedAt.Format("2006-01-02 15:04:05"),
		messageInfo, modelInfo, firstMessageInfo, tokenInfo, metadataInfo)
}

// Factory creates a new ThreadBlock from a GenericBlock and source.
// This method is part of the core.Block interface and enables reconstruction
// from database data without requiring separate factory objects.
func (t *ThreadBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	metadata := genericBlock.Metadata()
	summary := getStringFromMetadata(metadata, "summary", "")
	modelStr := getStringFromMetadata(metadata, "model", "")

	// Parse model string back to Model struct if possible
	var model *Model
	if modelStr != "" {
		parts := strings.Split(modelStr, "/")
		if len(parts) == 2 {
			model = &Model{
				Provider: parts[0],
				Model:    parts[1],
			}
		} else {
			model = &Model{
				Model: modelStr,
			}
		}
	}

	return &ThreadBlock{
		id:         genericBlock.ID(),
		text:       genericBlock.Text(),
		createdAt:  genericBlock.CreatedAt(),
		source:     source,
		metadata:   metadata,
		summary:    summary,
		updatedAt:  genericBlock.CreatedAt(), // Use creation time as updated time
		messages:   []Message{},              // Cannot reconstruct full message structure from metadata
		model:      model,
		tokenUsage: make(map[string]interface{}),
	}
}

func getModelString(model *Model) string {
	if model == nil {
		return ""
	}
	if model.Provider != "" && model.Model != "" {
		return fmt.Sprintf("%s/%s", model.Provider, model.Model)
	}
	if model.Model != "" {
		return model.Model
	}
	return ""
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
