package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/datasources/rtve"
)

// RTVEVideo represents a video from the RTVE JSON files
type RTVEVideo struct {
	URI             string `json:"uri"`
	HTMLURL         string `json:"htmlUrl"`
	ID              string `json:"id"`
	LongTitle       string `json:"longTitle"`
	PublicationDate string `json:"publicationDate"`
}

// ImportRequest is the payload sent to the importer API
type ImportRequest struct {
	Blocks []core.GenericBlock `json:"blocks"`
}

// ImportResponse is the response from the importer API
type ImportResponse struct {
	Accepted int      `json:"accepted"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors,omitempty"`
}

type Config struct {
	VideosDir     string
	ImporterURL   string
	APIToken      string
	TargetDatasrc string
	BatchSize     int
	DryRun        bool
}

func main() {
	cfg := Config{}

	flag.StringVar(&cfg.VideosDir, "videos-dir", "rtve-videos", "Directory containing RTVE video JSON files")
	flag.StringVar(&cfg.ImporterURL, "importer-url", "http://localhost:9090", "URL of the importer API server")
	flag.StringVar(&cfg.APIToken, "api-token", "", "API token for authentication (required)")
	flag.StringVar(&cfg.TargetDatasrc, "target-datasource", "rtve", "Target datasource name in Ergs")
	flag.IntVar(&cfg.BatchSize, "batch-size", 50, "Number of blocks to send per request")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Don't actually send blocks, just show what would be imported")
	flag.Parse()

	if cfg.APIToken == "" && !cfg.DryRun {
		log.Fatal("Error: --api-token is required (unless using --dry-run)")
	}

	if err := run(cfg); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(cfg Config) error {
	log.Printf("RTVE Importer")
	log.Printf("=============")
	log.Printf("Videos directory: %s", cfg.VideosDir)
	log.Printf("Importer URL: %s", cfg.ImporterURL)
	log.Printf("Target datasource: %s", cfg.TargetDatasrc)
	log.Printf("Batch size: %d", cfg.BatchSize)
	if cfg.DryRun {
		log.Printf("Mode: DRY RUN (no blocks will be sent)")
	}
	log.Printf("")

	// Find all JSON files
	videoFiles, err := findVideoFiles(cfg.VideosDir)
	if err != nil {
		return fmt.Errorf("finding video files: %w", err)
	}

	log.Printf("Found %d video JSON files", len(videoFiles))
	log.Printf("")

	// Process files in batches
	var batch []core.Block
	totalProcessed := 0
	totalAccepted := 0
	totalRejected := 0
	var allErrors []string

	for i, filePath := range videoFiles {
		// Read and parse JSON file
		video, err := readVideoFile(filePath)
		if err != nil {
			log.Printf("Warning: skipping %s: %v", filePath, err)
			continue
		}

		// Load subtitles if available
		hasSubtitles, subtitleLangs, subtitleText := loadSubtitles(filePath, video.ID)

		// Create RTVE block using the existing datasource code
		// The block already has all the correct properties
		block := createRTVEBlock(video, hasSubtitles, subtitleLangs, subtitleText, cfg.TargetDatasrc)

		batch = append(batch, block)

		// Send batch when it reaches the configured size or at the end
		if len(batch) >= cfg.BatchSize || i == len(videoFiles)-1 {
			if cfg.DryRun {
				log.Printf("Would import batch of %d blocks (total: %d/%d)", len(batch), totalProcessed+len(batch), len(videoFiles))
				for _, b := range batch {
					log.Printf("  - %s: %s", b.ID(), b.Metadata()["long_title"])
				}
			} else {
				resp, err := sendBatch(cfg.ImporterURL, cfg.APIToken, batch)
				if err != nil {
					return fmt.Errorf("sending batch: %w", err)
				}

				totalAccepted += resp.Accepted
				totalRejected += resp.Rejected
				allErrors = append(allErrors, resp.Errors...)

				log.Printf("Batch %d: accepted %d, rejected %d (total: %d/%d)",
					(totalProcessed/cfg.BatchSize)+1, resp.Accepted, resp.Rejected, totalProcessed+len(batch), len(videoFiles))

				if len(resp.Errors) > 0 {
					for _, errMsg := range resp.Errors {
						log.Printf("  Error: %s", errMsg)
					}
				}
			}

			totalProcessed += len(batch)
			batch = nil
		}
	}

	log.Printf("")
	log.Printf("Import complete!")
	log.Printf("Total files processed: %d", totalProcessed)
	if !cfg.DryRun {
		log.Printf("Total blocks accepted: %d", totalAccepted)
		log.Printf("Total blocks rejected: %d", totalRejected)
		if len(allErrors) > 0 {
			log.Printf("Total errors: %d", len(allErrors))
		}
	}

	return nil
}

func findVideoFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasPrefix(filepath.Base(path), "video_") && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

func readVideoFile(path string) (*RTVEVideo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var video RTVEVideo
	if err := json.Unmarshal(data, &video); err != nil {
		return nil, err
	}

	return &video, nil
}

func loadSubtitles(videoPath string, videoID string) (bool, []string, string) {
	// Find subs directory relative to video JSON file
	videoDir := filepath.Dir(videoPath)
	subsDir := filepath.Join(videoDir, "subs")

	// Check if subs directory exists
	if _, err := os.Stat(subsDir); os.IsNotExist(err) {
		return false, nil, ""
	}

	// Look for subtitle files for this video
	subsPattern := fmt.Sprintf("%s_*.vtt", videoID)
	matches, err := filepath.Glob(filepath.Join(subsDir, subsPattern))
	if err != nil || len(matches) == 0 {
		return false, nil, ""
	}

	// Find available languages and Spanish subtitle path
	var langs []string
	var esSubPath string

	for _, path := range matches {
		// Extract language from filename (e.g., 16755959_es.vtt -> es)
		base := filepath.Base(path)
		parts := strings.Split(base, "_")
		if len(parts) == 2 {
			langWithExt := parts[1]
			lang := strings.TrimSuffix(langWithExt, ".vtt")
			langs = append(langs, lang)
			if lang == "es" {
				esSubPath = path
			}
		}
	}

	if len(langs) == 0 {
		return false, nil, ""
	}

	// If no Spanish subs, use the first available
	if esSubPath == "" {
		esSubPath = matches[0]
	}

	// Read and parse VTT file
	vttContent, err := os.ReadFile(esSubPath)
	if err != nil {
		log.Printf("Warning: failed to read subtitles %s: %v", esSubPath, err)
		return true, langs, ""
	}

	// Parse VTT
	cues, err := parseVTT(string(vttContent))
	if err != nil {
		log.Printf("Warning: failed to parse VTT %s: %v", esSubPath, err)
		return true, langs, ""
	}

	// Convert to JSON
	cuesJSON, err := json.Marshal(cues)
	if err != nil {
		log.Printf("Warning: failed to marshal subtitles: %v", err)
		return true, langs, ""
	}

	return true, langs, string(cuesJSON)
}

func parseVTT(content string) ([]rtve.VTTCue, error) {
	lines := strings.Split(content, "\n")
	var cues []rtve.VTTCue
	var currentCue *rtve.VTTCue

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip WEBVTT header and empty lines
		if line == "" || strings.HasPrefix(line, "WEBVTT") {
			continue
		}

		// Check for timestamp line (00:00:08.537 --> 00:00:11.872)
		if strings.Contains(line, "-->") {
			// Save previous cue if exists
			if currentCue != nil && currentCue.Text != "" {
				cues = append(cues, *currentCue)
			}
			// Parse timestamps
			parts := strings.Split(line, "-->")
			if len(parts) == 2 {
				start := strings.TrimSpace(strings.Split(parts[0], " ")[0])
				end := strings.TrimSpace(strings.Split(parts[1], " ")[0])
				currentCue = &rtve.VTTCue{
					StartTime: start,
					EndTime:   end,
				}
			}
		} else if currentCue != nil {
			// Remove WebVTT positioning tags (line:XX%, etc.)
			if !strings.Contains(line, "line:") && !strings.Contains(line, "position:") {
				text := strings.TrimSpace(line)
				if text != "" {
					if currentCue.Text != "" {
						currentCue.Text += " "
					}
					currentCue.Text += text
				}
			}
		}
	}

	// Add last cue
	if currentCue != nil && currentCue.Text != "" {
		cues = append(cues, *currentCue)
	}

	return cues, nil
}

