# FTS5 Query Syntax Guide

This guide covers the full-text search query syntax supported by ergs. All queries use SQLite's FTS5 syntax, which provides powerful search capabilities across your indexed data.

## Basic Queries

### Simple Terms
```
golang
rust
"gas station"
```

### Multiple Terms (Implicit AND)
```
golang project
python web framework
fuel price data
```

## Column Filters

Ergs stores data in several searchable columns:
- `text` - The main searchable content
- `source` - The datasource instance name (e.g., "github-work", "firefox-home")  
- `datasource` - The datasource type (e.g., "github", "firefox", "gasstations")
- `metadata` - Structured metadata as JSON
- `hostname` - The hostname where the data was collected (e.g., "workstation", "server01")

### Basic Column Syntax
```
datasource:github
datasource:gasstations
datasource:firefox
source:github-work
text:golang
metadata:important
hostname:workstation
```

### Quoted Column Names
```
"datasource":github
"text":"hello world"
"source":"github-work"
"hostname":"workstation"
```

## Boolean Operators

### AND Operator
```
golang AND rust
datasource:github AND text:project
python AND (web OR api)
```

### OR Operator
```
python OR golang
datasource:github OR datasource:codeberg
(rust OR golang) AND project
```

### NOT Operator
```
golang NOT deprecated
datasource:github NOT archived
project NOT (test OR demo)
```

## Required Terms (+) and Exclusions (-)

### Required Terms
Use `+` to require specific terms to be present:
```
one + two + three
golang + project + active
```

### Excluded Terms  
Use `-` to exclude terms:
```
golang - deprecated
project - archived - demo
```

### Limitations
❌ **These don't work as expected:**
```
datasource:github + golang
"datasource":github + one + two
text:one + text:two
```

✅ **Use these alternatives instead:**
```
datasource:github AND golang
datasource:github golang
datasource:github AND (one + two)
text:one AND text:two
```

## Phrase Queries

### Exact Phrases
```
"gas station"
"golang web framework"
"open source project"
```

### Column-Specific Phrases
```
text:"hello world"
datasource:"gasstations"
source:"github-work"
```

## Proximity Queries (NEAR)

Find terms within a certain distance of each other:

### Basic NEAR
```
golang NEAR rust
price NEAR fuel
```

### NEAR with Distance
```
golang NEAR/5 project
fuel NEAR/3 price
```

## Wildcard Queries

### Prefix Matching
```
prog*
fuel*
git*
```

### Column Prefix Matching
```
text:prog*
datasource:git*
```

## Grouping with Parentheses

### Basic Grouping
```
(golang OR rust) AND project
datasource:(github OR codeberg)
text:(fuel OR gas) AND price
```

### Complex Grouping
```
(datasource:github OR datasource:codeberg) AND (golang OR rust)
text:(price OR cost) AND datasource:gasstations
(source:github-work OR source:github-personal) AND active
```

## Real-World Examples

### Find GitHub Projects
```
datasource:github
datasource:github AND golang
datasource:github AND text:project
```

### Search Gas Station Data
```
datasource:gasstations
datasource:gasstations AND text:diesel
datasource:gasstations AND (fuel OR gas OR diesel)
```

### Search Browser History
```
datasource:firefox
datasource:firefox AND github
text:documentation AND datasource:firefox
```

### Search by Hostname
```
hostname:workstation
hostname:server01
hostname:laptop AND datasource:github
```

### Cross-Datasource Searches
```
(datasource:github OR datasource:codeberg) AND rust
golang NOT datasource:firefox
project AND (datasource:github OR datasource:codeberg)
```

### Multi-Machine Searches
```
hostname:workstation OR hostname:server01
datasource:github AND hostname:workstation
hostname:laptop AND NOT hostname:server01
```

### Advanced Combinations
```
datasource:github AND text:"open source" AND rust
(datasource:gasstations AND diesel) OR (datasource:github AND fuel)
text:(price OR cost) AND NOT (test OR demo)
hostname:workstation AND datasource:github AND rust
(hostname:server01 OR hostname:server02) AND datasource:gasstations
```

## Special Considerations

### Case Sensitivity
FTS5 searches are **case-insensitive** by default:
```
GOLANG = golang = GoLang
```

### Tokenization
Text is tokenized by words, punctuation is generally ignored:
```
"hello-world" matches "hello world"
"api.github.com" matches "api github com"
```

### Empty Queries
Empty queries return recent results:
```
# No query = show recent items
```

## Query Performance Tips

1. **Use column filters** when possible to narrow search scope:
   ```
   datasource:github golang    # Better
   golang                      # Broader search
   ```

2. **Phrase queries** are more precise:
   ```
   "exact phrase"              # More precise
   exact phrase                # Matches "exact" AND "phrase" anywhere
   ```

3. **Wildcard queries** can be slower:
   ```
   prog*                       # Can be slow
   program                     # Faster
   ```

## Error Handling

If a query has syntax errors, it will be treated as a literal string search. Most malformed FTS5 queries will still return results, just not with the intended logic.

### Common Issues
- Unmatched quotes: `"unclosed quote`
- Unmatched parentheses: `(unclosed group`
- Invalid operators: `term === value`

These will typically be searched as literal text rather than causing errors.