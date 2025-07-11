package gasstations

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/gasdb/pkg/api"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("gasstations", prototype)
}

type Config struct {
	Latitude  float64 `toml:"latitude"`
	Longitude float64 `toml:"longitude"`
	Radius    float64 `toml:"radius"` // radius in meters
}

func (c *Config) Validate() error {
	if c.Latitude == 0 && c.Longitude == 0 {
		return fmt.Errorf("latitude and longitude must be specified")
	}
	if c.Latitude < -90 || c.Latitude > 90 {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if c.Longitude < -180 || c.Longitude > 180 {
		return fmt.Errorf("longitude must be between -180 and 180")
	}
	if c.Radius <= 0 {
		c.Radius = 10000 // Default to 10km if not specified
	}
	return nil
}

type Datasource struct {
	config       *Config
	client       *api.FuelPriceAPI
	instanceName string
}

func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var gasConfig *Config
	if config == nil {
		gasConfig = &Config{
			Latitude:  41.4847, // El Masnou default
			Longitude: 2.3199,  // El Masnou default
			Radius:    10000,   // 10km default
		}
	} else {
		var ok bool
		gasConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for gas stations datasource")
		}
	}

	client := api.NewFuelPriceAPI()

	return &Datasource{
		config:       gasConfig,
		client:       client,
		instanceName: instanceName,
	}, nil
}

func (d *Datasource) Type() string {
	return "gasstations"
}

func (d *Datasource) Name() string {
	return d.instanceName
}

func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"station_id": "TEXT",
		"name":       "TEXT",
		"address":    "TEXT",
		"locality":   "TEXT",
		"province":   "TEXT",
		"latitude":   "REAL",
		"longitude":  "REAL",
		"schedule":   "TEXT",
		"gasoline95": "TEXT",
		"diesel":     "TEXT",
		"gasoline98": "TEXT",
		"biodiesel":  "TEXT",
		"distance":   "REAL",
	}
}

func (d *Datasource) BlockPrototype() core.Block {
	return &GasStationBlock{}
}

func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

func (d *Datasource) SetConfig(config interface{}) error {
	if cfg, ok := config.(*Config); ok {
		d.config = cfg
		return cfg.Validate()
	}
	return fmt.Errorf("invalid config type for gas stations datasource")
}

func (d *Datasource) GetConfig() interface{} {
	return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	log.Printf("Fetching gas stations near lat: %.4f, lng: %.4f, radius: %.0fm",
		d.config.Latitude, d.config.Longitude, d.config.Radius)

	stations, err := d.client.NearbyPrices(d.config.Latitude, d.config.Longitude, d.config.Radius)
	if err != nil {
		return fmt.Errorf("fetching nearby gas stations: %w", err)
	}

	log.Printf("Found %d gas stations within %.0fm radius", len(stations), d.config.Radius)

	stationCount := 0
	now := time.Now()

	for _, station := range stations {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		block, err := d.convertStationToBlock(*station, now)
		if err != nil {
			log.Printf("Failed to convert gas station: %v", err)
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case blockCh <- block:
			stationCount++
		}
	}

	log.Printf("Successfully processed %d gas stations", stationCount)
	return nil
}

func (d *Datasource) convertStationToBlock(station api.GasStation, createdAt time.Time) (core.Block, error) {
	// Parse latitude and longitude (replace Spanish comma decimal separator with dot)
	latStr := strings.Replace(station.Latitud, ",", ".", -1)
	latitude, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		latitude = 0.0
	}

	lngStr := strings.Replace(station.Longitud, ",", ".", -1)
	longitude, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		longitude = 0.0
	}

	// Distance is already calculated by gasdb NearbyPrices
	distance := 0.0 // Will be set by gasdb if available

	// Use instance name as source instead of type
	sourceName := d.instanceName
	if sourceName == "" {
		sourceName = "gasstations" // fallback
	}

	// Extract prices, handling empty strings
	gasoline95 := station.PrecioGasolina95E5
	diesel := station.PrecioGasoleoA
	gasoline98 := station.PrecioGasolina98E5
	biodiesel := station.PrecioBiodiesel

	// Clean up empty prices
	if gasoline95 == "" {
		gasoline95 = "0"
	}
	if diesel == "" {
		diesel = "0"
	}
	if gasoline98 == "" {
		gasoline98 = "0"
	}
	if biodiesel == "" {
		biodiesel = "0"
	}

	block := NewGasStationBlockWithSource(
		station.IDEESS,
		station.Rotulo,
		station.Direccion,
		station.Localidad,
		station.Provincia,
		latitude,
		longitude,
		station.Horario,
		gasoline95,
		diesel,
		gasoline98,
		biodiesel,
		distance,
		createdAt,
		sourceName,
	)

	return block, nil
}

func (d *Datasource) Close() error {
	return nil
}

func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
