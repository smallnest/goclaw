#!/bin/bash
# Burner Phone Skill Setup
# Installs dependencies and downloads the vision model

set -e

echo "🔥 Burner Phone Skill Setup"
echo "==========================="

# Check for ADB
if ! command -v adb &> /dev/null; then
    echo "⚠️  ADB not found. Installing..."
    if command -v apt &> /dev/null; then
        sudo apt update && sudo apt install -y android-tools-adb
    elif command -v brew &> /dev/null; then
        brew install android-platform-tools
    else
        echo "❌ Please install ADB manually"
        exit 1
    fi
fi
echo "✅ ADB found"

# Check for Ollama
if ! command -v ollama &> /dev/null; then
    echo "❌ Ollama not found. Please install from https://ollama.ai"
    exit 1
fi
echo "✅ Ollama found"

# Download Vision Model
VISION_MODEL="${VISION_MODEL:-qwen2.5-omni:3b}"
echo ""
echo "📥 Downloading Vision Model: $VISION_MODEL"
ollama pull "$VISION_MODEL"

# Create assets directory
mkdir -p ./assets

# Test ADB connection
echo ""
echo "🔍 Checking for connected devices..."
adb devices

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Setup complete!"
echo ""
echo "To use this skill:"
echo "1. Connect your Android device via USB"
echo "2. Enable USB debugging on the device"
echo "3. Run: adb devices (should show your device)"
echo ""
echo "Environment Variables:"
echo "  SENTER_URL    - API server (default: http://localhost:8081)"
echo "  VISION_MODEL  - Vision model (default: qwen2.5-omni:3b)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
