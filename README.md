# Instagram Unfollow v2

A Go-based tool to automate unfollowing Instagram accounts that don't follow you back. Uses browser automation (chromedp) to interact with Instagram's web interface and respects rate limits to avoid account restrictions.

## Features

- **Data Import**: Parse Instagram data export (ZIP file) to extract followers and following lists
- **SQLite Storage**: Persist data locally for tracking and resuming operations
- **Browser Automation**: Uses Chrome via chromedp with anti-detection measures
- **Rate Limiting**: Configurable delays and hourly limits to avoid triggering Instagram's spam detection
- **Resume Support**: Tracks unfollowed users so you can stop and resume without duplicates
- **Manual Login**: Waits for you to log in manually, preserving 2FA and avoiding automation detection

## Requirements

- Go 1.21 or later
- Google Chrome browser installed
- Instagram data export (request from Instagram Settings → Your Activity → Download Your Information)

## Installation

```bash
git clone https://github.com/worlddrknss/instagram-unfollow-v2.git
cd instagram-unfollow-v2
go mod tidy
go build -o instagram-unfollow ./cmd
```

## Usage

### Step 1: Request Your Instagram Data

1. Go to Instagram Settings → Your Activity → Download Your Information
2. Select **JSON** format (not HTML)
3. Select **Followers and Following** only
4. Set timeframe to **All time**
5. Submit request and wait for Instagram to email you the download link
6. Download the ZIP file

### Step 2: Import Your Data

```bash
./instagram-unfollow --data path/to/instagram-export.zip
```

This will:
- Extract the ZIP file
- Parse `followers_*.json` and `following.json`
- Store the data in SQLite (`~/.instagram-unfollow/instagram.db`)

### Step 3: Run the Unfollow Process

```bash
./instagram-unfollow --unfollow
```

This will:
1. Open a Chrome browser window
2. Navigate to Instagram and wait for you to log in manually (10 minute timeout)
3. Find accounts you follow that don't follow you back
4. Unfollow them one by one, respecting rate limits

### Combined Command

You can import data and unfollow in one command:

```bash
./instagram-unfollow --data path/to/instagram-export.zip --unfollow
```

## Configuration

Edit `config.yaml` to customize behavior:

```yaml
app:
  name: "instagram-unfollow"
  version: "2.0.0"
  unfollow_delay_seconds: 60  # Delay between each unfollow action

instagram:
  automation_limits:
    actions:
      hourly: 50      # Max unfollows per hour
      daily: 10000    # Max unfollows per day (effectively unlimited)
    session:
      max_duration_minutes: 120
```

### Custom Config Path

```bash
./instagram-unfollow --config /path/to/custom-config.yaml --unfollow
```

## Data Storage

All data is stored in `~/.instagram-unfollow/`:

| File | Description |
|------|-------------|
| `instagram.db` | SQLite database with followers, following, and unfollowed tracking |
| `chrome-profile/` | Persistent Chrome profile (preserves login sessions) |

## Database Schema

```sql
-- Accounts you follow
CREATE TABLE following (
    username TEXT PRIMARY KEY,
    href TEXT,
    timestamp INTEGER
);

-- Accounts that follow you
CREATE TABLE followers (
    username TEXT PRIMARY KEY,
    href TEXT,
    timestamp INTEGER
);

-- Tracks unfollowed accounts (prevents duplicates on restart)
CREATE TABLE unfollowed (
    username TEXT PRIMARY KEY,
    unfollowed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## How It Works

1. **Candidate Selection**: Finds users in `following` table who are NOT in `followers` table and NOT in `unfollowed` table
2. **Browser Automation**: Opens Chrome with a persistent profile and anti-detection flags
3. **Manual Login**: Waits up to 10 minutes for you to log in (supports 2FA, CAPTCHA, etc.)
4. **Unfollow Loop**: For each candidate:
   - Navigate to their profile
   - Click the "Following" button (with dropdown arrow)
   - Click "Unfollow" in the modal
   - Mark as unfollowed in database
   - Wait for configured delay
5. **Rate Limiting**: Respects hourly limits and delays between actions

## Safety Features

- **Persistent Chrome Profile**: Reuses the same browser profile to appear more human-like
- **Manual Login**: No credential storage; you log in yourself
- **Anti-Detection Flags**: Disables automation indicators in Chrome
- **Configurable Rate Limits**: Default 50/hour with 60-second delays
- **Resume Support**: If interrupted, resumes where you left off

## Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Path to config.yaml | `./config.yaml` |
| `--data` | Path to Instagram export ZIP file | (none) |
| `--unfollow` | Run the unfollow process | `false` |

## Troubleshooting

### Login Detection Issues

If the tool doesn't detect your login:
- Make sure you're fully logged in (not on a 2FA or verification page)
- The tool checks for the presence of cookies to detect login state

### Unfollow Button Not Working

Instagram's UI changes frequently. If clicking fails:
- Check the browser window to see what's happening
- The tool uses JavaScript to find buttons containing "Following" text

### Rate Limiting

If you get temporarily blocked:
- Increase `unfollow_delay_seconds` in config
- Decrease `hourly` limit
- Wait 24-48 hours before resuming

## License

See [LICENSE](LICENSE) for details.

## Disclaimer

This tool is for educational purposes. Use at your own risk. Automating Instagram actions may violate their Terms of Service and could result in account restrictions or bans.
