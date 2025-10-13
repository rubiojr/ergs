# Open-Meteo Weather Datasource

The Open-Meteo datasource fetches current weather data for any location worldwide using the free Open-Meteo API. It automatically geocodes location names and provides comprehensive weather information including temperature, wind, humidity, pressure, and UV index.

## Features

- 🌍 **Worldwide Coverage** - Fetch weather for any location globally
- 📍 **Automatic Geocoding** - Simply provide a location name (city, town, etc.)
- 🌡️ **Comprehensive Data** - Temperature, feels-like, wind, humidity, pressure, UV index
- 🎨 **Weather Icons** - Visual representation with emoji icons for each weather condition
- 🆓 **Free API** - No API key required, uses the free Open-Meteo service
- 🔄 **Real-time Updates** - Configurable fetch intervals for up-to-date weather

## Configuration

### Basic Configuration

```toml
[datasources.madrid_weather]
type = 'openmeteo'
interval = '1h0m0s'  # Check weather every hour
[datasources.madrid_weather.config]
location = 'Madrid'  # Location name to geocode
```

### Example: Multiple Locations

```toml
[datasources.london_weather]
type = 'openmeteo'
interval = '30m0s'
[datasources.london_weather.config]
location = 'London'

[datasources.tokyo_weather]
type = 'openmeteo'
interval = '1h0m0s'
[datasources.tokyo_weather.config]
location = 'Tokyo'

[datasources.newyork_weather]
type = 'openmeteo'
interval = '1h0m0s'
[datasources.newyork_weather.config]
location = 'New York'
```

### Configuration Options

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `location` | string | Yes | - | Location name to fetch weather for (e.g., "Madrid", "San Francisco") |

### Validation Rules

- Location must be a non-empty string
- The location will be automatically geocoded using Open-Meteo's geocoding API
- If multiple matches are found, the first (most relevant) result is used

## Data Schema

The datasource stores the following fields for each weather report:

| Field | Type | Description |
|-------|------|-------------|
| `location` | TEXT | City/location name |
| `country` | TEXT | Country name |
| `latitude` | REAL | Location latitude |
| `longitude` | REAL | Location longitude |
| `timezone` | TEXT | Timezone identifier (e.g., "Europe/Madrid") |
| `population` | INTEGER | Population of the location (if available) |
| `temperature` | REAL | Current temperature (°C) |
| `wind_speed` | REAL | Wind speed (km/h) |
| `wind_direction` | REAL | Wind direction (degrees) |
| `weather_code` | INTEGER | WMO weather code (0-99) |
| `weather_description` | TEXT | Human-readable weather condition |
| `humidity` | REAL | Relative humidity (%) |
| `apparent_temperature` | REAL | Feels-like temperature (°C) |
| `surface_pressure` | REAL | Surface pressure (hPa) |
| `sealevel_pressure` | REAL | Sea level pressure (hPa) |
| `uv_index` | REAL | Maximum UV index for the day |

## Weather Codes and Icons

The datasource uses WMO weather codes and displays them with appropriate icons:

| Code | Condition | Icon |
|------|-----------|------|
| 0 | Clear Sky | ☀️ |
| 1 | Mainly Clear | 🌤️ |
| 2 | Partly Cloudy | ⛅ |
| 3 | Overcast | ☁️ |
| 45, 48 | Fog | 🌫️ |
| 51-57 | Drizzle | 🌧️ |
| 61-67 | Rain | 🌧️ |
| 71-77 | Snow | ❄️ |
| 80-82 | Rain Showers | 🌦️ |
| 85-86 | Snow Showers | 🌨️ |
| 95-99 | Thunderstorm | ⛈️ |

## Usage Examples

### Fetching Data

```bash
# One-time fetch
./ergs fetch --config config.toml

# Stream results to see them in real-time
./ergs fetch --stream --config config.toml
```

### Searching Weather Data

```bash
# Search for a specific location
./ergs search --query "Madrid" --datasource madrid_weather

# Search for weather conditions
./ergs search --query "rain" --datasource madrid_weather

# Search by temperature
./ergs search --query "temperature" --datasource madrid_weather

# Search across all weather datasources
./ergs search --query "sunny"
```

### Running the Scheduler

```bash
# Start the daemon to fetch weather data at configured intervals
./ergs serve --config config.toml
```

## Sample Output

When fetching or streaming data, you'll see output like this:

