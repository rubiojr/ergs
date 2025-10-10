# Datadis Importer

Import monthly electricity consumption data from Datadis JSON files into Ergs.

## Overview

This importer reads monthly aggregated consumption data from Datadis JSON files and imports them into Ergs via the importer API. The data includes consumption broken down by tariff periods (Valle/Valley, Llano/Flat, Punta/Peak).

## Prerequisites

1. Ergs importer API server running (`ergs importer`)
2. Datadis datasource configured in Ergs
3. Monthly consumption JSON file from Datadis

## JSON File Format

The importer expects a JSON file with the following structure:

```json
[
    {
        "CUPS": "ES0031408160870037YV0F",
        "Fecha": "2025/01",
        "Valle": "506,251",
        "Llano": "178,738",
        "Punta": "210,791",
        "Energia_vertida_kWh": "",
        "Energia_generada_kWh": "",
        "Energia_autoconsumida_kWh": "",
        "Consumo_Anual": ""
    }
]
```

**Field Descriptions:**
- `CUPS`: Electricity supply point identifier
- `Fecha`: Month in YYYY/MM format
- `Valle`: Valley tariff consumption (off-peak) in kWh
- `Llano`: Flat tariff consumption (mid-peak) in kWh
- `Punta`: Peak tariff consumption in kWh

Note: The importer handles Spanish decimal format (comma as separator).

## Building

```bash
cd importers/datadis-importer
go build -o datadis-importer
```

## Usage

### Basic Usage

```bash
./datadis-importer \
  --file Consumptions-2025.json \
  --api-key your-api-key
```

### All Options

```bash
./datadis-importer \
  --file Consumptions-2025.json \
  --api-key your-api-key \
  --importer-url http://localhost:9090 \
  --target-datasource datadis \
  --batch-size 50 \
  --dry-run
```

**Options:**
- `--file`: Path to Datadis JSON file (required)
- `--api-key`: API key for authentication (required unless --dry-run)
- `--importer-url`: URL of importer API server (default: http://localhost:9090)
- `--target-datasource`: Target datasource name (default: datadis)
- `--batch-size`: Number of blocks per request (default: 50)
- `--dry-run`: Show what would be imported without sending data

## How It Works

1. Reads the JSON file containing monthly consumption records
2. Parses each record and converts Spanish decimal format (comma → dot)
3. Creates consumption blocks with:
   - Monthly aggregated total consumption
   - Individual tariff period values (Valle, Llano, Punta)
   - Proper metadata matching the datadis datasource schema
4. Sends blocks to the importer API in batches
5. Reports acceptance/rejection statistics

## Example Workflow

1. **Start the importer API:**
   ```bash
   ergs importer --port 9090
   ```

2. **Get your API key** from the logs or configuration

3. **Run a dry-run** to verify the data:
   ```bash
   ./datadis-importer --file Consumptions-2025.json --dry-run
   ```

4. **Import the data:**
   ```bash
   ./datadis-importer --file Consumptions-2025.json --api-key YOUR_KEY
   ```

5. **Verify import** in Ergs:
   ```bash
   ergs search --datasource datadis --query "Valle"
   ```

## Block Format

Each monthly record creates one block with:

- **ID**: `monthly-{CUPS}-{YYYY/MM}`
- **Type**: `datadis`
- **Date**: First day of the month
- **Metadata**:
  - `cups`: CUPS identifier
  - `date`: Month (YYYY/MM)
  - `hour`: "00:00" (placeholder for monthly aggregate)
  - `consumption`: Total monthly consumption (Valle + Llano + Punta)
  - `obtain_method`: "monthly_aggregate"
  - `valle`: Valley tariff consumption
  - `llano`: Flat tariff consumption
  - `punta`: Peak tariff consumption

## Troubleshooting

### "API returned status 401"
- Check your API key is correct
- Ensure the importer API server is running

### "Failed to parse JSON"
- Verify the JSON file format matches the expected structure
- Check for valid JSON syntax

### "Invalid Valle/Llano/Punta value"
- The importer expects Spanish decimal format (comma as separator)
- Verify numeric values are present and properly formatted

## Notes

- Monthly aggregated data is imported as single blocks per month
- The importer handles Spanish number format (comma decimal separator)
- Empty fields for generated/exported energy are ignored
- Blocks use the first day of the month as the timestamp
- The `obtain_method` is set to "monthly_aggregate" to distinguish from hourly data