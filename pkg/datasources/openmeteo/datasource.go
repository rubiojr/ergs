package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/log"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("openmeteo", prototype)
}

type Config struct {
	Location string `toml:"location"` // Location name to geocode (e.g., "Madrid", "New York")
}

func (c *Config) Validate() error {
	if c.Location == "" {
		return fmt.Errorf("location must be specified")
	}
	return nil
}

type Datasource struct {
	config       *Config
	client       *http.Client
	instanceName string
}

// GeocodingResult represents the geocoding API response
type GeocodingResult struct {
	Results []struct {
		Name       string  `json:"name"`
		Country    string  `json:"country"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
		Timezone   string  `json:"timezone"`
		Population float64 `json:"population"`
	} `json:"results"`
}

// WeatherResult represents the forecast API response
type WeatherResult struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
	Current   struct {
		Time          string  `json:"time"`
		Temperature   float64 `json:"temperature_2m"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		WindDirection float64 `json:"wind_direction_10m"`
		WeatherCode   int     `json:"weather_code"`
	} `json:"current"`
	Hourly struct {
		Time                []string  `json:"time"`
		RelativeHumidity    []float64 `json:"relative_humidity_2m"`
		ApparentTemperature []float64 `json:"apparent_temperature"`
		SurfacePressure     []float64 `json:"surface_pressure"`
		PressureMSL         []float64 `json:"pressure_msl"`
	} `json:"hourly"`
}

// AirQualityResult represents the air quality API response
type AirQualityResult struct {
	Hourly struct {
		Time    []string  `json:"time"`
		UVIndex []float64 `json:"uv_index"`
	} `json:"hourly"`
}

func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var weatherConfig *Config
	if config == nil {
		// Registry creates datasource with nil config first; defer validation until SetConfig
		weatherConfig = &Config{}
	} else {
		var ok bool
		weatherConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for openmeteo datasource")
		}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &Datasource{
		config:       weatherConfig,
		client:       client,
		instanceName: instanceName,
	}, nil
}

func (d *Datasource) Type() string {
	return "openmeteo"
}

func (d *Datasource) Name() string {
	return d.instanceName
}

func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"location":             "TEXT",
		"country":              "TEXT",
		"latitude":             "REAL",
		"longitude":            "REAL",
		"timezone":             "TEXT",
		"population":           "INTEGER",
		"temperature":          "REAL",
		"wind_speed":           "REAL",
		"wind_direction":       "REAL",
		"weather_code":         "INTEGER",
		"weather_description":  "TEXT",
		"humidity":             "REAL",
		"apparent_temperature": "REAL",
		"surface_pressure":     "REAL",
		"sealevel_pressure":    "REAL",
		"uv_index":             "REAL",
	}
}

func (d *Datasource) BlockPrototype() core.Block {
	return &WeatherBlock{}
}

func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

func (d *Datasource) SetConfig(config interface{}) error {
	if cfg, ok := config.(*Config); ok {
		d.config = cfg
		return cfg.Validate()
	}
	return fmt.Errorf("invalid config type for openmeteo datasource")
}

func (d *Datasource) GetConfig() interface{} {
	return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	l := log.ForService("openmeteo:" + d.instanceName)
	l.Debugf("Fetching weather data for location: %s", d.config.Location)

	// Step 1: Geocode the location
	geoData, err := d.geocodeLocation(ctx)
	if err != nil {
		return fmt.Errorf("geocoding location: %w", err)
	}

	if len(geoData.Results) == 0 {
		return fmt.Errorf("no results found for location '%s'. Try a simpler location name like just the city (e.g., 'Madrid' instead of 'Madrid, Spain')", d.config.Location)
	}

	location := geoData.Results[0]
	l.Debugf("Found location: %s, %s (%.4f, %.4f)", location.Name, location.Country, location.Latitude, location.Longitude)

	// Step 2: Fetch weather data
	weatherData, err := d.fetchWeather(ctx, location.Latitude, location.Longitude)
	if err != nil {
		return fmt.Errorf("fetching weather data: %w", err)
	}

	// Step 3: Fetch air quality data
	airQualityData, err := d.fetchAirQuality(ctx, location.Latitude, location.Longitude)
	if err != nil {
		return fmt.Errorf("fetching air quality data: %w", err)
	}

	// Step 4: Create block
	now := time.Now().UTC()
	block := d.createWeatherBlock(location, weatherData, airQualityData, now)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case blockCh <- block:
		l.Debugf("Successfully processed weather data for %s", location.Name)
	}

	return nil
}

