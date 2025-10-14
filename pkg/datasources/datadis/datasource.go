package datadis

import (
	"context"
	"fmt"
	"time"

	"github.com/rubiojr/ergs/pkg/log"

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
	CUPS     string `toml:"cups,omitempty"` // Optional: comma-separated list of CUPS to filter
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
		// Registry creates datasource with nil config first; defer validation until SetConfig
		cfg = &Config{}
	} else {
		var ok bool
		cfg, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for Datadis datasource")
		}
		// Only validate when an explicit config struct is provided
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
	}

	return &Datasource{
		config:       cfg,
		client:       nil, // Will be initialized on first fetch
		instanceName: instanceName,
		supplies:     nil, // Will be fetched on first fetch
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
	l := log.ForService("datadis:" + d.instanceName)
	l.Debugf("Fetching Datadis consumption data for the current month")

	// Initialize client and fetch supplies if not already done
	if d.client == nil {
		d.client = datadis.NewClient()

		if err := d.client.Login(d.config.Username, d.config.Password); err != nil {
			return fmt.Errorf("failed to login to Datadis: %w", err)
		}

		supplies, err := d.client.Supplies()
		if err != nil {
			return fmt.Errorf("failed to fetch supplies: %w", err)
		}

		if len(supplies) == 0 {
			return fmt.Errorf("no electricity supplies found for this account")
		}

		d.supplies = supplies
		l.Debugf("Datadis datasource initialized with %d supplies", len(supplies))
	}

	// Get current date and beginning of month
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)

	// Datadis API expects YYYY/MM format, and both dates should be in the same month
	// The API will return all data from the beginning of the month until today
	l.Debugf("Fetching consumption data from %s to %s",
		startOfMonth.Format("2006/01/02"),
		now.Format("2006/01/02"))

	blockCount := 0

	// Filter supplies if CUPS is configured
	suppliesToFetch := d.supplies
	if d.config.CUPS != "" {
		suppliesToFetch = d.filterSuppliesByConfig()
		l.Debugf("Filtering to %d supplies based on config", len(suppliesToFetch))
	}

	for _, supply := range suppliesToFetch {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		l.Debugf("Fetching consumption data for supply %s (%s)", supply.Cups, supply.Address)

		// Fetch consumption data for this supply
		// Using the same month for both from and to parameters
		measurements, err := d.client.ConsumptionData(&supply, startOfMonth, now)
		if err != nil {
			l.Warnf("Failed to fetch consumption data for supply %s: %v", supply.Cups, err)
			continue
		}

		l.Debugf("Got %d measurements for supply %s", len(measurements), supply.Cups)

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
				l.Warnf("Failed to parse measurement time: %v", err)
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

	l.Debugf("Fetched %d consumption blocks from Datadis", blockCount)
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

// filterSuppliesByConfig filters supplies based on configured CUPS
func (d *Datasource) filterSuppliesByConfig() []datadis.Supply {
	if d.config.CUPS == "" {
		return d.supplies
	}

	// Parse comma-separated CUPS list
	configuredCUPS := make(map[string]bool)
	for _, cups := range splitAndTrim(d.config.CUPS, ",") {
		configuredCUPS[cups] = true
	}

	var filtered []datadis.Supply
	for _, supply := range d.supplies {
		if configuredCUPS[supply.Cups] {
			filtered = append(filtered, supply)
		}
	}

	return filtered
}

// splitAndTrim splits a string by delimiter and trims whitespace
func splitAndTrim(s, delimiter string) []string {
	var result []string
	for _, item := range splitString(s, delimiter) {
		trimmed := trimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitString splits a string by delimiter
func splitString(s, delimiter string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(delimiter) <= len(s) && s[i:i+len(delimiter)] == delimiter {
			result = append(result, s[start:i])
			start = i + len(delimiter)
			i += len(delimiter) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

// trimSpace removes leading and trailing whitespace
func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

// Factory creates a new instance of the datasource
func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
