# RSS Datasource

The RSS datasource fetches articles from RSS and Atom feeds, allowing you to index and search content from blogs, news sites, and other syndicated sources.

## Features

- Supports both RSS 2.0 and Atom 1.0 feeds
- Fetches articles with titles, descriptions, links, and metadata
- Configurable item limits across all feeds
- Built-in HTML tag stripping and entity decoding
- No external dependencies (uses Go's built-in XML parser)
- Automatic date parsing with multiple format support

## Configuration

```toml
[datasources.rss]
type = 'rss'
interval = '2h0m0s'  # Optional: fetch every 2 hours

[datasources.rss.config]
urls = [
    'https://feeds.arstechnica.com/arstechnica/index',
    'https://www.phoronix.com/rss.php'
]
max_items = 50  # Maximum items to fetch across all feeds (default: 50, max: 200)
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `urls` | array | `[]` | List of RSS/Atom feed URLs to fetch |
| `max_items` | integer | `50` | Maximum total items to fetch across all feeds (1-200) |

## Data Fields

Each RSS item contains the following metadata:

| Field | Type | Description |
|-------|------|-------------|
| `feed_title` | string | Title of the RSS feed |
| `feed_url` | string | URL of the RSS feed |
| `title` | string | Article title |
| `link` | string | Link to the full article |
| `description` | string | Article summary/content (HTML stripped) |
| `author` | string | Article author |
| `category` | string | Article category/tag |
| `guid` | string | Unique identifier for the article |
| `published` | string | Original publication date string |

## Search Examples

Search for articles about specific topics:
```
title:AI OR title:machine learning
description:kubernetes
author:"John Doe"
```

Find articles from specific feeds:
```
feed_title:"Hacker News"
feed_url:cnn.com
```

Search by category:
```
category:technology
category:politics OR category:business
```

Combine filters:
```
title:golang AND feed_title:"O'Reilly Radar"
description:docker AND NOT category:advertisement
```

## Supported Feed Formats

### RSS 2.0
- Standard RSS format used by most blogs and news sites
- Extracts: title, link, description, pubDate, author, category, guid

### Atom 1.0
- Modern syndication format
- Extracts: title, link, summary, content, updated, author, id

## Usage Tips

- **Feed Selection**: Choose high-quality feeds with consistent publishing schedules
- **Item Limits**: The `max_items` is distributed across all configured feeds
- **Update Frequency**: RSS feeds typically update every few hours; adjust interval accordingly
- **Content Quality**: Some feeds provide full content, others just summaries
- **Date Parsing**: Supports multiple date formats automatically

## Popular Feed Examples

Tech News:
```toml
urls = [
    'https://feeds.arstechnica.com/arstechnica/index',
    'https://www.phoronix.com/rss.php',
    'https://feeds.feedburner.com/oreilly/radar',
    'https://hnrss.org/frontpage',
    'https://feeds.feedburner.com/TechCrunch'
]
```

Development Blogs:
```toml
urls = [
    'https://blog.golang.org/feed.atom',
    'https://kubernetes.io/feed.xml',
    'https://blog.docker.com/feed/',
    'https://aws.amazon.com/blogs/aws/feed/'
]
```

## Example Configuration

Minimal setup (uses defaults):
```toml
[datasources.rss]
type = 'rss'
[datasources.rss.config]
# Uses default URLs: Ars Technica and Phoronix
max_items = 25
```

Comprehensive tech news aggregator:
```toml
[datasources.rss]
type = 'rss'
interval = '1h0m0s'
[datasources.rss.config]
urls = [
    'https://feeds.arstechnica.com/arstechnica/index',
    'https://www.phoronix.com/rss.php',
    'https://hnrss.org/frontpage',
    'https://feeds.feedburner.com/oreilly/radar',
    'https://feeds.feedburner.com/TechCrunch'
]
max_items = 100
```

## Notes

- RSS feeds are fetched sequentially with small delays between requests
- HTML content is automatically cleaned for better full-text search
- Duplicate articles across feeds are handled by unique GUIDs/links
- The datasource respects HTTP timeouts and context cancellation
- Invalid or unreachable feeds are logged but don't stop other feeds from processing

## Troubleshooting

- **Feed not updating**: Check if the feed URL is valid and accessible
- **No items found**: Some feeds may be empty or have strict item limits
- **Date parsing issues**: Most common date formats are supported automatically
- **Content encoding**: The datasource handles HTML entities and basic tag removal