package gasstations

import (
	"fmt"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

type GasStationBlock struct {
	id        string
	text      string
	createdAt time.Time
	source    string
	metadata  map[string]interface{}

	// Gas station specific fields
	stationID  string
	name       string
	address    string
	locality   string
	province   string
	latitude   float64
	longitude  float64
	schedule   string
	gasoline95 string
	diesel     string
	gasoline98 string
	biodiesel  string
	distance   float64
}

func NewGasStationBlock(
	stationID, name, address, locality, province string,
	latitude, longitude float64,
	schedule, gasoline95, diesel, gasoline98, biodiesel string,
	distance float64,
	createdAt time.Time,
) *GasStationBlock {
	return NewGasStationBlockWithSource(
		stationID, name, address, locality, province,
		latitude, longitude,
		schedule, gasoline95, diesel, gasoline98, biodiesel,
		distance, createdAt, "gasstations",
	)
}

func NewGasStationBlockWithSource(
	stationID, name, address, locality, province string,
	latitude, longitude float64,
	schedule, gasoline95, diesel, gasoline98, biodiesel string,
	distance float64,
	createdAt time.Time,
	source string,
) *GasStationBlock {
	// Create searchable text combining all relevant fields
	text := fmt.Sprintf("%s %s %s %s %s gasoline95 %s diesel %s",
		name, address, locality, province, schedule, gasoline95, diesel)

	metadata := map[string]interface{}{
		"station_id": stationID,
		"name":       name,
		"address":    address,
		"locality":   locality,
		"province":   province,
		"latitude":   latitude,
		"longitude":  longitude,
		"schedule":   schedule,
		"gasoline95": gasoline95,
		"diesel":     diesel,
		"gasoline98": gasoline98,
		"biodiesel":  biodiesel,
		"distance":   distance,
		"source":     source,
	}

	blockID := fmt.Sprintf("gasstation-%s-%d", stationID, createdAt.Unix())

	return &GasStationBlock{
		id:         blockID,
		text:       text,
		createdAt:  createdAt,
		source:     source,
		metadata:   metadata,
		stationID:  stationID,
		name:       name,
		address:    address,
		locality:   locality,
		province:   province,
		latitude:   latitude,
		longitude:  longitude,
		schedule:   schedule,
		gasoline95: gasoline95,
		diesel:     diesel,
		gasoline98: gasoline98,
		biodiesel:  biodiesel,
		distance:   distance,
	}
}

// Implement core.Block interface
func (b *GasStationBlock) ID() string                       { return b.id }
func (b *GasStationBlock) Text() string                     { return b.text }
func (b *GasStationBlock) CreatedAt() time.Time             { return b.createdAt }
func (b *GasStationBlock) Source() string                   { return b.source }
func (b *GasStationBlock) Metadata() map[string]interface{} { return b.metadata }

func (b *GasStationBlock) PrettyText() string {
	var prices []string

	if b.gasoline95 != "" && b.gasoline95 != "0" {
		prices = append(prices, fmt.Sprintf("95: %s€/L", b.gasoline95))
	}
	if b.diesel != "" && b.diesel != "0" {
		prices = append(prices, fmt.Sprintf("Diesel: %s€/L", b.diesel))
	}
	if b.gasoline98 != "" && b.gasoline98 != "0" {
		prices = append(prices, fmt.Sprintf("98: %s€/L", b.gasoline98))
	}
	if b.biodiesel != "" && b.biodiesel != "0" {
		prices = append(prices, fmt.Sprintf("Biodiesel: %s€/L", b.biodiesel))
	}

	priceText := "No prices available"
	if len(prices) > 0 {
		priceText = strings.Join(prices, ", ")
	}

	distanceText := ""
	if b.distance > 0 {
		if b.distance < 1000 {
			distanceText = fmt.Sprintf(" (%.0fm away)", b.distance)
		} else {
			distanceText = fmt.Sprintf(" (%.1fkm away)", b.distance/1000)
		}
	}

	schedule := b.schedule
	if schedule == "" {
		schedule = "Schedule not available"
	}

	metadataInfo := core.FormatMetadata(b.metadata)
	return fmt.Sprintf("⛽ %s%s\n  📍 %s, %s, %s\n  💰 %s\n  🕒 %s\n  📅 %s%s",
		b.name,
		distanceText,
		b.address,
		b.locality,
		b.province,
		priceText,
		schedule,
		b.createdAt.Format("2006-01-02 15:04:05"),
		metadataInfo)
}

// Summary returns a concise one-line summary of the gas station.
func (b *GasStationBlock) Summary() string {
	name := b.name

	var prices []string
	if b.gasoline95 != "" && b.gasoline95 != "0" {
		prices = append(prices, fmt.Sprintf("95: %s€", b.gasoline95))
	}
	if b.diesel != "" && b.diesel != "0" {
		prices = append(prices, fmt.Sprintf("D: %s€", b.diesel))
	}

	priceInfo := ""
	if len(prices) > 0 {
		priceInfo = fmt.Sprintf(" (%s)", strings.Join(prices, ", "))
	}

	return fmt.Sprintf("⛽ %s%s", name, priceInfo)
}

// Custom accessor methods
func (b *GasStationBlock) StationID() string  { return b.stationID }
func (b *GasStationBlock) Name() string       { return b.name }
func (b *GasStationBlock) Address() string    { return b.address }
func (b *GasStationBlock) Locality() string   { return b.locality }
func (b *GasStationBlock) Province() string   { return b.province }
func (b *GasStationBlock) Latitude() float64  { return b.latitude }
func (b *GasStationBlock) Longitude() float64 { return b.longitude }
func (b *GasStationBlock) Schedule() string   { return b.schedule }
func (b *GasStationBlock) Gasoline95() string { return b.gasoline95 }
func (b *GasStationBlock) Diesel() string     { return b.diesel }
func (b *GasStationBlock) Gasoline98() string { return b.gasoline98 }
func (b *GasStationBlock) Biodiesel() string  { return b.biodiesel }
func (b *GasStationBlock) Distance() float64  { return b.distance }

func (b *GasStationBlock) Type() string { return "gasstations" }

// Factory creates a new GasStationBlock from a GenericBlock and source.
// This method is part of the core.Block interface and enables reconstruction
// from database data without requiring separate factory objects.
func (b *GasStationBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	metadata := genericBlock.Metadata()
	stationID := getStringFromMetadata(metadata, "station_id", "unknown")
	name := getStringFromMetadata(metadata, "name", "Unknown Station")
	address := getStringFromMetadata(metadata, "address", "")
	locality := getStringFromMetadata(metadata, "locality", "")
	province := getStringFromMetadata(metadata, "province", "")
	latitude := getFloatFromMetadata(metadata, "latitude", 0.0)
	longitude := getFloatFromMetadata(metadata, "longitude", 0.0)
	schedule := getStringFromMetadata(metadata, "schedule", "")
	gasoline95 := getStringFromMetadata(metadata, "gasoline95", "")
	diesel := getStringFromMetadata(metadata, "diesel", "")
	gasoline98 := getStringFromMetadata(metadata, "gasoline98", "")
	biodiesel := getStringFromMetadata(metadata, "biodiesel", "")
	distance := getFloatFromMetadata(metadata, "distance", 0.0)

	return &GasStationBlock{
		id:         genericBlock.ID(),
		text:       genericBlock.Text(),
		createdAt:  genericBlock.CreatedAt(),
		source:     source,
		metadata:   metadata,
		stationID:  stationID,
		name:       name,
		address:    address,
		locality:   locality,
		province:   province,
		latitude:   latitude,
		longitude:  longitude,
		schedule:   schedule,
		gasoline95: gasoline95,
		diesel:     diesel,
		gasoline98: gasoline98,
		biodiesel:  biodiesel,
		distance:   distance,
	}
}

// BlockFactory implements the BlockFactory interface for Gas Stations
type BlockFactory struct{}

func (f *BlockFactory) CreateFromGeneric(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) core.Block {
	stationID := getStringFromMetadata(metadata, "station_id", "unknown")
	name := getStringFromMetadata(metadata, "name", "Unknown Station")
	address := getStringFromMetadata(metadata, "address", "")
	locality := getStringFromMetadata(metadata, "locality", "")
	province := getStringFromMetadata(metadata, "province", "")
	latitude := getFloatFromMetadata(metadata, "latitude", 0.0)
	longitude := getFloatFromMetadata(metadata, "longitude", 0.0)
	schedule := getStringFromMetadata(metadata, "schedule", "")
	gasoline95 := getStringFromMetadata(metadata, "gasoline95", "")
	diesel := getStringFromMetadata(metadata, "diesel", "")
	gasoline98 := getStringFromMetadata(metadata, "gasoline98", "")
	biodiesel := getStringFromMetadata(metadata, "biodiesel", "")
	distance := getFloatFromMetadata(metadata, "distance", 0.0)

	return &GasStationBlock{
		id:         id,
		text:       text,
		createdAt:  createdAt,
		source:     source,
		metadata:   metadata,
		stationID:  stationID,
		name:       name,
		address:    address,
		locality:   locality,
		province:   province,
		latitude:   latitude,
		longitude:  longitude,
		schedule:   schedule,
		gasoline95: gasoline95,
		diesel:     diesel,
		gasoline98: gasoline98,
		biodiesel:  biodiesel,
		distance:   distance,
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

func getFloatFromMetadata(metadata map[string]interface{}, key string, defaultValue float64) float64 {
	if value, exists := metadata[key]; exists {
		switch v := value.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return defaultValue
}
