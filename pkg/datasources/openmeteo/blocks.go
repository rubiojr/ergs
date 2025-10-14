package openmeteo

import (
	"fmt"
	"math"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

type WeatherBlock struct {
	id        string
	text      string
	createdAt time.Time
	source    string
	metadata  map[string]interface{}

	// Weather specific fields
	location            string
	country             string
	latitude            float64
	longitude           float64
	timezone            string
	population          int64
	temperature         float64
	windSpeed           float64
	windDirection       float64
	weatherCode         int
	weatherDescription  string
	humidity            float64
	apparentTemperature float64
	surfacePressure     float64
	sealevelPressure    float64
	uvIndex             float64
	hourlyForecast      []map[string]interface{}
}

func NewWeatherBlock(
	location, country string,
	latitude, longitude float64,
	timezone string,
	population int64,
	temperature, windSpeed, windDirection float64,
	weatherCode int,
	weatherDescription string,
	humidity, apparentTemperature, surfacePressure, sealevelPressure, uvIndex float64,
	hourlyForecast []map[string]interface{},
	createdAt time.Time,
	source string,
) *WeatherBlock {
	// Create searchable text combining all relevant fields
	text := fmt.Sprintf("%s %s weather %s temperature %.1f°C wind %.1f km/h humidity %.1f%%",
		location, country, weatherDescription, temperature, windSpeed, humidity)

	metadata := map[string]interface{}{
		"location":             location,
		"country":              country,
		"latitude":             latitude,
		"longitude":            longitude,
		"timezone":             timezone,
		"population":           population,
		"temperature":          temperature,
		"wind_speed":           windSpeed,
		"wind_direction":       windDirection,
		"weather_code":         weatherCode,
		"weather_description":  weatherDescription,
		"humidity":             humidity,
		"apparent_temperature": apparentTemperature,
		"surface_pressure":     surfacePressure,
		"sealevel_pressure":    sealevelPressure,
		"uv_index":             uvIndex,
		"source":               source,
		"hourly_forecast":      hourlyForecast,
	}

	// Use content-based ID so we only create new blocks when weather changes significantly
	blockID := fmt.Sprintf("weather-%s-%.1f-%.1f-%d",
		location, temperature, windSpeed, weatherCode)

	return &WeatherBlock{
		id:                  blockID,
		text:                text,
		createdAt:           createdAt,
		source:              source,
		metadata:            metadata,
		location:            location,
		country:             country,
		latitude:            latitude,
		longitude:           longitude,
		timezone:            timezone,
		population:          population,
		temperature:         temperature,
		windSpeed:           windSpeed,
		windDirection:       windDirection,
		weatherCode:         weatherCode,
		weatherDescription:  weatherDescription,
		humidity:            humidity,
		apparentTemperature: apparentTemperature,
		surfacePressure:     surfacePressure,
		sealevelPressure:    sealevelPressure,
		uvIndex:             uvIndex,
		hourlyForecast:      hourlyForecast,
	}
}

// Implement core.Block interface
func (b *WeatherBlock) ID() string                       { return b.id }
func (b *WeatherBlock) Text() string                     { return b.text }
func (b *WeatherBlock) CreatedAt() time.Time             { return b.createdAt }
func (b *WeatherBlock) Source() string                   { return b.source }
func (b *WeatherBlock) Metadata() map[string]interface{} { return b.metadata }

func (b *WeatherBlock) PrettyText() string {
	metadataInfo := core.FormatMetadata(b.metadata)
	return fmt.Sprintf("🌤️  %s, %s\n  🌡️  Temperature: %.1f°C (feels like %.1f°C)\n  💨 Wind: %.1f km/h at %.0f°\n  ☁️  Condition: %s\n  💧 Humidity: %.1f%%\n  🔆 UV Index: %.0f\n  📊 Pressure: %.1f hPa (surface), %.1f hPa (sea level)\n  📍 Coordinates: %.4f, %.4f\n  🕒 Timezone: %s\n  📅 %s%s",
		b.location,
		b.country,
		b.temperature,
		b.apparentTemperature,
		b.windSpeed,
		b.windDirection,
		b.weatherDescription,
		b.humidity,
		math.Round(b.uvIndex),
		b.surfacePressure,
		b.sealevelPressure,
		b.latitude,
		b.longitude,
		b.timezone,
		b.createdAt.Format("2006-01-02 15:04:05"),
		metadataInfo)
}

// Summary returns a concise one-line summary of the weather
func (b *WeatherBlock) Summary() string {
	return fmt.Sprintf("🌤️  %s: %.1f°C, %s", b.location, b.temperature, b.weatherDescription)
}

// Custom accessor methods
func (b *WeatherBlock) Location() string                         { return b.location }
func (b *WeatherBlock) Country() string                          { return b.country }
func (b *WeatherBlock) Latitude() float64                        { return b.latitude }
func (b *WeatherBlock) Longitude() float64                       { return b.longitude }
func (b *WeatherBlock) Timezone() string                         { return b.timezone }
func (b *WeatherBlock) Population() int64                        { return b.population }
func (b *WeatherBlock) Temperature() float64                     { return b.temperature }
func (b *WeatherBlock) WindSpeed() float64                       { return b.windSpeed }
func (b *WeatherBlock) WindDirection() float64                   { return b.windDirection }
func (b *WeatherBlock) WeatherCode() int                         { return b.weatherCode }
func (b *WeatherBlock) WeatherDescription() string               { return b.weatherDescription }
func (b *WeatherBlock) Humidity() float64                        { return b.humidity }
func (b *WeatherBlock) ApparentTemperature() float64             { return b.apparentTemperature }
func (b *WeatherBlock) SurfacePressure() float64                 { return b.surfacePressure }
func (b *WeatherBlock) SealevelPressure() float64                { return b.sealevelPressure }
func (b *WeatherBlock) UVIndex() float64                         { return b.uvIndex }
func (b *WeatherBlock) HourlyForecast() []map[string]interface{} { return b.hourlyForecast }

func (b *WeatherBlock) Type() string { return "openmeteo" }

// Factory creates a new WeatherBlock from a GenericBlock and source.
// This method is part of the core.Block interface and enables reconstruction
// from database data without requiring separate factory objects.
func (b *WeatherBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	metadata := genericBlock.Metadata()
	location := getStringFromMetadata(metadata, "location", "Unknown")
	country := getStringFromMetadata(metadata, "country", "Unknown")
	latitude := getFloatFromMetadata(metadata, "latitude", 0.0)
	longitude := getFloatFromMetadata(metadata, "longitude", 0.0)
	timezone := getStringFromMetadata(metadata, "timezone", "UTC")
	population := getIntFromMetadata(metadata, "population", 0)
	temperature := getFloatFromMetadata(metadata, "temperature", 0.0)
	windSpeed := getFloatFromMetadata(metadata, "wind_speed", 0.0)
	windDirection := getFloatFromMetadata(metadata, "wind_direction", 0.0)
	weatherCode := getIntFromMetadata(metadata, "weather_code", 0)
	weatherDescription := getStringFromMetadata(metadata, "weather_description", "Unknown")
	humidity := getFloatFromMetadata(metadata, "humidity", 0.0)
	apparentTemperature := getFloatFromMetadata(metadata, "apparent_temperature", 0.0)
	surfacePressure := getFloatFromMetadata(metadata, "surface_pressure", 0.0)
	sealevelPressure := getFloatFromMetadata(metadata, "sealevel_pressure", 0.0)
	uvIndex := getFloatFromMetadata(metadata, "uv_index", 0.0)
	hourlyForecast := getSliceFromMetadata(metadata, "hourly_forecast")

	return &WeatherBlock{
		id:                  genericBlock.ID(),
		text:                genericBlock.Text(),
		createdAt:           genericBlock.CreatedAt(),
		source:              source,
		metadata:            metadata,
		location:            location,
		country:             country,
		latitude:            latitude,
		longitude:           longitude,
		timezone:            timezone,
		population:          int64(population),
		temperature:         temperature,
		windSpeed:           windSpeed,
		windDirection:       windDirection,
		weatherCode:         weatherCode,
		weatherDescription:  weatherDescription,
		humidity:            humidity,
		apparentTemperature: apparentTemperature,
		surfacePressure:     surfacePressure,
		sealevelPressure:    sealevelPressure,
		uvIndex:             uvIndex,
		hourlyForecast:      hourlyForecast,
	}
}

// BlockFactory implements the BlockFactory interface for Weather
type BlockFactory struct{}

func (f *BlockFactory) CreateFromGeneric(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) core.Block {
	location := getStringFromMetadata(metadata, "location", "Unknown")
	country := getStringFromMetadata(metadata, "country", "Unknown")
	latitude := getFloatFromMetadata(metadata, "latitude", 0.0)
	longitude := getFloatFromMetadata(metadata, "longitude", 0.0)
	timezone := getStringFromMetadata(metadata, "timezone", "UTC")
	population := getIntFromMetadata(metadata, "population", 0)
	temperature := getFloatFromMetadata(metadata, "temperature", 0.0)
	windSpeed := getFloatFromMetadata(metadata, "wind_speed", 0.0)
	windDirection := getFloatFromMetadata(metadata, "wind_direction", 0.0)
	weatherCode := getIntFromMetadata(metadata, "weather_code", 0)
	weatherDescription := getStringFromMetadata(metadata, "weather_description", "Unknown")
	humidity := getFloatFromMetadata(metadata, "humidity", 0.0)
	apparentTemperature := getFloatFromMetadata(metadata, "apparent_temperature", 0.0)
	surfacePressure := getFloatFromMetadata(metadata, "surface_pressure", 0.0)
	sealevelPressure := getFloatFromMetadata(metadata, "sealevel_pressure", 0.0)
	uvIndex := getFloatFromMetadata(metadata, "uv_index", 0.0)
	hourlyForecast := getSliceFromMetadata(metadata, "hourly_forecast")

	return &WeatherBlock{
		id:                  id,
		text:                text,
		createdAt:           createdAt,
		source:              source,
		metadata:            metadata,
		location:            location,
		country:             country,
		latitude:            latitude,
		longitude:           longitude,
		timezone:            timezone,
		population:          int64(population),
		temperature:         temperature,
		windSpeed:           windSpeed,
		windDirection:       windDirection,
		weatherCode:         weatherCode,
		weatherDescription:  weatherDescription,
		humidity:            humidity,
		apparentTemperature: apparentTemperature,
		surfacePressure:     surfacePressure,
		sealevelPressure:    sealevelPressure,
		uvIndex:             uvIndex,
		hourlyForecast:      hourlyForecast,
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

func getSliceFromMetadata(metadata map[string]interface{}, key string) []map[string]interface{} {
	if value, exists := metadata[key]; exists {
		if slice, ok := value.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(slice))
			for _, item := range slice {
				if m, ok := item.(map[string]interface{}); ok {
					result = append(result, m)
				}
			}
			return result
		}
		if slice, ok := value.([]map[string]interface{}); ok {
			return slice
		}
	}
	return []map[string]interface{}{}
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

func getIntFromMetadata(metadata map[string]interface{}, key string, defaultValue int) int {
	if value, exists := metadata[key]; exists {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case float32:
			return int(v)
		}
	}
	return defaultValue
}
