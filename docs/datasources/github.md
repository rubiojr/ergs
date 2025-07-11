# GitHub Datasource

The GitHub datasource fetches your public GitHub activity feed, allowing you to index and search your GitHub events such as commits, pull requests, issues, and repository interactions.

## Overview

This datasource connects to GitHub's public activity API to retrieve:
- Repository events (pushes, forks, stars)
- Issue and pull request activity
- Repository creation and collaboration
- Public activity timeline

The datasource can work without authentication for public data, but providing a token increases rate limits and may provide access to more events.

## Configuration

### Basic Configuration (Public Data)

```toml
[datasources.github-activity]
type = 'github'

[datasources.github-activity.config]
# Optional: Leave empty for public data only
token = ""
```

### Authenticated Configuration

```toml
[datasources.github-activity]
type = 'github'

[datasources.github-activity.config]
token = "ghp_your_personal_access_token_here"
language = "Go"  # Optional: filter by programming language
```

### Configuration Fields

- `token` (optional): GitHub personal access token for higher rate limits
- `language` (optional): Filter events by programming language

## GitHub Token Setup

To create a personal access token:

1. Go to GitHub Settings → Developer settings → Personal access tokens
2. Click "Generate new token (classic)"
3. Select scopes:
   - `read:user` - Read user profile information
   - `public_repo` - Access public repositories (if you want more detailed repo info)
4. Copy the generated token to your configuration

**Note**: The token is optional. Without it, you'll get public data with lower rate limits.

## Language Filtering

You can filter events to only include repositories that use a specific programming language:

```toml
[datasources.github-work]
type = 'github'

[datasources.github-work.config]
token = "your_token"
language = "Python"  # Only show Python repositories

[datasources.github-personal]
type = 'github'

[datasources.github-personal.config]
token = "your_token"
language = "JavaScript"  # Only show JavaScript repositories
```

## Usage

Once configured, the GitHub datasource works with all standard ergs commands:

```bash
# Fetch GitHub activity
ergs fetch

# Search your GitHub activity
ergs search --query "pull request"

# List recent GitHub events
ergs list --datasource github-activity --limit 10
```

## Data Fields

Each GitHub event includes:
- **event_type**: Type of GitHub event (PushEvent, IssuesEvent, etc.)
- **actor_login**: GitHub username who performed the action
- **repo_name**: Repository name where the event occurred
- **repo_url**: URL to the repository
- **repo_description**: Repository description
- **language**: Primary programming language of the repository
- **stars**: Number of stars the repository has
- **forks**: Number of forks the repository has
- **public**: Whether the repository is public
- **payload**: Raw event data from GitHub

## Rate Limits

- **Without token**: 60 requests per hour
- **With token**: 5,000 requests per hour

The datasource automatically respects GitHub's rate limits and will wait between requests when necessary.

## Troubleshooting

### "API rate limit exceeded"
- Add a personal access token to increase rate limits
- Wait for the rate limit to reset (check GitHub's response headers)
- Reduce fetch frequency if running automated fetches

### "Token authentication failed"
- Verify your token is correct and hasn't expired
- Check that the token has the necessary scopes
- Regenerate the token if needed

### Empty results
- Verify your GitHub username has public activity
- Check that you have recent GitHub activity (events are limited to recent activity)
- Try running without language filtering to see all events

### "Repository not found" warnings
- Some events may reference deleted or private repositories
- These warnings are normal and can be ignored
- The datasource will continue processing other events