func (d *Datasource) geocodeLocation(ctx context.Context) (*GeocodingResult, error) {
	l := log.ForService("openmeteo:" + d.instanceName)
	geocodingURL := "https://geocoding-api.open-meteo.com/v1/search"
	params := url.Values{}
	params.Add("name", d.config.Location)
	params.Add("count", "1")

	reqURL := fmt.Sprintf("%s?%s", geocodingURL, params.Encode())
	l.Debugf("Geocoding location: %s (URL: %s)", d.config.Location, reqURL)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geocoding API returned status %d", resp.StatusCode)
	}

	var result GeocodingResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (d *Datasource) fetchWeather(ctx context.Context, lat, lon float64) (*WeatherResult, error) {
	forecastURL := "https://api.open-meteo.com/v1/forecast"
	params := url.Values{}
	params.Add("latitude", fmt.Sprintf("%.4f", lat))
	params.Add("longitude", fmt.Sprintf("%.4f", lon))
	params.Add("current", "temperature_2m,wind_speed_10m,wind_direction_10m,weather_code")
	params.Add("hourly", "relative_humidity_2m,apparent_temperature,surface_pressure,pressure_msl")
	params.Add("timezone", "auto")

	reqURL := fmt.Sprintf("%s?%s", forecastURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather API returned status %d", resp.StatusCode)
	}

	var result WeatherResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (d *Datasource) fetchAirQuality(ctx context.Context, lat, lon float64) (*AirQualityResult, error) {
	airQualityURL := "https://air-quality-api.open-meteo.com/v1/air-quality"
	params := url.Values{}
	params.Add("latitude", fmt.Sprintf("%.4f", lat))
	params.Add("longitude", fmt.Sprintf("%.4f", lon))
	params.Add("hourly", "uv_index")
	params.Add("timezone", "auto")

	reqURL := fmt.Sprintf("%s?%s", airQualityURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("air quality API returned status %d", resp.StatusCode)
	}

	var result AirQualityResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (d *Datasource) createWeatherBlock(
	location struct {
		Name       string  `json:"name"`
		Country    string  `json:"country"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
		Timezone   string  `json:"timezone"`
		Population float64 `json:"population"`
	},
	weather *WeatherResult,
	airQuality *AirQualityResult,
	createdAt time.Time,
) core.Block {
	// Get current hour's data
	currentHour := time.Now().Hour()

	humidity := 0.0
	apparentTemp := 0.0
	surfacePressure := 0.0
	sealevelPressure := 0.0

	if currentHour < len(weather.Hourly.RelativeHumidity) {
		humidity = weather.Hourly.RelativeHumidity[currentHour]
	}
	if currentHour < len(weather.Hourly.ApparentTemperature) {
		apparentTemp = weather.Hourly.ApparentTemperature[currentHour]
	}
	if currentHour < len(weather.Hourly.SurfacePressure) {
		surfacePressure = weather.Hourly.SurfacePressure[currentHour]
	}
	if currentHour < len(weather.Hourly.PressureMSL) {
		sealevelPressure = weather.Hourly.PressureMSL[currentHour]
	}

	// Calculate max UV index for today
	maxUVIndex := 0.0
	if len(airQuality.Hourly.UVIndex) > 0 {
		for i := 0; i < len(airQuality.Hourly.UVIndex) && i < 24; i++ {
			if airQuality.Hourly.UVIndex[i] > maxUVIndex {
				maxUVIndex = airQuality.Hourly.UVIndex[i]
			}
		}
	}

	weatherDesc := translateWeatherCode(weather.Current.WeatherCode)

	sourceName := d.instanceName
	if sourceName == "" {
		sourceName = "openmeteo"
	}

	return NewWeatherBlock(
		location.Name,
		location.Country,
		location.Latitude,
		location.Longitude,
		location.Timezone,
		int64(location.Population),
		weather.Current.Temperature,
		weather.Current.WindSpeed,
		weather.Current.WindDirection,
		weather.Current.WeatherCode,
		weatherDesc,
		humidity,
		apparentTemp,
		surfacePressure,
		sealevelPressure,
		maxUVIndex,
		createdAt,
		sourceName,
	)
}

func (d *Datasource) Close() error {
	return nil
}

func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}

func translateWeatherCode(code int) string {
	switch code {
	case 0:
		return "Clear Sky"
	case 1:
		return "Mainly Clear"
	case 2:
		return "Partly Cloudy"
	case 3:
		return "Overcast"
	case 45:
		return "Fog"
	case 48:
		return "Depositing Rime Fog"
	case 51:
		return "Light Drizzle"
	case 53:
		return "Moderate Drizzle"
	case 55:
		return "Dense Drizzle"
	case 56:
		return "Light Freezing Drizzle"
	case 57:
		return "Dense Freezing Drizzle"
	case 61:
		return "Slight Rain"
	case 63:
		return "Moderate Rain"
	case 65:
		return "Heavy Rain"
	case 66:
		return "Light Freezing Rain"
	case 67:
		return "Heavy Freezing Rain"
	case 71:
		return "Slight Snow Fall"
	case 73:
		return "Moderate Snow Fall"
	case 75:
		return "Heavy Snow Fall"
	case 77:
		return "Snow Grains"
	case 80:
		return "Slight Rain Showers"
	case 81:
		return "Moderate Rain Showers"
	case 82:
		return "Violent Rain Showers"
	case 85:
		return "Slight Snow Showers"
	case 86:
		return "Heavy Snow Showers"
	case 95:
		return "Thunderstorm"
	case 96:
		return "Thunderstorm With Light Hail"
	case 99:
		return "Thunderstorm With Heavy Hail"
	default:
		return "Unknown"
	}
}
