# skill-vault

Back up your AI agent skills, config, memory, and conversation logs across all installed tools.

## How It Works

Run `skill-vault init` once. It scans for installed AI tools, lets you select what to back up, and configures backup targets. Subsequent syncs run manually or on a schedule.

```
$ skill-vault init

Scanning for installed AI tools...

  [x] Claude Code   skills, config, memory, conversations
  [x] Cursor        rules, config
  [x] Windsurf      rules, config, memory
  [ ] Aider         config, conversations
  [ ] Continue      rules, config

Backup targets:

  [x] Git (github.com/you/ai-backup)
  [x] AWS S3 (s3://my-ai-backups)
  [ ] Google Cloud Storage
  [ ] Azure Blob Storage
  [ ] iCloud Drive
  [ ] Time Machine

Schedule automatic backups? [y/N]: y
Interval (default 24h): 24h

Config written to ~/.skill-vault/config.yaml
Launchd job installed. Run `skill-vault sync` to back up now.
```

## Supported Tools

| Tool | Skills / Rules | Config | Memory | Conversations |
|------|---------------|--------|--------|---------------|
| Claude Code | `skills/`, `agents/`, `commands/` | `settings.json` | `projects/*/memory/` | `projects/*.jsonl` |
| Cursor | `rules/` | `settings.json`, `mcp.json` | | |
| Codex | | `config.yaml`, `instructions.md` | | |
| Windsurf | `rules/` | `settings.json` | `memories/` | |
| Aider | | `.aider.conf.yml` | | `chat-history/` |
| Continue | `rules/` | `config.json`, `config.ts`, `config.yaml` | | |
| Copilot | | config dir | | |
| Amp | | `config.yaml` | | `threads/` |
| Cline | `rules/` | `config.json` | | `tasks/` |
| Roo Code | `rules/` | `config.json` | | `tasks/` |
| Tabnine | | `config/` | | |
| Supermaven | | config dir | | |
| Zed AI | `rules/` | `settings.json`, `keymap.json` | | `conversations/` |
| Warp AI | | `config.yaml` | | `sessions/` |
| Amazon Q | | config dir | | |
| Gemini CLI | | `settings.json` | | `history/` |
| Claude Dev | | `config.json` | | `tasks/` |

## Backup Targets

| Target | What gets backed up | Requires |
|--------|---------------------|----------|
| Git (GitHub / GitLab) | Skills, config, memory, rules | `git` |
| AWS S3 | Conversation logs (compressed) | `aws` CLI |
| Google Cloud Storage | Conversation logs (compressed) | `gcloud` CLI |
| Azure Blob Storage | Conversation logs (compressed) | `az` CLI |
| iCloud Drive | Conversation logs (compressed) | macOS |
| Time Machine | Verifies tool dirs are included | macOS |

## Installation

```bash
go install github.com/lorenjphillips/skill-vault@latest
```

## Usage

### `skill-vault init`

Interactive setup. Detects installed tools, configures backup targets, and optionally installs a macOS launchd job for automatic syncs.

### `skill-vault sync`

Run a backup immediately using the current config.

### `skill-vault status`

Show the last sync time and state of each configured target.

### `skill-vault uninstall`

Remove the scheduled launchd job.

## Config

`skill-vault init` writes `~/.skill-vault/config.yaml`. You can edit it directly.

```yaml
tools:
  claude:
    enabled: true
    categories: [skills, config, memory, conversations]
  cursor:
    enabled: true
    categories: [rules, config]

git:
  enabled: true
  provider: github
  repo: git@github.com:you/ai-backup.git
  local_path: ~/Development/ai-backup

s3:
  enabled: true
  bucket: my-ai-backups
  profile: default
  region: us-east-1

schedule:
  enabled: true
  interval: 24h
```

## Requirements

- macOS (launchd scheduling)
- `git` and `rsync` for git-based backup
- `tar` for cloud storage backup
- Cloud CLI matching your target: `aws`, `gcloud`, or `az`

## Contributing

PRs welcome.

## License

MIT. See [LICENSE](LICENSE).
