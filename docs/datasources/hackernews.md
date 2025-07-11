# HackerNews Datasource

The HackerNews datasource fetches stories, comments, jobs, polls, and other items from [Hacker News](https://news.ycombinator.com/) using the [official HN API](https://github.com/HackerNews/API).

## Features

- Fetches top stories, new stories, Ask HN, Show HN, and job postings
- Optionally fetches top-level comments for stories
- Configurable item limits and source types
- No authentication required
- Respects API rate limits with built-in delays

## Configuration

```toml
[datasources.hackernews]
type = 'hackernews'
interval = '1h0m0s'  # Optional: fetch every hour

[datasources.hackernews.config]
fetch_top = true        # Fetch top stories (default: true)
fetch_new = false       # Fetch new stories (default: false)
fetch_ask = false       # Fetch Ask HN stories (default: false)
fetch_show = false      # Fetch Show HN stories (default: false)
fetch_jobs = false      # Fetch job postings (default: false)
max_items = 100         # Maximum items to fetch (default: 100, max: 500)
fetch_comments = false  # Also fetch top-level comments (default: false)
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `fetch_top` | boolean | `true` | Fetch top stories from the front page |
| `fetch_new` | boolean | `false` | Fetch newest stories |
| `fetch_ask` | boolean | `false` | Fetch "Ask HN" stories |
| `fetch_show` | boolean | `false` | Fetch "Show HN" stories |
| `fetch_jobs` | boolean | `false` | Fetch job postings |
| `max_items` | integer | `100` | Maximum number of items to fetch (1-500) |
| `fetch_comments` | boolean | `false` | Fetch top-level comments for stories |

## Data Fields

Each HackerNews item contains the following metadata:

| Field | Type | Description |
|-------|------|-------------|
| `item_type` | string | Type of item: "story", "comment", "job", "poll", "pollopt" |
| `title` | string | Title of the story, job, or poll |
| `url` | string | URL of the story (if external link) |
| `author` | string | Username of the item's author |
| `score` | integer | Points/score of the item |
| `descendants` | integer | Total number of comments |
| `parent_id` | integer | Parent item ID (for comments) |
| `poll_id` | integer | Associated poll ID (for poll options) |
| `deleted` | boolean | Whether the item is deleted |
| `dead` | boolean | Whether the item is dead/killed |

## Search Examples

Search for stories about specific topics:
```
title:rust
title:golang OR title:python
author:pg
```

Find high-scoring items:
```
score >= 100
```

Search by item type:
```
item_type:job
item_type:comment
```

Combine filters:
```
item_type:story AND score >= 50 AND title:AI
```

## Usage Tips

- **Start Small**: Begin with just `fetch_top = true` to avoid overwhelming your database
- **Comments**: Only enable `fetch_comments` if you need them, as they significantly increase data volume
- **Rate Limiting**: The datasource includes built-in delays to respect HN's API
- **Duplicates**: Items may appear in multiple categories (e.g., a story in both "top" and "new")
- **Content**: Story text is indexed for full-text search, including titles and comment content

## Item Types

- **story**: Regular stories, Ask HN, and Show HN posts
- **comment**: User comments on stories
- **job**: Job postings from the jobs section
- **poll**: Poll questions
- **pollopt**: Individual poll options/answers

## Example Configuration

Minimal setup (top stories only):
```toml
[datasources.hackernews]
type = 'hackernews'
[datasources.hackernews.config]
fetch_top = true
max_items = 50
```

Comprehensive setup:
```toml
[datasources.hackernews]
type = 'hackernews'
interval = '30m0s'
[datasources.hackernews.config]
fetch_top = true
fetch_new = true
fetch_ask = true
fetch_show = true
fetch_jobs = true
max_items = 200
fetch_comments = true
```

## Notes

- The HackerNews API has no explicit rate limits but requests should be reasonable
- Items are deduplicated automatically if they appear in multiple categories
- Deleted and dead items are filtered out automatically
- The datasource respects context cancellation for graceful shutdowns