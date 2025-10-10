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
	"strconv"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// ConsumptionRecord represents a monthly consumption record from the JSON file
type ConsumptionRecord struct {
	CUPS                    string `json:"CUPS"`
	Fecha                   string `json:"Fecha"` // Format: YYYY/MM
	Valle                   string `json:"Valle"`
	Llano                   string `json:"Llano"`
	Punta                   string `json:"Punta"`
	EnergiaVertidaKWh       string `json:"Energia_vertida_kWh"`
	EnergiaGeneradaKWh      string `json:"Energia_generada_kWh"`
	EnergiaAutoconsumidaKWh string `json:"Energia_autoconsumida_kWh"`
	ConsumoAnual            string `json:"Consumo_Anual"`
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
	FilePath      string
	ImporterURL   string
	APIKey        string
	TargetDatasrc string
	BatchSize     int
	DryRun        bool
}

func main() {
	cfg := Config{}

	flag.StringVar(&cfg.FilePath, "file", "", "Path to Consumptions JSON file (required)")
	flag.StringVar(&cfg.ImporterURL, "importer-url", "http://localhost:9090", "URL of the importer API server")
	flag.StringVar(&cfg.APIKey, "api-key", "", "API token for authentication (required unless --dry-run)")
	flag.StringVar(&cfg.TargetDatasrc, "target-datasource", "datadis", "Target datasource name in Ergs")
	flag.IntVar(&cfg.BatchSize, "batch-size", 50, "Number of blocks to send per request")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Don't actually send blocks, just show what would be imported")
	flag.Parse()

	if cfg.FilePath == "" {
		log.Fatal("Error: --file is required")
	}

	if cfg.APIKey == "" && !cfg.DryRun {
		log.Fatal("Error: --api-key is required (unless using --dry-run)")
	}

	if err := run(cfg); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(cfg Config) error {
	log.Printf("Datadis Importer")
	log.Printf("================")
	log.Printf("File path: %s", cfg.FilePath)
	log.Printf("Importer URL: %s", cfg.ImporterURL)
	log.Printf("Target datasource: %s", cfg.TargetDatasrc)
	log.Printf("Batch size: %d", cfg.BatchSize)
	if cfg.DryRun {
		log.Printf("Mode: DRY RUN (no blocks will be sent)")
	}
	log.Printf("")

	// Read and parse JSON file
	data, err := os.ReadFile(cfg.FilePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	var records []ConsumptionRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	log.Printf("Found %d monthly consumption records", len(records))
	log.Printf("")

	// Convert records to blocks
	var allBlocks []core.GenericBlock
	for _, record := range records {
		blocks, err := convertRecordToBlocks(record, cfg.TargetDatasrc)
		if err != nil {
			log.Printf("Warning: failed to convert record for %s: %v", record.Fecha, err)
			continue
		}
		allBlocks = append(allBlocks, blocks...)
	}

	log.Printf("Generated %d consumption blocks from %d records", len(allBlocks), len(records))
	log.Printf("")

	// Send blocks in batches
	totalProcessed := 0
	totalAccepted := 0
	totalRejected := 0
	var allErrors []string

	for i := 0; i < len(allBlocks); i += cfg.BatchSize {
		end := i + cfg.BatchSize
		if end > len(allBlocks) {
			end = len(allBlocks)
		}

		batch := allBlocks[i:end]

		if cfg.DryRun {
			log.Printf("Would import batch of %d blocks (total: %d/%d)", len(batch), end, len(allBlocks))
			for j, b := range batch {
				if j < 3 { // Show first 3 of each batch
					metadata := b.Metadata()
					log.Printf("  - %s: %s on %s at %s", b.ID(), metadata["cups"], metadata["date"], metadata["hour"])
				}
			}
			if len(batch) > 3 {
				log.Printf("  ... and %d more", len(batch)-3)
			}
		} else {
			resp, err := sendBatch(cfg.ImporterURL, cfg.APIKey, batch)
			if err != nil {
				return fmt.Errorf("sending batch: %w", err)
			}

			totalAccepted += resp.Accepted
			totalRejected += resp.Rejected
			allErrors = append(allErrors, resp.Errors...)

			log.Printf("Batch %d: accepted %d, rejected %d (progress: %d/%d)",
				(i/cfg.BatchSize)+1, resp.Accepted, resp.Rejected, end, len(allBlocks))

			if len(resp.Errors) > 0 {
				for _, errMsg := range resp.Errors {
					log.Printf("  Error: %s", errMsg)
				}
			}
		}

		totalProcessed += len(batch)
	}

	log.Printf("")
	log.Printf("Import complete!")
	log.Printf("Total blocks processed: %d", totalProcessed)
	if !cfg.DryRun {
		log.Printf("Total blocks accepted: %d", totalAccepted)
		log.Printf("Total blocks rejected: %d", totalRejected)
		if len(allErrors) > 0 {
			log.Printf("Total errors: %d", len(allErrors))
		}
	}

	return nil
}

// convertRecordToBlocks converts a monthly consumption record into multiple hourly blocks
// This creates synthetic hourly data by distributing the monthly totals across the month
func convertRecordToBlocks(record ConsumptionRecord, targetDatasrc string) ([]core.GenericBlock, error) {
	// Parse the date (YYYY/MM format)
	parts := strings.Split(record.Fecha, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid date format: %s", record.Fecha)
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid year: %s", parts[0])
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid month: %s", parts[1])
	}

	// Parse consumption values (replace comma with dot for Spanish decimal format)
	valle, err := parseSpanishFloat(record.Valle)
	if err != nil {
		return nil, fmt.Errorf("invalid Valle value: %w", err)
	}

	llano, err := parseSpanishFloat(record.Llano)
	if err != nil {
		return nil, fmt.Errorf("invalid Llano value: %w", err)
	}

	punta, err := parseSpanishFloat(record.Punta)
	if err != nil {
		return nil, fmt.Errorf("invalid Punta value: %w", err)
	}

	// Calculate total monthly consumption
	totalConsumption := valle + llano + punta

	// Create a single aggregated block for the month with all tariff periods
	firstDay := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)

	// Use a distinct ID format for monthly aggregates to avoid conflicts with hourly data
	// Format: monthly-{CUPS}-{YYYY/MM}
	blockID := fmt.Sprintf("monthly-%s-%s", record.CUPS, record.Fecha)

	// Create searchable text
	text := fmt.Sprintf("Electricity consumption: %.2f kWh | Date: %s | CUPS: %s | Valle: %.2f kWh | Llano: %.2f kWh | Punta: %.2f kWh",
		totalConsumption, record.Fecha, record.CUPS, valle, llano, punta)

	// Create metadata matching the datadis datasource schema
	metadata := map[string]interface{}{
		"cups":          record.CUPS,
		"date":          record.Fecha,
		"hour":          "00:00", // Monthly aggregate, using midnight as placeholder
		"consumption":   totalConsumption,
		"obtain_method": "monthly_aggregate",
		"valle":         valle,
		"llano":         llano,
		"punta":         punta,
		// These fields might not be in the monthly data, so we leave them empty
		"address":      "",
		"province":     "",
		"postal_code":  "",
		"municipality": "",
		"distributor":  "",
	}

	block := core.NewGenericBlock(
		blockID,
		text,
		targetDatasrc,
		"datadis",
		firstDay,
		metadata,
	)

	return []core.GenericBlock{*block}, nil
}

// parseSpanishFloat parses a Spanish-formatted float (comma as decimal separator)
func parseSpanishFloat(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	// Replace comma with dot
	s = strings.ReplaceAll(s, ",", ".")
	// Remove any spaces
	s = strings.TrimSpace(s)

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing float from %q: %w", s, err)
	}
	return f, nil
}

func sendBatch(importerURL, apiKey string, blocks []core.GenericBlock) (*ImportResponse, error) {
	url := fmt.Sprintf("%s/api/import/blocks", importerURL)

	req := ImportRequest{
		Blocks: blocks,
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
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

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
