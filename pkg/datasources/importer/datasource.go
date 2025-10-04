package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// Config for importer datasource
type Config struct {
	// APIURL is the URL of the importer API server
	// Default: http://localhost:9090
	APIURL string `toml:"api_url"`

	// APIKey is the bearer token for authenticating with the importer API
	APIKey string `toml:"api_key"`
}

func (c *Config) Validate() error {
	if c.APIURL == "" {
		c.APIURL = "http://localhost:9090"
	}
	return nil
}

type Datasource struct {
	instanceName string
	config       Config
	httpClient   *http.Client
}

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("importer", prototype)
}

func (d *Datasource) Name() string {
	return d.instanceName
}

func (d *Datasource) Type() string {
	return "importer"
}

func (d *Datasource) SetInstanceName(name string) {
	d.instanceName = name
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	if d.httpClient == nil {
		d.httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	log.Printf("Importer datasource: fetching blocks from API at %s", d.config.APIURL)

	// Fetch blocks from the API
	url := fmt.Sprintf("%s/api/blocks/export", d.config.APIURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Add authorization header if token is configured
	if d.config.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", d.config.APIKey))
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching blocks from API: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response struct {
		Blocks []struct {
			ID         string                 `json:"id"`
			Text       string                 `json:"text"`
			CreatedAt  time.Time              `json:"created_at"`
			Type       string                 `json:"type"`
			Datasource string                 `json:"datasource"`
			Metadata   map[string]interface{} `json:"metadata"`
		} `json:"blocks"`
		Count int `json:"count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("decoding API response: %w", err)
	}

	blockCount := 0
	for _, blockData := range response.Blocks {
		// Validate datasource field
		if blockData.Datasource == "" {
			log.Printf("Warning: block %s missing datasource field, skipping", blockData.ID)
			continue
		}

		// Create a generic block with the datasource from block data as source
		// This routes the block to the correct native datasource storage
		block := core.NewGenericBlock(
			blockData.ID,
			blockData.Text,
			blockData.Datasource, // Route to the datasource specified in block data
			blockData.Type,
			blockData.CreatedAt,
			blockData.Metadata,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case blockCh <- block:
			blockCount++
		}
	}

	if blockCount > 0 {
		log.Printf("Importer datasource: fetched and processed %d blocks", blockCount)
	} else {
		log.Printf("Importer datasource: no blocks to process")
	}

	return nil
}

func (d *Datasource) Schema() map[string]any {
	// Importer doesn't need its own storage - it routes blocks to other datasources
	// Returning nil signals that no database should be created for this datasource
	return nil
}

func (d *Datasource) BlockPrototype() core.Block {
	return &core.GenericBlock{}
}

func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

func (d *Datasource) SetConfig(config interface{}) error {
	cfg, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("invalid config type: expected *importer.Config")
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	d.config = *cfg
	return nil
}

func (d *Datasource) GetConfig() interface{} {
	return &d.config
}

func (d *Datasource) Close() error {
	if d.httpClient != nil {
		d.httpClient.CloseIdleConnections()
	}
	return nil
}

func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	ds := &Datasource{
		instanceName: instanceName,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	if config != nil {
		if err := ds.SetConfig(config); err != nil {
			return nil, err
		}
	} else {
		// Set default config
		ds.config = Config{
			APIURL: "http://localhost:9090",
		}
	}

	return ds, nil
}
