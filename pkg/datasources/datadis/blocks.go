package datadis

import (
	"fmt"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// ConsumptionBlock represents an electricity consumption measurement
type ConsumptionBlock struct {
	id           string
	text         string
	cups         string
	date         string
	hour         string
	consumption  float32
	obtainMethod string
	createdAt    time.Time
	source       string
	metadata     map[string]interface{}

	// Supply metadata
	address      string
	province     string
	postalCode   string
	municipality string
	distributor  string
}

// NewConsumptionBlock creates a new ConsumptionBlock
func NewConsumptionBlock(
	id string,
	cups string,
	date string,
	hour string,
	consumption float32,
	obtainMethod string,
	address string,
	province string,
	postalCode string,
	municipality string,
	distributor string,
	createdAt time.Time,
	source string,
) *ConsumptionBlock {
	text := fmt.Sprintf(
		"Electricity consumption: %.2f kWh | Date: %s Hour: %s | CUPS: %s | Address: %s | Municipality: %s | Province: %s | Distributor: %s",
		consumption, date, hour, cups, address, municipality, province, distributor,
	)

	metadata := map[string]interface{}{
		"cups":          cups,
		"date":          date,
		"hour":          hour,
		"consumption":   consumption,
		"obtain_method": obtainMethod,
		"address":       address,
		"province":      province,
		"postal_code":   postalCode,
		"municipality":  municipality,
		"distributor":   distributor,
	}

	return &ConsumptionBlock{
		id:           id,
		text:         text,
		cups:         cups,
		date:         date,
		hour:         hour,
		consumption:  consumption,
		obtainMethod: obtainMethod,
		address:      address,
		province:     province,
		postalCode:   postalCode,
		municipality: municipality,
		distributor:  distributor,
		createdAt:    createdAt,
		source:       source,
		metadata:     metadata,
	}
}

// ID returns the unique identifier for this block
func (b *ConsumptionBlock) ID() string {
	return b.id
}

// Text returns searchable text content for the block
func (b *ConsumptionBlock) Text() string {
	return b.text
}

// CreatedAt returns the creation timestamp
func (b *ConsumptionBlock) CreatedAt() time.Time {
	return b.createdAt
}

// Source returns the datasource name
func (b *ConsumptionBlock) Source() string {
	return b.source
}

// Type returns the datasource type
func (b *ConsumptionBlock) Type() string {
	return "datadis"
}

// Metadata returns structured metadata for the block
func (b *ConsumptionBlock) Metadata() map[string]interface{} {
	return b.metadata
}

// PrettyText returns a human-readable formatted version of the block
func (b *ConsumptionBlock) PrettyText() string {
	var sb strings.Builder

	sb.WriteString("⚡ Electricity Consumption\n")
	sb.WriteString(fmt.Sprintf("📅 %s at %s:00\n", b.date, b.hour))
	sb.WriteString(fmt.Sprintf("📊 %.2f kWh\n", b.consumption))

	if b.address != "" {
		sb.WriteString(fmt.Sprintf("📍 %s\n", b.address))
		if b.municipality != "" && b.province != "" {
			sb.WriteString(fmt.Sprintf("   %s, %s %s\n", b.municipality, b.province, b.postalCode))
		}
	}

	if b.distributor != "" {
		sb.WriteString(fmt.Sprintf("🏢 %s\n", b.distributor))
	}

	sb.WriteString(fmt.Sprintf("🔌 CUPS: %s\n", b.cups))

	if b.obtainMethod != "" {
		sb.WriteString(fmt.Sprintf("📝 Method: %s\n", b.obtainMethod))
	}

	// Format metadata using utility function
	metadataInfo := core.FormatMetadata(b.metadata)
	sb.WriteString(metadataInfo)

	return sb.String()
}

// Summary returns a concise one-line summary of the consumption block
func (b *ConsumptionBlock) Summary() string {
	return fmt.Sprintf("⚡ %.2f kWh on %s at %s:00", b.consumption, b.date, b.hour)
}

// Factory creates a new ConsumptionBlock from a GenericBlock and source
func (b *ConsumptionBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	metadata := genericBlock.Metadata()
	cups := getStringFromMetadata(metadata, "cups", "")
	date := getStringFromMetadata(metadata, "date", "")
	hour := getStringFromMetadata(metadata, "hour", "")
	consumption := getFloatFromMetadata(metadata, "consumption", 0.0)
	obtainMethod := getStringFromMetadata(metadata, "obtain_method", "")
	address := getStringFromMetadata(metadata, "address", "")
	province := getStringFromMetadata(metadata, "province", "")
	postalCode := getStringFromMetadata(metadata, "postal_code", "")
	municipality := getStringFromMetadata(metadata, "municipality", "")
	distributor := getStringFromMetadata(metadata, "distributor", "")

	return &ConsumptionBlock{
		id:           genericBlock.ID(),
		text:         genericBlock.Text(),
		cups:         cups,
		date:         date,
		hour:         hour,
		consumption:  float32(consumption),
		obtainMethod: obtainMethod,
		address:      address,
		province:     province,
		postalCode:   postalCode,
		municipality: municipality,
		distributor:  distributor,
		createdAt:    genericBlock.CreatedAt(),
		source:       source,
		metadata:     metadata,
	}
}

// CUPS returns the CUPS identifier
func (b *ConsumptionBlock) CUPS() string {
	return b.cups
}

// Date returns the date of the measurement
func (b *ConsumptionBlock) Date() string {
	return b.date
}

// Hour returns the hour of the measurement
func (b *ConsumptionBlock) Hour() string {
	return b.hour
}

// Consumption returns the consumption in kWh
func (b *ConsumptionBlock) Consumption() float32 {
	return b.consumption
}

// ObtainMethod returns how the measurement was obtained
func (b *ConsumptionBlock) ObtainMethod() string {
	return b.obtainMethod
}

// Address returns the supply point address
func (b *ConsumptionBlock) Address() string {
	return b.address
}

// Province returns the province
func (b *ConsumptionBlock) Province() string {
	return b.province
}

// PostalCode returns the postal code
func (b *ConsumptionBlock) PostalCode() string {
	return b.postalCode
}

// Municipality returns the municipality
func (b *ConsumptionBlock) Municipality() string {
	return b.municipality
}

// Distributor returns the distributor name
func (b *ConsumptionBlock) Distributor() string {
	return b.distributor
}

// Helper functions for safe metadata extraction
func getStringFromMetadata(metadata map[string]interface{}, key, defaultValue string) string {
	if val, exists := metadata[key]; exists {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

func getFloatFromMetadata(metadata map[string]interface{}, key string, defaultValue float64) float64 {
	if val, exists := metadata[key]; exists {
		switch v := val.(type) {
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
