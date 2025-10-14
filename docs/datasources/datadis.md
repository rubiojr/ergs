# Datadis Datasource

The Datadis datasource fetches electricity consumption data from the [Datadis platform](https://datadis.es), which provides access to electricity consumption information for Spanish electricity supplies.

## Overview

This datasource connects to the Datadis API to retrieve hourly electricity consumption measurements for all supplies associated with your account. It fetches data for the current month (from the beginning of the month until today) and is designed to run once per day.

## Configuration

Add the following to your `config.toml`:

```toml
[datasources.datadis]
type = 'datadis'
interval = '24h0m0s'  # Fetch once per day (default: 24h)

[datasources.datadis.config]
username = 'your-datadis-username'  # Required
password = 'your-datadis-password'  # Required
cups = ''  # Optional: Comma-separated list of CUPS to filter
```

### Configuration Parameters

- **username** (required): Your Datadis account username
- **password** (required): Your Datadis account password
- **cups** (optional): Comma-separated list of CUPS identifiers to filter which supplies to fetch. If not set or empty, fetches data for all supplies in the account. Example: `'ES0021000000000001AA,ES0021000000000002BB'`
- **interval**: How often to fetch data (default: 24h). Since consumption data is typically updated daily, fetching more frequently is not recommended.

## Data Collection

The datasource:
1. Authenticates with the Datadis API using your credentials
2. Retrieves all electricity supplies associated with your account
3. Filters supplies based on the `cups` configuration (if specified)
4. For each supply, fetches hourly consumption data for the current month
5. Creates a block for each hourly measurement

## Block Structure

Each consumption measurement creates a block with the following metadata:

- **cups**: The CUPS (Código Universal del Punto de Suministro) identifier
- **date**: The date of the measurement (YYYY/MM/DD format)
- **hour**: The hour of the measurement (00-23)
- **consumption**: The electricity consumption in kWh
- **obtain_method**: How the measurement was obtained (real, estimated, etc.)
- **address**: The supply point address
- **province**: The province where the supply is located
- **postal_code**: The postal code
- **municipality**: The municipality
- **distributor**: The electricity distributor company

## Example Block

A typical consumption block looks like:

```
⚡ Electricity Consumption
📅 2025/01/15 at 14:00
📊 1.23 kWh
📍 Calle Example 123
   Madrid, Madrid 28001
🏢 i-DE Redes Eléctricas Inteligentes
🔌 CUPS: ES0021000000000000XX
📝 Method: R
```

## Search Examples

You can search for consumption data using the FTS5 query syntax:

```sql
-- Find all consumption over 2 kWh
consumption:">2"

-- Find consumption for a specific date
date:"2025/01/15"

-- Find consumption by address
address:"Calle Example"

-- Find consumption by CUPS
cups:"ES0021000000000000XX"
```

## Prerequisites

1. A Datadis account (register at https://datadis.es)
2. Your electricity supply must be registered in the platform
3. Your electricity distributor must be providing data to Datadis

## Notes

- The datasource fetches data for the current month only to minimize API calls
- Consumption data is typically available with a 1-2 day delay
- The first fetch may take longer if you have multiple supplies or a full month of data
- Rate limiting is implemented to avoid overwhelming the Datadis API

## Troubleshooting

### No supplies found
If you get "no electricity supplies found for this account", ensure:
- Your Datadis account is properly configured
- You have added your electricity supply in the Datadis web interface
- Your distributor is providing data to Datadis

### Authentication failures
If authentication fails:
- Verify your username and password are correct
- Check if you can log in to the Datadis website
- Ensure your account is active and not locked

### Missing data
If consumption data is missing:
- Data is typically available with a 1-2 day delay
- Some distributors may have longer delays
- Check the Datadis website to confirm data availability