---
name: coding-agent
description: Run Codex CLI, Claude Code, Kiro CLI, OpenCode, or Pi Coding Agent via background process for programmatic control.
metadata:
  {
    "openclaw": { "emoji": "🧩", "requires": { "anyBins": ["claude", "codex", "opencode", "pi", "kiro-cli"] } },
  }
---

# Coding Agent (bash-first)

Use **bash** (with optional background mode) for all coding agent work. Simple and effective.

## ⚠️ PTY Mode Required!

Coding agents (Codex, Claude Code, Kiro, Pi) are **interactive terminal applications** that need a pseudo-terminal (PTY) to work correctly. Without PTY, you'll get broken output, missing colors, or the agent may hang.

**Always use `pty:true`** when running coding agents:

```bash
# ✅ Correct - with PTY
bash pty:true command:"codex exec 'Your prompt'"

# ❌ Wrong - no PTY, agent may break
bash command:"codex exec 'Your prompt'"
```

### Bash Tool Parameters

| Parameter    | Type    | Description                                                                 |
| ------------ | ------- | --------------------------------------------------------------------------- |
| `command`    | string  | The shell command to run                                                    |
| `pty`        | boolean | **Use for coding agents!** Allocates a pseudo-terminal for interactive CLIs |
| `workdir`    | string  | Working directory (agent sees only this folder's context)                   |
| `background` | boolean | Run in background, returns sessionId for monitoring                         |
| `timeout`    | number  | Timeout in seconds (kills process on expiry)                                |
| `elevated`   | boolean | Run on host instead of sandbox (if allowed)                                 |

### Process Tool Actions (for background sessions)

| Action      | Description                                          |
| ----------- | ---------------------------------------------------- |
| `list`      | List all running/recent sessions                     |
| `poll`      | Check if session is still running                    |
| `log`       | Get session output (with optional offset/limit)      |
| `write`     | Send raw data to stdin                               |
| `submit`    | Send data + newline (like typing and pressing Enter) |
| `send-keys` | Send key tokens or hex bytes                         |
| `paste`     | Paste text (with optional bracketed mode)            |
| `kill`      | Terminate the session                                |

---

## Quick Start: One-Shot Tasks

For quick prompts/chats, create a temp git repo and run:

```bash
# Quick chat (Codex needs a git repo!)
SCRATCH=$(mktemp -d) && cd $SCRATCH && git init && codex exec "Your prompt here"

# Or in a real project - with PTY!
bash pty:true workdir:~/Projects/myproject command:"codex exec 'Add error handling to the API calls'"
```

**Why git init?** Codex refuses to run outside a trusted git directory. Creating a temp repo solves this for scratch work.

---

## The Pattern: workdir + background + pty

For longer tasks, use background mode with PTY:

```bash
# Start agent in target directory (with PTY!)
bash pty:true workdir:~/project background:true command:"codex exec --full-auto 'Build a snake game'"
# Returns sessionId for tracking

# Monitor progress
process action:log sessionId:XXX

# Check if done
process action:poll sessionId:XXX

# Send input (if agent asks a question)
process action:write sessionId:XXX data:"y"

# Submit with Enter (like typing "yes" and pressing Enter)
process action:submit sessionId:XXX data:"yes"

# Kill if needed
process action:kill sessionId:XXX
```

**Why workdir matters:** Agent wakes up in a focused directory, doesn't wander off reading unrelated files (like your soul.md 😅).

---

## Codex CLI

**Model:** `gpt-5.2-codex` is the default (set in ~/.codex/config.toml)

### Flags

| Flag            | Effect                                             |
| --------------- | -------------------------------------------------- |
| `exec "prompt"` | One-shot execution, exits when done                |
| `--full-auto`   | Sandboxed but auto-approves in workspace           |
| `--yolo`        | NO sandbox, NO approvals (fastest, most dangerous) |

### Building/Creating

```bash
# Quick one-shot (auto-approves) - remember PTY!
bash pty:true workdir:~/project command:"codex exec --full-auto 'Build a dark mode toggle'"

# Background for longer work
bash pty:true workdir:~/project background:true command:"codex --yolo 'Refactor the auth module'"
```

### Reviewing PRs

**⚠️ CRITICAL: Never review PRs in OpenClaw's own project folder!**
Clone to temp folder or use git worktree.

```bash
# Clone to temp for safe review
REVIEW_DIR=$(mktemp -d)
git clone https://github.com/user/repo.git $REVIEW_DIR
cd $REVIEW_DIR && gh pr checkout 130
bash pty:true workdir:$REVIEW_DIR command:"codex review --base origin/main"
# Clean up after: trash $REVIEW_DIR

# Or use git worktree (keeps main intact)
git worktree add /tmp/pr-130-review pr-130-branch
bash pty:true workdir:/tmp/pr-130-review command:"codex review --base main"
```

### Batch PR Reviews (parallel army!)

```bash
# Fetch all PR refs first
git fetch origin '+refs/pull/*/head:refs/remotes/origin/pr/*'

# Deploy the army - one Codex per PR (all with PTY!)
bash pty:true workdir:~/project background:true command:"codex exec 'Review PR #86. git diff origin/main...origin/pr/86'"
bash pty:true workdir:~/project background:true command:"codex exec 'Review PR #87. git diff origin/main...origin/pr/87'"

# Monitor all
process action:list

# Post results to GitHub
gh pr comment <PR#> --body "<review content>"
```

---

## Claude Code

```bash
# With PTY for proper terminal output
bash pty:true workdir:~/project command:"claude 'Your task'"

# Background
bash pty:true workdir:~/project background:true command:"claude 'Your task'"
```

---

## Kiro CLI (AWS)

AWS AI coding assistant with session persistence, custom agents, steering, and MCP integration.

**Install:** https://kiro.dev/docs/cli/installation

### Basic Usage

```bash
kiro-cli                           # Start interactive chat (default)
kiro-cli chat "Your question"      # Direct question
kiro-cli --agent my-agent          # Use specific agent
kiro-cli chat --resume             # Resume last session (per-directory)
kiro-cli chat --resume-picker      # Pick from saved sessions
kiro-cli chat --list-sessions      # List all sessions
```

### Non-Interactive Mode (scripting/automation)

```bash
# Single response to STDOUT, then exit
kiro-cli chat --no-interactive "Show current directory"

# Trust all tools (no confirmation prompts)
kiro-cli chat --no-interactive --trust-all-tools "Create hello.py"

# Trust specific tools only (comma-separated)
kiro-cli chat --no-interactive --trust-tools "fs_read,fs_write" "Read package.json"
```

**🔐 Tool Trust:** Use `--trust-all-tools` for automation (default). For untrusted input or sensitive systems, consider `--trust-tools "fs_read,fs_write,shell"` to limit scope.

### OpenClaw Integration

```bash
# Interactive session (background)
bash pty:true workdir:~/project background:true command:"kiro-cli"

# One-shot query (non-interactive)
bash pty:true workdir:~/project command:"kiro-cli chat --no-interactive --trust-all-tools 'List all TODO comments in src/'"

# With specific agent
bash pty:true workdir:~/project background:true command:"kiro-cli --agent aws-expert 'Set up Lambda'"

# Resume previous session
bash pty:true workdir:~/project command:"kiro-cli chat --resume"
```

### Custom Agents

Pre-define tool permissions, context resources, and behaviors:

```bash
kiro-cli agent list              # List available agents
kiro-cli agent create my-agent   # Create new agent
kiro-cli agent edit my-agent     # Edit agent config
kiro-cli agent validate ./a.json # Validate config file
kiro-cli agent set-default my-agent  # Set default
```

**Benefits:** Pre-approve trusted tools, limit tool access, auto-load project docs, share configs across team.

### Steering (Project Context)

Provide persistent project knowledge via markdown files in `.kiro/steering/`:

```
.kiro/steering/
├── product.md       # Product overview
├── tech.md          # Tech stack
├── structure.md     # Project structure
└── api-standards.md # API conventions
```

- **Workspace steering:** `.kiro/steering/` — applies to current project only
- **Global steering:** `~/.kiro/steering/` — applies to all projects
- **AGENTS.md support:** Place in project root or `~/.kiro/steering/`

**In custom agents:** Add `"resources": ["file://.kiro/steering/**/*.md"]` to config.

### MCP Integration

Connect external tools and data sources via Model Context Protocol:

```bash
kiro-cli mcp add --name my-server --command "node server.js" --scope workspace
kiro-cli mcp list [workspace|global]
kiro-cli mcp status --name my-server
kiro-cli mcp remove --name my-server --scope workspace
```

### Plan Agent

Plan Agent is a built-in agent for structured planning before execution. It helps transform ideas into detailed implementation plans.

**When to suggest Plan Agent to user:**
- Complex multi-step features (e.g., "build a user authentication system")
- Requirements are unclear or need clarification
- Large features that benefit from task breakdown

**When NOT to use Plan Agent:**
- Simple queries or single-step tasks
- User already has clear, specific instructions
- Quick fixes or small changes

**How to use:**

```bash
# Start Plan mode
> /plan
# Or with immediate prompt
> /plan Build a REST API for user authentication
# Or toggle with keyboard shortcut
Shift + Tab
```

**Plan workflow (4 phases):**

1. **Requirements gathering** — Structured questions with multiple choice options
2. **Research and analysis** — Explores codebase, identifies patterns
3. **Implementation plan** — Detailed task breakdown with clear objectives
4. **Handoff to execution** — After approval (`y`), plan transfers to execution agent

**Plan Agent limitations (read-only):**
- ✓ Can read files, search code, research documentation
- ✗ Cannot write files or execute commands (until handoff)

---

## OpenCode

```bash
bash pty:true workdir:~/project command:"opencode run 'Your task'"
```

---

## Pi Coding Agent

```bash
# Install: npm install -g @mariozechner/pi-coding-agent
bash pty:true workdir:~/project command:"pi 'Your task'"

# Non-interactive mode (PTY still recommended)
bash pty:true command:"pi -p 'Summarize src/'"

# Different provider/model
bash pty:true command:"pi --provider openai --model gpt-4o-mini -p 'Your task'"
```

**Note:** Pi now has Anthropic prompt caching enabled (PR #584, merged Jan 2026)!

---

## Parallel Issue Fixing with git worktrees

For fixing multiple issues in parallel, use git worktrees:

```bash
# 1. Create worktrees for each issue
git worktree add -b fix/issue-78 /tmp/issue-78 main
git worktree add -b fix/issue-99 /tmp/issue-99 main

# 2. Launch agent in each (background + PTY!)
bash pty:true workdir:/tmp/issue-78 background:true command:"pnpm install && codex --yolo 'Fix issue #78: <description>. Commit and push.'"
bash pty:true workdir:/tmp/issue-99 background:true command:"pnpm install && codex --yolo 'Fix issue #99: <description>. Commit and push.'"

# 3. Monitor progress
process action:list
process action:log sessionId:XXX

# 4. Create PRs after fixes
cd /tmp/issue-78 && git push -u origin fix/issue-78
gh pr create --repo user/repo --head fix/issue-78 --title "fix: ..." --body "..."

# 5. Cleanup
git worktree remove /tmp/issue-78
git worktree remove /tmp/issue-99
```

---

## ⚠️ Rules

1. **Always use pty:true** - coding agents need a terminal!
2. **Respect tool choice** - if user asks for Kiro, use Kiro; if they ask for Codex, use Codex.
   - Orchestrator mode: do NOT hand-code patches yourself.
   - If an agent fails/hangs, respawn it or ask the user for direction, but don't silently take over.
3. **Be patient** - don't kill sessions because they're "slow"
4. **Monitor with process:log** - check progress without interfering
5. **--full-auto/--yolo for Codex building** - auto-approves changes
6. **--trust-all-tools for Kiro automation** - skips confirmation prompts; use `--trust-tools` for untrusted input
7. **--no-interactive for Kiro one-shots** - single response mode
8. **Parallel is OK** - run many agent processes at once for batch work
9. **NEVER start agents in ~/clawd/** - it'll read your soul docs and get weird ideas about the org chart!
10. **NEVER checkout branches in ~/Projects/openclaw/** - that's the LIVE OpenClaw instance!
11. **Suggest Kiro /plan for complex tasks** - when requirements are unclear or task is multi-step, suggest Plan Agent to user and let them decide

---

## Progress Updates (Critical)

When you spawn coding agents in the background, keep the user in the loop.

- Send 1 short message when you start (what's running + where).
- Then only update again when something changes:
  - a milestone completes (build finished, tests passed)
  - the agent asks a question / needs input
  - you hit an error or need user action
  - the agent finishes (include what changed + where)
- If you kill a session, immediately say you killed it and why.

This prevents the user from seeing only "Agent failed before reply" and having no idea what happened.

---

## Auto-Notify on Completion

For long-running background tasks, append a wake trigger to your prompt so OpenClaw gets notified immediately when the agent finishes (instead of waiting for the next heartbeat):

```
... your task here.

When completely finished, run this command to notify me:
openclaw gateway wake --text "Done: [brief summary of what was built]" --mode now
```

**Example (Codex):**

```bash
bash pty:true workdir:~/project background:true command:"codex --yolo exec 'Build a REST API for todos.

When completely finished, run: openclaw gateway wake --text \"Done: Built todos REST API with CRUD endpoints\" --mode now'"
```

This triggers an immediate wake event — gets pinged in seconds, not 10 minutes.

---

## Learnings (Jan 2026)

- **PTY is essential:** Coding agents are interactive terminal apps. Without `pty:true`, output breaks or agent hangs.
- **Git repo required:** Codex won't run outside a git directory. Use `mktemp -d && git init` for scratch work.
- **exec is your friend:** `codex exec "prompt"` runs and exits cleanly - perfect for one-shots.
- **submit vs write:** Use `submit` to send input + Enter, `write` for raw data without newline.
- **Sass works:** Codex responds well to playful prompts. Asked it to write a haiku about being second fiddle to a space lobster, got: _"Second chair, I code / Space lobster sets the tempo / Keys glow, I follow"_ 🦞
