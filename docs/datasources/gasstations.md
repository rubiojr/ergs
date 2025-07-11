# Gas Stations Datasource

The Gas Stations datasource fetches current fuel prices from Spanish gas stations using the official government API. It finds stations within a specified radius from given coordinates and provides real-time pricing information for different fuel types.

## Features

- 🚗 Fetches current fuel prices from Spanish gas stations
- 📍 Finds stations within a configurable radius from specified coordinates
- ⛽ Supports multiple fuel types (Gasoline 95/98, Diesel, Biodiesel)
- 🕒 Includes station schedules and operating hours
- 📊 Provides detailed station information (name, address, coordinates)
- 🔄 Automatically calculates and displays distances

## Configuration

### Basic Configuration

```toml
[datasources.madrid_gas]
type = 'gasstations'
interval = '2h0m0s'  # Check for updates every 2 hours
[datasources.madrid_gas.config]
latitude = 40.4168   # Madrid coordinates
longitude = -3.7038  # Madrid coordinates
radius = 10000       # 10km radius in meters
```

### Example: El Masnou, Barcelona

```toml
[datasources.masnou_gas]
type = 'gasstations'
interval = '1h0m0s'  # Check for updates every hour
[datasources.masnou_gas.config]
latitude = 41.4847   # El Masnou coordinates
longitude = 2.3199   # El Masnou coordinates
radius = 5000        # 5km radius in meters
```

### Configuration Options

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `latitude` | float64 | Yes | - | Latitude of center point |
| `longitude` | float64 | Yes | - | Longitude of center point |
| `radius` | float64 | No | 10000 | Search radius in meters |

### Validation Rules

- Latitude must be between -90 and 90 degrees
- Longitude must be between -180 and 180 degrees
- Radius must be greater than 0 (defaults to 10km if not specified)

## Data Schema

The datasource stores the following fields for each gas station:

| Field | Type | Description |
|-------|------|-------------|
| `station_id` | TEXT | Unique station identifier from government API |
| `name` | TEXT | Station brand/name (e.g., "REPSOL", "BP") |
| `address` | TEXT | Full street address |
| `locality` | TEXT | City/town name |
| `province` | TEXT | Province name |
| `latitude` | REAL | Station latitude coordinate |
| `longitude` | REAL | Station longitude coordinate |
| `schedule` | TEXT | Operating hours |
| `gasoline95` | TEXT | Gasoline 95 price (€/L) |
| `diesel` | TEXT | Diesel price (€/L) |
| `gasoline98` | TEXT | Gasoline 98 price (€/L) |
| `biodiesel` | TEXT | Biodiesel price (€/L) |
| `distance` | REAL | Distance from center point (meters) |

## Usage Examples

### Fetching Data

```bash
# One-time fetch
./ergs fetch --config config.toml

# Stream results to see them in real-time
./ergs fetch --stream --config config.toml
```

### Searching Gas Stations

```bash
# Search for REPSOL stations
./ergs search --query "REPSOL" --datasource masnou_gas

# Search for stations in a specific town
./ergs search --query "Masnou" --datasource masnou_gas

# Search for diesel prices
./ergs search --query "diesel" --datasource masnou_gas

# Search across all datasources
./ergs search --query "gasoline"
```

### Running the Scheduler

```bash
# Start the daemon to fetch data at configured intervals
./ergs serve --config config.toml
```

## Sample Output

When fetching or streaming data, you'll see output like this:

```
⛽ REPSOL
  📍 CARRETERA N-II KM. 635,5, MASNOU (EL), BARCELONA
  💰 95: 1,629€/L, Diesel: 1,469€/L, 98: 1,779€/L
  🕒 L-D: 06:00-22:00
  Metadata:
    station_id: 2935
    name: REPSOL
    address: CARRETERA N-II KM. 635,5
    locality: MASNOU (EL)
    province: BARCELONA
    latitude: 41.482556
    longitude: 2.333639
    schedule: L-D: 06:00-22:00
    gasoline95: 1,629
    diesel: 1,469
    gasoline98: 1,779
    biodiesel: 0
    distance: 0
```

## Data Source

This datasource uses the [gasdb library](https://github.com/rubiojr/gasdb) which fetches data from the official Spanish Ministry of Industry API:

- **Current prices**: `EstacionesTerrestres` endpoint
- **Data provider**: Spanish government
- **Update frequency**: Data is updated regularly by the government

## Popular Coordinates

Here are coordinates for some major Spanish cities:

| City | Latitude | Longitude |
|------|----------|-----------|
| Madrid | 40.4168 | -3.7038 |
| Barcelona | 41.3851 | 2.1734 |
| Valencia | 39.4699 | -0.3763 |
| Sevilla | 37.3891 | -5.9845 |
| Bilbao | 43.2627 | -2.9253 |
| Málaga | 36.7213 | -4.4214 |
| El Masnou | 41.4847 | 2.3199 |

## Performance Considerations

- **API Rate Limits**: Be respectful of the government API
- **Radius Size**: Larger radius values will return more stations but take longer to process
- **Update Frequency**: Gas prices don't change very frequently, so intervals of 1-2 hours are usually sufficient
- **Storage**: Each station creates one block per fetch, so frequent fetching will create multiple entries

## Troubleshooting

### Common Issues

1. **No stations found**: Check if your coordinates are correct and try increasing the radius
2. **API timeout errors**: The government API may be temporarily unavailable, try again later
3. **Invalid coordinates**: Ensure latitude/longitude are valid Spanish coordinates

### Debug Tips

```bash
# Test with a smaller radius first
radius = 1000  # 1km

# Verify coordinates are correct for Spain
# Latitude should be roughly 36-44 for mainland Spain
# Longitude should be roughly -9 to 3 for mainland Spain
```

## Integration with Ergs

The gas stations datasource integrates seamlessly with Ergs' features:

- **Scheduling**: Configure different update intervals per region
- **Search**: Full-text search across station names, addresses, and fuel types
- **Storage**: Automatic deduplication and efficient storage
- **Streaming**: Real-time data processing and display

This makes it easy to monitor fuel prices across multiple regions of Spain and get alerts when prices change significantly.