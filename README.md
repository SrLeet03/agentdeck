# agentdeck (`deck`)

[![ci](https://github.com/SrLeet03/agentdeck/actions/workflows/ci.yml/badge.svg)](https://github.com/SrLeet03/agentdeck/actions/workflows/ci.yml)

A terminal dashboard for all your local coding-agent sessions — **Claude Code**, **Codex** and **Pi** — in one place.
Browse and search your history, see what's running right now, resume anything in a new terminal tab, organise sessions
into pins and groups, and hand context from one session (or several) to another — across agents.

```
 ◆ agentdeck   good evening, Alex                          ⠹ 2 working │ ◐ 1 waiting │ ▂▅▃█▆▄▇ 41.2M tokens today
╭─ NOW RUNNING · 3 ───────────────────────────────────────────────────────────────────────────────────────╮
│ ⠹ CLAUDE fix flaky checkout tests      ⠹ CODEX migrate auth to OAuth      ◐ PI draft release notes     │
│   web-app · ⠹ working                    api-server · ⠹ working             docs · ◐ waiting 12m        │
╰─────────────────────────────────────────────────────────────────────────────────────────────────────────╯
  All 212  ★ Pinned 2  ● Live 3  Claude 60  Codex 98  Pi 54  Archived 0  ▸launch 4      / search   ⌃K commands
╭──────────────────────────────────────────────────────╮╭───────────────────────────────────────────────╮
│ ▾ ★ PINNED  2 ────────────────────────────────────── ┃││  CLAUDE CODE  cli                 ⠹ WORKING   │
│▌⠹ ★ fix flaky checkout tests                     now ┃││                                               │
│▌    CLAUDE  web-app  ⎇ main          ▁▂▅█▇▃ 12.4M   ┃││ fix flaky checkout tests                      │
│ ⠹ ★ migrate auth to OAuth                        18m │││ ⌂ ~/code/web-app   ⎇ main                     │
│     CODEX   api-server  ⎇ feat/oauth  ▁▁▃▅▂  3.1M   │││ Σ 12.4M tokens ██████████░░ in out cache      │
│ ▾ RECENT  210 ────────────────────────────────────── │││ ──────────── conversation ────────────        │
│   ◐ draft release notes                           2d │││ ▎ you · 14:02                                 │
│     PI      docs  ▸launch                       880K │││ ▎ the checkout test fails on CI only          │
╰──────────────────────────────────────────────────────╯╰───────────────────────────────────────────────╯
 enter open   n new   ⌃K commands   / search   p pin   + group   c share   T theme   ? help
```

## Requirements

- **macOS** or **Linux**
- Any of: [Claude Code](https://docs.anthropic.com/en/docs/claude-code), [Codex CLI](https://github.com/openai/codex), [Pi](https://github.com/earendil-works/pi)
- `claude` on your PATH for the context-sharing summaries

## Install

**Script** (macOS/Linux, verifies the checksum, installs to `~/.local/bin`):

```sh
curl -fsSL https://raw.githubusercontent.com/SrLeet03/agentdeck/main/install.sh | sh
```

**Homebrew** (macOS/Linux):

```sh
brew tap SrLeet03/agentdeck https://github.com/SrLeet03/agentdeck
brew install SrLeet03/agentdeck/agentdeck
```

**Debian/Ubuntu or Fedora** — download the `.deb` / `.rpm` from [Releases](https://github.com/SrLeet03/agentdeck/releases), then
`sudo apt install ./agentdeck_*.deb` or `sudo dnf install ./agentdeck-*.rpm`.

**Go** 1.26+: `go install github.com/SrLeet03/agentdeck/cmd/deck@latest` · from a checkout: `make install`.

```sh
deck doctor         # checks data folders + which terminal deck will use
deck                # open the dashboard
```

AUR and Nix packages are planned.

## Terminals

deck opens sessions in the terminal it is running in, detected automatically:

| Terminal | New tab/window | Jump to running session | Type into running session |
|---|---|---|---|
| **iTerm2** (macOS) | new tab | ✓ | ✓ |
| **Terminal.app** (macOS) | new window | ✓ | clipboard + focus |
| **tmux** (macOS/Linux, also over SSH) | new window | ✓ | ✓ |
| **GNOME Terminal** (Linux) | new tab | — | clipboard |
| **Konsole** (Linux) | new tab | — | clipboard |

Running inside tmux always uses tmux. Otherwise deck uses iTerm2/Terminal.app on macOS, or GNOME Terminal / Konsole on
Linux. Outside tmux with no GUI terminal, it falls back to a detached tmux session named `deck`. Override with
`"terminal"` in the config below.

Other commands: `deck list [n]`, `deck live`, `deck handoff <id-prefix>...`.

## What it does

- **Browse & search history** across `~/.claude/projects`, `~/.codex/sessions` and `~/.pi/agent/sessions`:
  title, first/last prompt, folder, branch, model, message counts, token usage and a live conversation preview.
- **Resume / start** — `enter` resumes the selected session in a **new tab** of your terminal
  (`claude --resume`, `codex resume`, `pi --session`); `n` starts a fresh session in any folder.
- **Live sessions** — detects every running agent, however it was started:
  - Claude Code: reads `~/.claude/sessions/<pid>.json` (exact `working / idle / shell` status, incl. background jobs).
  - Codex / Pi: scans processes, maps them to their session file via `lsof` (falls back to the most recent
    session in the same folder); “working” means the log was written in the last ~12 s.
  - `enter` on a live session jumps to its tab (or runs `claude attach <job>` for background Claude jobs);
    `x` stops it (`claude stop` for bg jobs, `SIGTERM` otherwise) after confirmation.
- **Pin** (`p`) any session. Pinned sessions sit in a **★ Pinned** section at the top of every list and in the Pinned tab.
- **Groups** — `+` adds the selected session (or every `m`-marked one) to a group: type an existing group name or a new
  one. Each group gets its own tab, mixing agents freely; a session can be in several groups.
  On a group's tab: `-` removes a session, `E` renames the group, `X` deletes the group (sessions stay).
  Search `@name` filters by group.
- **Command palette** (`⌃K`) — fuzzy-search every action, tab, theme and session from one box.
- **Fuzzy search** (`/`) — typo-tolerant and ranked, with matched letters highlighted; `#tag` and `@group` filters.
- **Themes** (`T`) — Midnight (default), Catppuccin, Mono and Retro; your choice is remembered.
- **Organise** — rename (`r`), tag (`t`, filter with `#tag` in search), archive (`a`).
  All of this lives in `~/.agentdeck/state.json`; **agent session files are never modified or deleted**.
- **Share context** (`m` to mark one or more sessions, then `c`) — deck has `claude -p` write a handoff note
  (goal, current state, decisions, files/commands/IDs, next steps) from the marked transcripts, saved in
  `~/.agentdeck/handoffs/`, then:
  - `n` starts a **new session with any agent** in a folder you pick, with the note as its first message;
  - `s` lets you pick a **running session** and types a "read this handoff" line into its tab (you press enter; terminals that can't type get it on the clipboard);
  - `y` copies the note to the clipboard.

  Each note also links the full transcript files, so the new agent can open the raw history when it needs
  exact details. Notes are reused while the source sessions haven't changed.
- **Stats** (`s`) — tokens per agent, 14-day token chart, top projects over 30 days.
- **Group by project** (`g`) — worktrees under `.claude/worktrees/` fold into their repo.
- **Mouse** — wheel scrolls the list or the preview; click selects (hold ⌥ to select text).

Press `?` inside the dashboard for all keys.

## Config

`~/.agentdeck/config.json` (created on first run):

- `"theme"`: `midnight` (default), `catppuccin`, `mono` or `retro` — also set by pressing `T`.
- `"terminal"`: `auto` (default), `iterm2`, `terminal`, `tmux`, `gnome-terminal`, `konsole`, or `none`
  (copy commands to the clipboard instead of opening anything).
- `"env"`: environment variables per agent (`claude`, `codex`, `pi`) for everything deck launches. Commands typed into
  terminal tabs get `export K=V && …`, and commands deck runs itself (`claude -p`, `claude stop`) get them in their
  environment. For example, if your shell profile enables a different Claude provider that you don't want for
  interactive sessions:

```json
{ "terminal": "auto", "env": { "claude": { "CLAUDE_CODE_USE_FOUNDRY": "0" } } }
```

## Notes

- First launch parses everything (a few seconds for very large Codex logs); results are cached in
  `~/.agentdeck/cache.json` keyed by file size + mtime, so later launches take well under a second.
- The preview reads only the last 8 MB of huge logs.
- Codex subagent threads are hidden by default (`h` to show). Claude subagent token usage is folded into its parent session.
- On macOS, the first time you open a session macOS asks to let your terminal control iTerm2/Terminal — allow it.

## License

MIT
