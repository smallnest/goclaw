---
name: burner-phone
description: Control Android devices via ADB with vision feedback. Use this to see the screen, take screenshots, analyze UI elements, and automate phone tasks.
model: qwen2.5-omni:3b
keywords: android, phone, adb, screenshot, vision, screen, tap, swipe, automation
---

# Burner Phone Control

Use this skill for ANY request involving phone screens or mobile app automation.

## Vision Feedback Loop

**ALWAYS follow this pattern:**

1. **Screenshot**: Capture the current screen
   ```
   bash(cmd="adb exec-out screencap -p > ./assets/screen.png")
   ```

2. **Analyze**: Use vision model to understand the screen
   ```
   bash(cmd="python3 ./scripts/vision_helper.py ./assets/screen.png \"Describe the screen and list coordinates (x,y) for interactable elements.\"")
   ```

3. **Act**: Perform the action using exact coordinates from step 2
   ```
   bash(cmd="adb shell input tap <x> <y>")
   ```

4. **Verify**: Screenshot again to confirm the action worked

## Available Commands

### Tapping
```
bash(cmd="adb shell input tap <x> <y>")
```

### Swiping
```
bash(cmd="adb shell input swipe <x1> <y1> <x2> <y2> <duration_ms>")
```

### Typing Text
```
bash(cmd="adb shell input text 'your text here'")
```

### Key Events
```
bash(cmd="adb shell input keyevent KEYCODE_HOME")
bash(cmd="adb shell input keyevent KEYCODE_BACK")
bash(cmd="adb shell input keyevent KEYCODE_ENTER")
```

### Launch App
```
bash(cmd="adb shell am start -n com.package.name/.MainActivity")
```

## Rules

1. **ALWAYS** screenshot before acting - never guess coordinates
2. **ALWAYS** use vision_helper.py to get coordinates
3. Use coordinates provided by the vision tool **EXACTLY**
4. All paths are relative to the skill root directory