func createRTVEBlock(video *RTVEVideo, hasSubtitles bool, subtitleLangs []string, subtitleText string, source string) *rtve.RTVEBlock {
	// Use the existing RTVE datasource code to create blocks

	return rtve.NewRTVEBlockWithSource(
		video.ID,
		video.LongTitle,
		video.PublicationDate,
		video.HTMLURL,
		video.URI,
		hasSubtitles,
		subtitleLangs,
		subtitleText,
		source,
	)
}

func sendBatch(importerURL, apiToken string, blocks []core.Block) (*ImportResponse, error) {
	url := fmt.Sprintf("%s/api/import/blocks", importerURL)

	// Convert core.Block to core.GenericBlock for JSON marshaling
	genericBlocks := make([]core.GenericBlock, len(blocks))
	for i, block := range blocks {
		// If it's already a GenericBlock, use it directly
		if gb, ok := block.(*rtve.RTVEBlock); ok {
			// Convert to GenericBlock using the block's data
			genericBlocks[i] = *core.NewGenericBlock(
				gb.ID(),
				gb.Text(),
				gb.Source(),
				gb.Type(),
				gb.CreatedAt(),
				gb.Metadata(),
			)
		} else if gb, ok := block.(*core.GenericBlock); ok {
			genericBlocks[i] = *gb
		} else {
			// Fallback for any other block type
			genericBlocks[i] = *core.NewGenericBlock(
				block.ID(),
				block.Text(),
				block.Source(),
				block.Type(),
				block.CreatedAt(),
				block.Metadata(),
			)
		}
	}

	req := ImportRequest{
		Blocks: genericBlocks,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var importResp ImportResponse
	if err := json.Unmarshal(body, &importResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &importResp, nil
}
