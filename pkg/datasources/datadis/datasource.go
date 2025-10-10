package datadis

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/go-datadis"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("datadis", prototype)
}

// Config holds the configuration for the Datadis datasource
type Config struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}
	if c.Password == "" {
		return fmt.Errorf("password is required")
	}
	return nil
}

// Datasource implements the core.Datasource interface for Datadis
type Datasource struct {
	config       *Config
	client       *datadis.Client
	instanceName string
	supplies     []datadis.Supply
}

// NewDatasource creates a new Datadis datasource instance
func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var cfg *Config
	if config == nil {
		cfg = &Config{}
	} else {
		var ok bool
		cfg, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for Datadis datasource")
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	client := datadis.NewClient()

	// Try to login to validate credentials
	if err := client.Login(cfg.Username, cfg.Password); err != nil {
		return nil, fmt.Errorf("failed to login to Datadis: %w", err)
	}

	// Fetch supplies once during initialization
	supplies, err := client.Supplies()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch supplies: %w", err)
	}

	if len(supplies) == 0 {
		return nil, fmt.Errorf("no electricity supplies found for this account")
	}

	log.Printf("Datadis datasource initialized with %d supplies", len(supplies))

	return &Datasource{
		config:       cfg,
		client:       client,
		instanceName: instanceName,
		supplies:     supplies,
	}, nil
}

// Type returns the datasource type
func (d *Datasource) Type() string {
	return "datadis"
}

// Name returns the instance name
func (d *Datasource) Name() string {
	return d.instanceName
}

// Schema returns the database schema for this datasource
func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"cups":          "TEXT",
		"date":          "TEXT",
		"hour":          "TEXT",
		"consumption":   "REAL",
		"obtain_method": "TEXT",
		"address":       "TEXT",
		"province":      "TEXT",
		"postal_code":   "TEXT",
		"municipality":  "TEXT",
		"distributor":   "TEXT",
	}
}

// BlockPrototype returns a prototype block for this datasource
func (d *Datasource) BlockPrototype() core.Block {
	return &ConsumptionBlock{}
}

// ConfigType returns the configuration type for this datasource
func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

// SetConfig updates the datasource configuration
func (d *Datasource) SetConfig(config interface{}) error {
	if cfg, ok := config.(*Config); ok {
		if err := cfg.Validate(); err != nil {
			return err
		}

		d.config = cfg

		// Recreate client with new credentials
		client := datadis.NewClient()
		if err := client.Login(cfg.Username, cfg.Password); err != nil {
			return fmt.Errorf("failed to login with new credentials: %w", err)
		}

		// Refresh supplies
		supplies, err := client.Supplies()
		if err != nil {
			return fmt.Errorf("failed to fetch supplies: %w", err)
		}

		d.client = client
		d.supplies = supplies

		return nil
	}
	return fmt.Errorf("invalid config type for Datadis datasource")
}

// GetConfig returns the current configuration
func (d *Datasource) GetConfig() interface{} {
	return d.config
}

// FetchBlocks fetches electricity consumption data for the current month
func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	log.Printf("Fetching Datadis consumption data for the current month")

	// Ensure we're logged in
	if err := d.client.Login(d.config.Username, d.config.Password); err != nil {
		return fmt.Errorf("failed to login to Datadis: %w", err)
	}

	// Get current date and beginning of month
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)

	// Datadis API expects YYYY/MM format, and both dates should be in the same month
	// The API will return all data from the beginning of the month until today
	log.Printf("Fetching consumption data from %s to %s",
		startOfMonth.Format("2006/01/02"),
		now.Format("2006/01/02"))

	blockCount := 0

	for _, supply := range d.supplies {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("Fetching consumption data for supply %s (%s)", supply.Cups, supply.Address)

		// Fetch consumption data for this supply
		// Using the same month for both from and to parameters
		measurements, err := d.client.ConsumptionData(&supply, startOfMonth, now)
		if err != nil {
			log.Printf("Failed to fetch consumption data for supply %s: %v", supply.Cups, err)
			continue
		}

		log.Printf("Got %d measurements for supply %s", len(measurements), supply.Cups)

		for _, measurement := range measurements {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Parse the measurement time to get a proper timestamp
			// Datadis returns date as "YYYY/MM/DD" and time as "HH"
			measurementTime, err := d.parseMeasurementTime(measurement.Date, measurement.Time)
			if err != nil {
				log.Printf("Failed to parse measurement time: %v", err)
				measurementTime = time.Now()
			}

			// Create unique block ID
			blockID := fmt.Sprintf("consumption-%s-%s-%s",
				supply.Cups,
				measurement.Date,
				measurement.Time)

			block := NewConsumptionBlock(
				blockID,
				measurement.Cups,
				measurement.Date,
				measurement.Time,
				measurement.Consumption,
				measurement.ObtainMethod,
				supply.Address,
				supply.Province,
				supply.PostalCode,
				supply.Municipality,
				supply.Distributor,
				measurementTime,
				d.instanceName,
			)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case blockCh <- block:
				blockCount++
			}
		}

		// Small delay between supplies to avoid hitting rate limits
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Fetched %d consumption blocks from Datadis", blockCount)
	return nil
}

// parseMeasurementTime converts Datadis date and hour to a time.Time
func (d *Datasource) parseMeasurementTime(date, hour string) (time.Time, error) {
	// Datadis returns date as "YYYY/MM/DD" and hour as "HH" (00-23)
	// Combine them into a single timestamp
	dateTimeStr := fmt.Sprintf("%s %s:00:00", date, hour)

	// Parse with the expected format
	t, err := time.ParseInLocation("2006/01/02 15:04:05", dateTimeStr, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse date/time %s: %w", dateTimeStr, err)
	}

	return t, nil
}

// Close closes the datasource
func (d *Datasource) Close() error {
	// Nothing to close for HTTP client
	return nil
}

// Factory creates a new instance of the datasource
func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
