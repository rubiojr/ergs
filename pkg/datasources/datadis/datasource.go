package datadis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
func NewDatasource(instanceName string, config any) (core.Datasource, error) {
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
func (d *Datasource) ConfigType() any {
	return &Config{}
}

// SetConfig updates the datasource configuration
func (d *Datasource) SetConfig(config any) error {
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
func (d *Datasource) GetConfig() any {
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

		// Datadis repeats the "02:00" label on the day clocks go back, so
		// count how many times each date/time label has been seen.
		seen := make(map[string]int)

		for _, measurement := range measurements {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			labelKey := measurement.Date + " " + measurement.Time
			seen[labelKey]++
			occurrence := seen[labelKey]

			measurementTime, err := parseMeasurementTime(measurement.Date, measurement.Time, occurrence, datadisLocation)
			if err != nil {
				l.Warnf("Skipping measurement for supply %s: %v", supply.Cups, err)
				continue
			}

			// Create unique block ID. The repeated DST hour gets a suffix so
			// it doesn't overwrite the first one.
			blockID := fmt.Sprintf("consumption-%s-%s-%s",
				supply.Cups,
				measurement.Date,
				measurement.Time)
			if occurrence > 1 {
				blockID = fmt.Sprintf("%s-%d", blockID, occurrence)
			}

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

// datadisLocation is the time zone Datadis reports measurements in.
var datadisLocation = loadDatadisLocation()

func loadDatadisLocation() *time.Location {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		return time.Local
	}
	return loc
}

// parseMeasurementTime returns the start of the hour a Datadis measurement
// covers.
//
// Datadis labels hourly measurements with date "YYYY/MM/DD" and time "HH:MM",
// where the time is the wall-clock *end* of the hour, from "01:00" to "24:00":
// "01:00" is consumption from 00:00 to 00:59, "24:00" from 23:00 to 23:59.
// On the day clocks go forward "02:00" is missing; on the day they go back
// "02:00" appears twice, and occurrence (1 or 2) says which one this is.
// A bare "HH" is accepted too.
func parseMeasurementTime(date, hourLabel string, occurrence int, loc *time.Location) (time.Time, error) {
	day, err := time.Parse("2006/01/02", date)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid measurement date %q: %w", date, err)
	}

	hh, mm, err := parseHourLabel(hourLabel)
	if err != nil {
		return time.Time{}, err
	}

	// The wall clock the interval ends at; normalising in UTC turns "24:00"
	// into 00:00 the next day without any DST interference.
	want := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, time.UTC)

	// A wall-clock time maps to two instants during the hour clocks go back.
	// Collect every instant near it that shows that wall clock, in order.
	guess := time.Date(want.Year(), want.Month(), want.Day(), want.Hour(), want.Minute(), 0, 0, loc)
	var matches []time.Time
	for _, c := range []time.Time{guess.Add(-time.Hour), guess, guess.Add(time.Hour)} {
		if sameWallClock(c.In(loc), want) {
			matches = append(matches, c)
		}
	}

	if occurrence < 1 {
		occurrence = 1
	}
	var end time.Time
	switch {
	case len(matches) == 0:
		// A wall-clock time skipped by DST; Datadis shouldn't send it, but
		// time.Date's normalisation is the best reading of it.
		end = guess
	case occurrence > len(matches):
		return time.Time{}, fmt.Errorf("measurement %s %s seen %d times, expected at most %d",
			date, hourLabel, occurrence, len(matches))
	default:
		end = matches[occurrence-1]
	}

	return end.Add(-time.Hour), nil
}

// parseHourLabel parses a Datadis time label, "HH:MM" or "HH", allowing hours
// up to 24 and only "24:00" at the top.
func parseHourLabel(label string) (int, int, error) {
	hourPart, minPart, hasMinutes := strings.Cut(strings.TrimSpace(label), ":")
	hh, err := strconv.Atoi(hourPart)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid measurement time %q", label)
	}
	mm := 0
	if hasMinutes {
		mm, err = strconv.Atoi(minPart)
		if err != nil || len(minPart) != 2 {
			return 0, 0, fmt.Errorf("invalid measurement time %q", label)
		}
	}
	if hh < 0 || hh > 24 || mm < 0 || mm > 59 || (hh == 24 && mm != 0) {
		return 0, 0, fmt.Errorf("measurement time %q out of range", label)
	}
	return hh, mm, nil
}

func sameWallClock(t, want time.Time) bool {
	return t.Year() == want.Year() && t.Month() == want.Month() && t.Day() == want.Day() &&
		t.Hour() == want.Hour() && t.Minute() == want.Minute()
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
func (d *Datasource) Factory(instanceName string, config any) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
