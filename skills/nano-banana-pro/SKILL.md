---
name: nano-banana-pro
description: Generate or edit images via Gemini 3 Pro Image (Nano Banana Pro).
homepage: https://ai.google.dev/
metadata:
  {
    "goclaw":
      {
        "emoji": "🍌",
        "requires": { "bins": ["uv"], "env": ["GOOGLE_API_KEY"] },
        "primaryEnv": "GOOGLE_API_KEY",
        "install":
          [
            {
              "id": "uv-brew",
              "kind": "brew",
              "formula": "uv",
              "bins": ["uv"],
              "label": "Install uv (brew)",
            },
          ],
      },
  }
---

# Nano Banana Pro (Gemini 3 Pro Image)

Use the bundled script to generate or edit images.

## Quick Start

Generate an image:

```bash
uv run skills/nano-banana-pro/scripts/generate_image.py --prompt "your image description" --filename "output.png" --resolution 1K
```

## Commands

### Generate Image

```bash
uv run skills/nano-banana-pro/scripts/generate_image.py --prompt "your image description" --filename "output.png" --resolution 1K
```

### Edit Image

```bash
uv run skills/nano-banana-pro/scripts/generate_image.py --prompt "edit instructions" --filename "output.png" -i "/path/in.png" --resolution 2K
```

### Multi-Image Composition

Combine up to 14 images:

```bash
uv run skills/nano-banana-pro/scripts/generate_image.py --prompt "combine these into one scene" --filename "output.png" -i img1.png -i img2.png -i img3.png
```

## Parameters

- `--prompt`: Image description (required)
- `--filename`: Output filename (required)
- `--resolution`: Image resolution: `1K` (default), `2K`, `4K`
- `-i`: Input image path for editing/composition (can be used multiple times)

## API Key

Set one of the following environment variables:
- `GOOGLE_API_KEY` (preferred)
- `GEMINI_API_KEY`

## Notes

- Resolutions: `1K` (default), `2K`, `4K`.
- Use timestamps in filenames: `yyyy-mm-dd-hh-mm-ss-name.png`.
- The script prints a `MEDIA:` line for auto-attachment on supported chat providers.
- Do not read the image back; report the saved path only.
