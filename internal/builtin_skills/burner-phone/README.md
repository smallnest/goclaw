# 🔥 Burner Phone Skill

![Burner Phone](burner-phone-pfp.webp)

Control Android devices directly via ADB commands - perfect for burner phones, testing devices, or automation tasks.

## Features

- **Vision-First**: Uses AI to analyze screen content and provide exact coordinates
- **Direct Control**: ADB commands for tapping, swiping, typing
- **Openskills Compatible**: Works with any agent that supports the openskills format

## Quick Start

```bash
# Clone the skill to your skills directory
git clone https://github.com/SouthpawIN/burner-phone.git ~/.opencode/skills/burner-phone

# Run setup
cd ~/.opencode/skills/burner-phone
chmod +x scripts/setup.sh
./scripts/setup.sh
```

## Requirements

- Python 3.8+
- ADB (Android Debug Bridge)

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SENTER_URL` | `http://localhost:8081` | Senter Server URL |
| `VISION_MODEL` | `qwen2.5-omni:3b` | Vision model name |

## Usage

The skill follows a **Vision Feedback Loop**:

1. **Screenshot** → Capture current screen
2. **Analyze** → AI identifies UI elements and coordinates
3. **Act** → Execute ADB command with exact coordinates
4. **Verify** → Screenshot again to confirm

### Example Commands

```bash
# Take screenshot
adb exec-out screencap -p > ./assets/screen.png

# Analyze screen
python3 ./scripts/vision_helper.py ./assets/screen.png "Find the Settings icon"

# Tap at coordinates
adb shell input tap 540 1200

# Swipe up
adb shell input swipe 540 1800 540 800 300
```

## Directory Structure

```
burner-phone/
├── SKILL.md              # Skill manifest (openskills format)
├── README.md             # This file
├── scripts/
│   ├── vision_helper.py  # Vision analysis helper
│   └── setup.sh          # Installation script
└── assets/
    └── screen.png        # Screenshots saved here
```

## Related Projects

- [Senter](https://github.com/SouthpawIN/Senter) - Agent that uses this skill
- [Senter-Server](https://github.com/SouthpawIN/Senter-Server) - Model proxy server

## License

MIT
