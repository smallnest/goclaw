#!/usr/bin/env python3
"""
Vision Helper for Burner Phone Skill
Sends images to Senter Server for visual analysis.
"""

import json, os, sys, base64, urllib.request

# Configuration via environment
SENTER_URL = os.getenv("SENTER_URL", "http://localhost:8081")
VISION_MODEL = os.getenv("VISION_MODEL", "qwen2.5-omni:3b")

def log(msg):
    print(f"DEBUG: {msg}", file=sys.stderr, flush=True)

def main():
    if len(sys.argv) < 2:
        print("Usage: vision_helper.py <path_to_image> [prompt]")
        sys.exit(1)

    path = sys.argv[1]
    user_prompt = sys.argv[2] if len(sys.argv) > 2 else "Analyze this screen."

    if not os.path.exists(path):
        log(f"Error: File not found: {path}")
        sys.exit(1)

    # Read image as Base64
    try:
        with open(path, "rb") as f:
            img_b64 = base64.b64encode(f.read()).decode()
    except Exception as e:
        log(f"Image Read Error: {e}")
        sys.exit(1)

    # Standard OpenAI-Format Request
    messages = [
        {
            "role": "user",
            "content": [
                {"type": "text", "text": user_prompt},
                {
                    "type": "image_url",
                    "image_url": {"url": f"data:image/png;base64,{img_b64}"}
                }
            ]
        }
    ]

    data = {
        "model": VISION_MODEL,
        "messages": messages,
        "max_tokens": 512,
        "temperature": 0.1
    }

    log("Calling vision model...")
    try:
        req = urllib.request.Request(
            f"{SENTER_URL}/v1/chat/completions",
            data=json.dumps(data).encode(), 
            headers={"Content-Type": "application/json"}
        )
        with urllib.request.urlopen(req, timeout=300) as res:
            raw_response = res.read().decode()
            try:
                resp_json = json.loads(raw_response)
                if "choices" in resp_json:
                    content = resp_json["choices"][0]["message"]["content"]
                    print(content)
                else:
                    log(f"API Error: {raw_response}")
            except json.JSONDecodeError:
                log(f"Invalid JSON: {raw_response}")
            
    except Exception as e:
        log(f"Vision Error: {e}")
        print(f"[ERROR: Vision analysis failed. Details: {e}]")

if __name__ == "__main__":
    main()
