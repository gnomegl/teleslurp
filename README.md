# Teleslurp

Teleslurp is a command-line tool that allows you to search for and analyze Telegram users' activities across different groups and channels. It combines the TGScan API with Telegram's official API to provide comprehensive user information and message history.

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
teleslurp search [username] [flags]
```

Search for a Telegram user's activity across groups and channels.

Flags:
- `--api-hash string`   Telegram API Hash
- `--api-id int`        Telegram API ID
- `--api-key string`    TGScan API key
- `--csv`               Export results to CSV file
- `-h, --help`          Help for search command
- `--json`              Export results to JSON file
- `--no-prompt`         Disable interactive prompts

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
