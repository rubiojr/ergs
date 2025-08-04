package testrand

import (
	"fmt"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// RandomBlock represents a randomly generated block for testing purposes.
type RandomBlock struct {
	blockID    string
	randomData string
	prefix     string
	sequence   int
	createdAt  time.Time
	source     string
}

// NewRandomBlock creates a new random block with default source.
func NewRandomBlock(blockID, randomData, prefix string, sequence int, createdAt time.Time) *RandomBlock {
	return &RandomBlock{
		blockID:    blockID,
		randomData: randomData,
		prefix:     prefix,
		sequence:   sequence,
		createdAt:  createdAt,
		source:     "testrand",
	}
}

// NewRandomBlockWithSource creates a new random block with specified source.
func NewRandomBlockWithSource(blockID, randomData, prefix string, sequence int, createdAt time.Time, source string) *RandomBlock {
	return &RandomBlock{
		blockID:    blockID,
		randomData: randomData,
		prefix:     prefix,
		sequence:   sequence,
		createdAt:  createdAt,
		source:     source,
	}
}

// ID returns the unique identifier for this block.
func (b *RandomBlock) ID() string {
	return b.blockID
}

// Text returns the main text content of the block.
func (b *RandomBlock) Text() string {
	return b.randomData
}

// CreatedAt returns when this block was created.
func (b *RandomBlock) CreatedAt() time.Time {
	return b.createdAt
}

// Source returns the datasource that created this block.
func (b *RandomBlock) Source() string {
	return b.source
}

// Type returns the block type identifier.
func (b *RandomBlock) Type() string {
	return "testrand"
}

// Metadata returns additional structured data about this block.
func (b *RandomBlock) Metadata() map[string]interface{} {
	return map[string]interface{}{
		"block_id":    b.blockID,
		"random_data": b.randomData,
		"prefix":      b.prefix,
		"sequence":    b.sequence,
	}
}

// Summary returns a brief summary of the block.
func (b *RandomBlock) Summary() string {
	return fmt.Sprintf("[%s] Random block #%d: %s", b.prefix, b.sequence, b.randomData)
}

// PrettyText returns a formatted representation of the block.
func (b *RandomBlock) PrettyText() string {
	return fmt.Sprintf("🎲 Random Test Block [%s]\n  ID: %s\n  Sequence: %d\n  Content: %s\n  Created: %s\n  Source: %s",
		b.prefix,
		b.blockID,
		b.sequence,
		b.randomData,
		b.createdAt.Format("2006-01-02 15:04:05"),
		b.source,
	)
}

// Factory creates a RandomBlock from a generic block and source.
// This is used when reconstructing blocks from database storage.
func (b *RandomBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	metadata := genericBlock.Metadata()

	// Extract metadata with proper type assertions and defaults
	blockID, _ := metadata["block_id"].(string)
	if blockID == "" {
		blockID = genericBlock.ID()
	}

	randomData, _ := metadata["random_data"].(string)
	if randomData == "" {
		randomData = genericBlock.Text()
	}

	prefix, _ := metadata["prefix"].(string)
	if prefix == "" {
		prefix = "RAND"
	}

	sequence := 0
	if seq, ok := metadata["sequence"].(int64); ok {
		sequence = int(seq)
	} else if seq, ok := metadata["sequence"].(int); ok {
		sequence = seq
	}

	return &RandomBlock{
		blockID:    blockID,
		randomData: randomData,
		prefix:     prefix,
		sequence:   sequence,
		createdAt:  genericBlock.CreatedAt(),
		source:     source,
	}
}
