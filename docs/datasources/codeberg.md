# Codeberg Datasource

The Codeberg datasource fetches your public Codeberg activity, allowing you to index and search your interactions with Codeberg repositories, issues, and pull requests.

## Overview

This datasource connects to Codeberg's API to retrieve:
- Repository events and interactions
- Issue and pull request activity
- Repository creation and collaboration
- Public activity timeline
- Commit and push events

Codeberg is a European, non-profit alternative to GitHub that provides Git hosting and collaboration tools.

## Configuration

### Basic Configuration

```toml
[datasources.codeberg-activity]
type = 'codeberg'

[datasources.codeberg-activity.config]
username = "your_codeberg_username"
```

### Authenticated Configuration

```toml
[datasources.codeberg-activity]
type = 'codeberg'

[datasources.codeberg-activity.config]
username = "your_codeberg_username"
token = "your_access_token_here"
language = "Go"  # Optional: filter by programming language
```

### Configuration Fields

- `username` (required): Your Codeberg username
- `token` (optional): Codeberg access token for higher rate limits
- `language` (optional): Filter events by programming language

## Codeberg Token Setup

To create an access token:

1. Log into your Codeberg account
2. Go to Settings → Applications → Generate New Token
3. Select appropriate scopes:
   - `read:user` - Read user profile information
   - `read:repository` - Access repository information
4. Copy the generated token to your configuration

**Note**: The token is optional but recommended for better rate limits and more comprehensive data access.

## Language Filtering

You can filter events to only include repositories that use a specific programming language:

```toml
[datasources.codeberg-work]
type = 'codeberg'

[datasources.codeberg-work.config]
username = "workuser"
token = "your_token"
language = "Rust"  # Only show Rust repositories

[datasources.codeberg-personal]
type = 'codeberg'

[datasources.codeberg-personal.config]
username = "personaluser"
token = "your_token"
language = "Python"  # Only show Python repositories
```

## Usage

Once configured, the Codeberg datasource works with all standard ergs commands:

```bash
# Fetch Codeberg activity
ergs fetch

# Search your Codeberg activity
ergs search --query "merge request"

# List recent Codeberg events
ergs list --datasource codeberg-activity --limit 10
```

## Data Fields

Each Codeberg event includes:
- **event_type**: Type of event (push, issue, pull_request, etc.)
- **actor_login**: Codeberg username who performed the action
- **repo_name**: Repository name where the event occurred
- **repo_url**: URL to the repository
- **repo_description**: Repository description
- **language**: Primary programming language of the repository
- **stars**: Number of stars the repository has
- **forks**: Number of forks the repository has
- **public**: Whether the repository is public
- **payload**: Raw event data from Codeberg

## Rate Limits

Codeberg has rate limiting in place to ensure fair usage:
- **Without token**: Lower rate limits for anonymous access
- **With token**: Higher rate limits for authenticated users

The datasource automatically respects rate limits and will wait between requests when necessary.

## Troubleshooting

### "Rate limit exceeded"
- Add an access token to increase rate limits
- Wait for the rate limit to reset
- Reduce fetch frequency if running automated fetches

### "Authentication failed"
- Verify your token is correct and hasn't expired
- Check that the token has the necessary scopes
- Regenerate the token if needed

### "User not found"
- Verify your Codeberg username is correct
- Check that the user profile is public
- Ensure the username exists on Codeberg

### Empty results
- Verify your Codeberg account has public activity
- Check that you have recent activity (events may be limited to recent activity)
- Try running without language filtering to see all events

### Connection issues
- Verify you can access codeberg.org from your network
- Check for any firewall or proxy restrictions
- Ensure your internet connection is stable

## Privacy Notes

- Only public activity is fetched by default
- Private repository activity requires appropriate token permissions
- No sensitive data like passwords or private keys are ever accessed
- The datasource only reads data, never modifies anything on Codeberg