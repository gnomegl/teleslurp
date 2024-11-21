# Teleslurp

Teleslurp is a command-line tool that allows you to search for and analyze Telegram users' activities across different groups and channels. It combines the TGScan API with Telegram's official API to provide comprehensive user information and message history.

## Features

- User information lookup (ID, username, first name, last name)
- Username history tracking
- ID history tracking
- Group membership analysis
- Message history search across multiple groups
- Automatic session management
- Secure credential storage

## Prerequisites

- Go 1.16 or higher
- Telegram API credentials (API ID and API Hash)
- TGScan API key
- A Telegram account for authentication

# Installation Methods
```bash
go install github.com/gnomegl/teleslurp/cmd/teleslurp@latest
```
The binary will be installed to your $GOPATH/bin directory. Make sure this directory is in your system's PATH.

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

## Usage

```bash
teleslurp <username>
```

Example:
```bash
teleslurp ytcracka
```

### Authentication

On first run, you'll need to authenticate with Telegram. The tool will:
1. Request your phone number
2. Send a verification code to your Telegram account
3. Request 2FA password if enabled
4. Store the session for future use

## Output

The tool provides detailed information including:

1. User Information:
   - Current ID and username
   - First and last name
   - Username history
   - ID history

2. Meta Information:
   - Number of known groups
   - Total groups found
   - Operation costs

3. Group Information:
   - Group names and usernames
   - Last update dates
   - Message history for each group

4. Message Details:
   - Timestamp
   - Content
   - Direct message links
   - Channel/group context

## Rate Limiting

The tool implements reasonable delays between requests:
- 500ms between message searches
- 2 seconds between group searches

## Dependencies

- github.com/gotd/td - Telegram client implementation
- Standard Go libraries for HTTP requests and JSON handling

## Security

- Credentials are stored locally in the user's config directory
- Session data is encrypted and stored separately from configuration
- API keys and credentials are never logged or transmitted except to their respective services

