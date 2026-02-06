---
name: patchright-skill
description: Patchright-based browser automation with bot detection bypass. Use when Claude needs to interact with local web applications, test localhost/dev servers, take screenshots, or perform UI interactions on private networks. Ideal for QA automation, frontend debugging, E2E testing, and pre-deployment verification on local development environments.
version: 1.0.0
author: Claude
always: true
metadata:
  openclaw:
    emoji: 🌐
    always: true
    requires:
      bins: [python3]
      os: [darwin, linux, windows]
---

# Patchright - Browser Automation Skill

Patchright-based browser automation with bot detection bypass. Use for localhost, dev servers, web app testing, screenshots, and UI interactions.

## Triggers

**URL/Network:**
- localhost (http://localhost:*, http://127.0.0.1:*)
- Local IPs (192.168.x.x, 10.x.x.x, 172.16-31.x.x)
- Dev server ports (3000, 5173, 8080, 4200, 5000, etc.)

**Web App Testing:**
- "test the app", "check the site"
- "open localhost", "view in browser"
- "take screenshot", "capture screen"
- "check UI", "view page"
- QA, E2E testing, dev build verification

**Browser Interaction:**
- "click", "press button"
- "type", "fill form"
- "login", "sign up"
- "open menu", "click tab"
- "scroll", "navigate"

**Visual Verification:**
- "how does it look?", "is it working?"
- "check design", "verify layout"
- "responsive test", "screen size"
- "check rendering", "verify component"

**Development Workflow:**
- Verify changes after code edits
- Frontend debugging
- Real-time dev feedback
- Pre-deployment checks

## Core: Server Mode (Session Persistence!)

**Problem**: scripts/executor.py terminates process on each call -> browser session lost
**Solution**: scripts/server.py runs background server -> session persists

### Start Server (Required!)

```bash
cd ~/.claude/skills/patchright-skill
python scripts/server.py start &
```

### Server Commands

```bash
# Check status
python scripts/server.py status

# Stop server
python scripts/server.py stop

# Call tool
python scripts/server.py call '{"tool": "...", "args": {...}}'
```

## Usage

### 1. Navigate + Screenshot (Most Common Pattern)

```bash
cd ~/.claude/skills/patchright-skill

# Start server (if not running)
python scripts/server.py start &
sleep 2

# Navigate to page
python scripts/server.py call '{"tool": "navigate", "args": {"url": "http://localhost:3000"}}'

# Take screenshot
python scripts/server.py call '{"tool": "screenshot", "args": {"path": "screenshot.png", "full_page": true}}'
```

### 2. Click + Interaction

```bash
# Click element
python scripts/server.py call '{"tool": "click", "args": {"selector": "button.submit"}}'
python scripts/server.py call '{"tool": "click", "args": {"selector": "#menu-btn"}}'
python scripts/server.py call '{"tool": "click", "args": {"selector": "body"}}'  # Click anywhere

# Type text
python scripts/server.py call '{"tool": "type", "args": {"selector": "#email", "text": "test@test.com"}}'
python scripts/server.py call '{"tool": "type", "args": {"selector": "input[name=password]", "text": "password123"}}'
```

### 3. Get Information

```bash
# Current URL
python scripts/server.py call '{"tool": "get_url"}'

# Page title
python scripts/server.py call '{"tool": "get_title"}'

# Element text
python scripts/server.py call '{"tool": "get_text", "args": {"selector": ".error-message"}}'
```

### 4. Wait

```bash
# Wait for element to appear
python scripts/server.py call '{"tool": "wait_for", "args": {"selector": ".loading-complete", "timeout": 10000}}'
```

## Tool Reference

| Tool | Description | Args |
|------|-------------|------|
| launch | Start browser | headless: bool (default: false) |
| close | Close browser | - |
| navigate | Go to URL | url: string (required) |
| screenshot | Save to file | path: string, full_page: bool |
| click | Click element | selector: string (required) |
| type | Type text | selector: string, text: string |
| get_text | Get element text | selector: string |
| wait_for | Wait for element | selector: string, timeout: int |
| get_url | Get current URL | - |
| get_title | Get page title | - |

## Examples

### Login Test

```bash
cd ~/.claude/skills/patchright-skill
python scripts/server.py start &
sleep 2

# Navigate to login page
python scripts/server.py call '{"tool": "navigate", "args": {"url": "http://localhost:3000/login"}}'
python scripts/server.py call '{"tool": "screenshot", "args": {"path": "login_page.png"}}'

# Fill form
python scripts/server.py call '{"tool": "type", "args": {"selector": "#email", "text": "admin@test.com"}}'
python scripts/server.py call '{"tool": "type", "args": {"selector": "#password", "text": "admin123"}}'
python scripts/server.py call '{"tool": "screenshot", "args": {"path": "login_filled.png"}}'

# Submit
python scripts/server.py call '{"tool": "click", "args": {"selector": "button[type=submit]"}}'
sleep 2
python scripts/server.py call '{"tool": "screenshot", "args": {"path": "login_result.png"}}'
```

### App Navigation

```bash
# Enter app
python scripts/server.py call '{"tool": "navigate", "args": {"url": "http://localhost:3000"}}'
python scripts/server.py call '{"tool": "click", "args": {"selector": "body"}}'  # Click to enter
sleep 2
python scripts/server.py call '{"tool": "screenshot", "args": {"path": "app_main.png", "full_page": true}}'

# Explore features
python scripts/server.py call '{"tool": "click", "args": {"selector": ".create-btn"}}'
python scripts/server.py call '{"tool": "screenshot", "args": {"path": "after_action.png"}}'
```

## Selector Tips

```css
/* ID */
#submit-btn

/* Class */
.nav-menu
button.primary

/* Attribute */
input[type=email]
button[data-testid="login"]
a[href="/about"]

/* Text content */
text=Login
text=Submit

/* Combined */
form#login button[type=submit]
.sidebar .menu-item:first-child
```

## Technical Specs

- **Engine**: patchright (undetected playwright fork)
- **Browser**: Google Chrome (channel: 'chrome')
- **Bot Detection Bypass**: YES (Cloudflare, reCAPTCHA, etc.)
- **Localhost Support**: YES
- **Private IP Support**: YES
- **Server Port**: 9222

## Troubleshooting

**"Server not running" error:**
```bash
python scripts/server.py start &
sleep 2
```

**Browser not visible:**
- headless=False is default, browser window should appear
- In server mode, browser persists in background

**Session disconnected:**
- Use scripts/server.py instead of scripts/executor.py
- Server keeps session alive once started

**Element not found:**
- Use wait_for to wait first
- Verify selector in DevTools
