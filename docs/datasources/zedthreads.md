# Zed Threads Datasource

The Zed threads datasource extracts conversation threads from the Zed editor's AI assistant feature, allowing you to index and search your AI chat history with the editor.

## Overview

This datasource reads thread data from Zed's local SQLite database, including:
- Conversation summaries
- Complete message threads (user and assistant messages)
- AI model information
- Token usage statistics
- Thread timestamps

The datasource safely creates a temporary copy of the database to avoid conflicts with Zed when it's running.

## Configuration

### Basic Configuration (Default Path)

```toml
[datasources.zed-threads]
type = 'zedthreads'

# No additional config needed - uses default path
```

### Custom Database Path

```toml
[datasources.zed-threads]
type = 'zedthreads'

[datasources.zed-threads.config]
database_path = '/custom/path/to/threads.db'
```

### Configuration Fields

- `database_path` (optional): Path to Zed's threads database file
  - Default: `$HOME/.local/share/zed/threads/threads.db`

## Default Database Location

The datasource automatically uses the standard Zed threads database location:

### Linux
```
~/.local/share/zed/threads/threads.db
```

### macOS
```
~/Library/Application Support/Zed/threads/threads.db
```

### Windows
```
%APPDATA%\Zed\threads\threads.db
```

If your Zed installation uses a different location, you can specify the full path in the configuration.

## Multiple Profiles

You can configure multiple Zed installations or profiles as separate datasources:

```toml
[datasources.zed-main]
type = 'zedthreads'

# Uses default path

[datasources.zed-dev]
type = 'zedthreads'

[datasources.zed-dev.config]
database_path = '/home/user/.local/share/zed-dev/threads/threads.db'
```

## Usage

Once configured, the Zed threads datasource works with all standard ergs commands:

```bash
# Fetch all thread data
ergs fetch

# Search your AI conversations
ergs search --query "optimization"

# List recent conversations
ergs list --datasource zed-threads --limit 10

# Search for specific topics
ergs search --query "database migration" --datasource zed-threads
```

## Data Fields

Each thread record includes:
- **summary**: AI-generated summary of the conversation
- **updated_at**: When the thread was last updated
- **model**: AI model used (e.g., "gpt-4", "claude-3")
- **version**: Zed thread format version
- **message_count**: Total number of messages in the thread
- **user_messages**: Number of user messages
- **assistant_messages**: Number of AI responses
- **token_total**: Total tokens used (if available)
- **token_input**: Input tokens used
- **token_output**: Output tokens generated

## Data Content

The datasource indexes the full content of conversations, making it searchable:
- Thread summaries
- All user questions and prompts
- Complete AI assistant responses
- Code snippets and explanations
- Context from conversations

## Important Notes

- **Database Locks**: Close Zed before running ergs to avoid database locking issues
- **Privacy**: Only conversations stored in Zed's local database are included
- **Safety**: The datasource creates a temporary copy of your database file, so your original Zed data is never modified
- **Compression**: Zed uses zstd compression for message data, which is automatically handled
- **Performance**: Large conversation histories may take longer to process initially

## Troubleshooting

### "Database file does not exist"
- Verify that Zed has been used for AI conversations (threads database is created on first use)
- Check that Zed is installed and has created the threads directory
- Ensure you have read permissions for the Zed data directory

### "Required Zed threads schema not found"
- The database file may be from a different application
- Verify the file is actually Zed's threads database
- Try updating Zed to the latest version

### "Failed to decompress thread data"
- The database may be corrupted or from an incompatible Zed version
- Try using a backup of the threads database if available
- Check Zed's logs for any database corruption issues

### Empty results
- Verify that you have AI conversation history in Zed
- Check that Zed is completely closed when running ergs
- Try running ergs with `--debug` flag for more detailed logging

### Permission errors
- Ensure you have read access to the Zed data directory
- On some systems, you may need to adjust file permissions
- Try running ergs as the same user that runs Zed

## Use Cases

The Zed threads datasource is particularly useful for:
- **Code Learning**: Search through AI explanations of coding concepts
- **Problem Solving**: Find previous conversations about similar issues
- **Knowledge Management**: Build a searchable archive of programming discussions
- **Research**: Track AI responses on specific topics over time
- **Documentation**: Reference past AI-generated code examples and explanations