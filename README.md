# Teleslurp

Teleslurp is a command-line tool that allows you to search for and analyze Telegram users' activities across different groups and channels. It has built-in support for the TGScan API with Telegram's official API to provide comprehensive user information and message history.

## Prerequisites

- Go 1.16 or higher
- Telegram API credentials (API ID and API Hash)
- TGScan API key
- A Telegram account for authentication

## Installation
```bash
go install github.com/gnomegl/teleslurp
```

## Commands

### Search Command
```bash
teleslurp search [username|user_id] [flags]
```

Search for a Telegram user's activity across groups and channels. You can search using either:
- A username (e.g., `teleslurp search johndoe`)
- A numeric user ID (e.g., `teleslurp search 5338795474`)

The tool will:
1. Find the user's information and group memberships
2. Crawl all accessible groups for messages from that user
3. Export the results based on the specified format (JSON or CSV)

#### Output Format
When using CSV or JSON export, each message will include:
- Channel Information:
  - Title and username
  - List of channel administrators (if accessible)
  - Total member count
  - When the target user joined the channel (if accessible)
- Message Details:
  - Message ID and content
  - Date and time
  - Direct link to message

Note: Some channel information may be unavailable depending on your access level and the channel's privacy settings.

#### Flags
- `--api-hash string`   Telegram API Hash (optional if already set in config)
- `--api-id int`        Telegram API ID (optional if already set in config)
- `--api-key string`    TGScan API key (optional if already set in config)
- `--input-file string` Input file containing Telegram channels/groups to search (CSV or text file)
- `--csv`               Export results and channel metadata to CSV files
- `-h, --help`          Help for search command
- `--json`              Export results and channel metadata to JSON files
- `--no-prompt`         Disable interactive prompts

Note: When using `--csv` or `--json`, two files will be created:
- `username_messages.[csv|json]` - Contains all messages found
- `username_channel_metadata.[csv|json]` - Contains detailed information about each channel

### Completion Command
```bash
teleslurp completion [shell]
```

Generate shell completion scripts for bash, zsh, fish, or powershell.

### Help Command
```bash
teleslurp help [command]
```

Get help about any command.

## Output

### Summary Statistics
After the search completes, the tool displays a comprehensive summary:

For each channel with messages:
- Channel name and username/link
- Number of messages found
- Member count
- Date of user's first message
- Admin status

Overall statistics:
- Total number of channels with messages
- Total messages found
- Total members in channels
- Average messages per channel

### Export Formats
When using CSV or JSON export, each message will include:
- Channel Information:
  - Title and username
  - List of channel administrators (if accessible)
  - Total member count
  - When the target user joined the channel (if accessible)
- Message Details:
  - Message ID and content
  - Date and time
  - Direct link to message

## Configuration

The tool stores its configuration in the following locations:
- Windows: `%LOCALAPPDATA%\teleslurp\config.json`
- Linux/macOS: `~/.config/teleslurp/config.json`

On first run, you'll be prompted to enter:
1. TGScan API key
2. Telegram API ID
3. Telegram API Hash
4. Phone number (during authentication)
5. Your 2FA password (if applicable)

## Input File

The `--input-file` flag allows you to specify a file containing Telegram channels or groups to search. The tool supports various input formats:

### Telegram Links
The tool will automatically detect and parse t.me links in any of these formats:
- `https://t.me/channelname`
- `https://t.me/c/1234567890`
- `https://t.me/s/channelname`
- `t.me/channelname`
- `t.me/c/1234567890`
- `t.me/s/channelname`

### Channel IDs and Usernames
If no t.me links are found, each line in the file will be treated as either:
- A numeric channel ID (e.g., `1234567890`)
- A channel username (e.g., `channelname`)

### Example Input File
```text
# Comments are supported
https://t.me/channel1
t.me/c/1234567890
t.me/s/channel2
channel3
1234567890
```

Note: The tool will attempt to resolve each entry to a valid Telegram channel or group. Invalid or inaccessible entries will be skipped with a warning message.

## Technical Details

### Rate Limiting

The tool implements reasonable delays between requests:
- 500ms between message searches
- 2 seconds between group searches

### Dependencies

- github.com/spf13/cobra - CLI framework
- github.com/gotd/td - Telegram client implementation
- Standard Go libraries for HTTP requests and JSON handling

### Security

- Credentials are stored locally in the user's config directory
- Session data is encrypted and stored separately from configuration
- API keys and credentials are never logged or transmitted except to their respective services