```
🌤️  Madrid, Spain
  🌡️  Temperature: 22.5°C (feels like 21.8°C)
  💨 Wind: 12.3 km/h at 180°
  ☁️  Condition: Partly Cloudy
  💧 Humidity: 65.2%
  🔆 UV Index: 6
  📊 Pressure: 1013.2 hPa (surface), 1013.5 hPa (sea level)
  📍 Coordinates: 40.4168, -3.7038
  🕒 Timezone: Europe/Madrid
  📅 2024-01-15 10:30:00
  Metadata:
    location: Madrid
    country: Spain
    latitude: 40.4168
    longitude: -3.7038
    timezone: Europe/Madrid
    temperature: 22.5
    wind_speed: 12.3
    weather_description: Partly Cloudy
    humidity: 65.2
    uv_index: 6
```

## Data Source

This datasource uses the free [Open-Meteo API](https://open-meteo.com/):

- **Geocoding**: `geocoding-api.open-meteo.com` - Converts location names to coordinates
- **Weather Data**: `api.open-meteo.com` - Provides current weather and forecasts
- **Air Quality**: `air-quality-api.open-meteo.com` - UV index and air quality data
- **No API Key Required**: Free for non-commercial use
- **Attribution**: Data provided by Open-Meteo.com

## Location Examples

You can use various location formats:

```toml
# City names
location = 'Tokyo'
location = 'Paris'
location = 'New York'

# City with state/region
location = 'San Francisco, California'
location = 'Cambridge, Massachusetts'

# Smaller towns
location = 'El Masnou'
location = 'Zermatt'

# International locations
location = 'São Paulo'
location = 'München'
location = 'København'
```

## Performance Considerations

- **API Rate Limits**: Open-Meteo is free but has reasonable rate limits
- **Update Frequency**: Weather data typically updates every 15-60 minutes
- **Recommended Interval**: 30 minutes to 1 hour is usually sufficient
- **Storage**: Creates one block per fetch, deduplicates based on weather conditions
- **Network**: Requires 3 API calls per fetch (geocoding, weather, air quality)

## Troubleshooting

### Common Issues

1. **Location not found**: Check spelling and try adding country name
   ```toml
   location = 'Springfield, USA'  # More specific
   ```

2. **API timeout errors**: Check internet connection or increase timeout
   - Default timeout is 30 seconds
   - API may be temporarily unavailable

3. **No weather data**: Verify location is valid by testing in browser:
   ```
   https://geocoding-api.open-meteo.com/v1/search?name=YourLocation
   ```

### Debug Tips

```bash
# Test with a well-known location first
location = 'London'

# Check logs for geocoding results
./ergs serve --config config.toml --verbose

# Verify API accessibility
curl "https://api.open-meteo.com/v1/forecast?latitude=51.5074&longitude=-0.1278&current=temperature_2m"
```

## Integration with Ergs

The Open-Meteo datasource integrates seamlessly with Ergs' features:

- **Scheduling**: Configure different update intervals per location
- **Search**: Full-text search across location, conditions, and weather data
- **Storage**: Automatic deduplication prevents storing duplicate weather reports
- **Streaming**: Real-time weather updates during fetch operations
- **Web Interface**: Beautiful visual rendering with weather icons and gradients

## Privacy and Terms

- **No Personal Data**: Only location names are sent to Open-Meteo API
- **No API Key**: No authentication required
- **Terms of Use**: Follow Open-Meteo's [terms of service](https://open-meteo.com/en/terms)
- **Attribution**: Credit Open-Meteo when sharing weather data publicly

## Advanced Usage

### Combining with Other Datasources

Track weather alongside your other data:

```toml
# Weather for your location
[datasources.local_weather]
type = 'openmeteo'
interval = '30m0s'
[datasources.local_weather.config]
location = 'Your City'

# Track when weather changes affect your activities
[datasources.github]
type = 'github'
interval = '30m0s'
[datasources.github.config]
token = 'your-token'
```

### Multiple Locations Monitoring

Monitor weather in different cities:

```toml
[datasources.home_weather]
type = 'openmeteo'
interval = '30m0s'
[datasources.home_weather.config]
location = 'San Francisco'

[datasources.office_weather]
type = 'openmeteo'
interval = '30m0s'
[datasources.office_weather.config]
location = 'London'

[datasources.vacation_weather]
type = 'openmeteo'
interval = '1h0m0s'
[datasources.vacation_weather.config]
location = 'Barcelona'
```

This makes it easy to track weather patterns across multiple locations and correlate with your other activities and data sources.