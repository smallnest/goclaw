---
name: find-skills
description: Helps users discover, install, and manage goclaw skills. Use when users ask "how do I do X", "find a skill for X", "is there a skill that can...", or express interest in extending capabilities. Provides guidance for finding skills in the goclaw ecosystem and the broader open agent skills community.
version: 1.0.0
author: Ducc
metadata:
  openclaw:
    emoji: 🔍
    always: false
    requires:
      bins: [goclaw]
      os: [darwin, linux, windows]
---

# Find Skills

Helps discover and install skills for goclaw.

## Triggers

**Discovery requests:**
- "find a skill for X", "is there a skill for X"
- "how do I do X" where X might have an existing skill
- "can you do X" for specialized capabilities
- "I need help with X"

**Extension interest:**
- "I wish I had X capability"
- "Is there a tool for X?"
- "extend agent capabilities"
- "add new functionality"

**Management:**
- "list skills", "show installed skills"
- "update skills", "remove skill"
- "install skill", "add skill"

---

## Quick Start

### List Installed Skills

```bash
goclaw skills list
goclaw skills list --verbose  # Show detailed info including content
```

### Install from URL

```bash
# From Git repository
goclaw skills install https://github.com/user/skill-repo

# From local path
goclaw skills install ./path/to/skill
```

### Validate Skills

```bash
# Check if skill dependencies are satisfied
goclaw skills validate skill-name
```

### Test Skills

```bash
# Test a skill with a prompt
goclaw skills test skill-name --prompt "your test prompt here"
```

---

## Skill Locations

Skills are searched in these locations (in order):

| Location | Description |
|----------|-------------|
| `~/.goclaw/skills` | User-level skills |
| `<workspace>/.goclaw/skills` | Workspace-specific |
| `<exe-path>/skills` | Bundled with binary |
| `./skills` | Current directory (dev) |

---

## Installing Skills

### From Git Repository

```bash
# Basic installation
goclaw skills install https://github.com/user/repo

# The skill will be installed to ~/.goclaw/skills/<repo-name>
```

### From Local Path

```bash
# From a directory containing SKILL.md
goclaw skills install /path/to/skill
```

### Updating Skills

```bash
# Update a specific skill (must be a Git repo)
goclaw skills update skill-name
```

### Uninstalling Skills

```bash
# Remove a skill
goclaw skills uninstall skill-name
```

---

## Finding Relevant Skills

### 1. Search Built-in Skills

Use `goclaw skills list` to see what's available:

```bash
goclaw skills list --verbose
```

### 2. Browse External Sources

**Official goclaw skills:** Check the project repository for bundled skills.

**Community sources:**
- GitHub: Search for `goclaw skill` or `agent skill`
- Skills compatible with OpenClaw/Agent Skills format can often be adapted

### 3. Check Skill Metadata

Each skill has a `description` field in its YAML frontmatter that indicates when it should be used:

```yaml
---
name: crawl4ai
description: This skill should be used when users need to scrape websites,
extract structured data, handle JavaScript-heavy pages, or build automated
web data pipelines.
---
```

---

## Creating Custom Skills

If no existing skill meets your needs, create one:

### Initialize New Skill

```bash
cd ~/.goclaw/skills
mkdir my-skill
cd my-skill

# Create SKILL.md with proper frontmatter
```

### SKILL.md Template

```markdown
---
name: my-skill
description: Brief description of what this skill does and when to use it.
version: 1.0.0
author: Your Name
metadata:
  openclaw:
    emoji: 🎯
    always: false
    requires:
      bins: []        # Required binaries
      anyBins: []     # At least one of these must exist
      env: []         # Required environment variables
      config: []      # Required config keys
      os: [darwin, linux, windows]
---

# My Skill

## Overview

Brief description of what this skill does.

## Triggers

When this skill should activate.

## Quick Start

Basic usage examples.

## Reference

Detailed documentation.
```

---

## Best Practices

1. **Search first** - Check if a skill already exists before creating
2. **Validate before use** - Run `goclaw skills validate <name>` to check dependencies
3. **Test skills** - Use `goclaw skills test <name>` to verify behavior
4. **Keep skills lean** - Skills should be concise and focused
5. **Use references** - Move detailed docs to `references/` directory

---

## Troubleshooting

### Skill not found

```bash
# Check skill is in correct location
ls -la ~/.goclaw/skills/

# Verify skill has SKILL.md
ls ~/.goclaw/skills/<skill-name>/SKILL.md
```

### Dependencies not satisfied

```bash
# Validate to see what's missing
goclaw skills validate <skill-name>

# Install missing binaries or set environment variables
```

### Skill not loading

```bash
# Check skill format is valid
cat ~/.goclaw/skills/<skill-name>/SKILL.md

# YAML frontmatter must start with `---`
# and have a name and description field
```
