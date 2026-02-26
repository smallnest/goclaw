#!/usr/bin/env python3
"""
Feishu/Lark Image Upload Script

Uploads images to Feishu/Lark and returns the image_key.
"""

import argparse
import base64
import io
import json
import os
import sys
from pathlib import Path
from typing import Optional

try:
    import requests
except ImportError:
    print("Error: requests module is required. Install with: pip3 install requests")
    sys.exit(1)


class FeishuImageUploader:
    """Feishu/Lark Image Uploader"""

    API_BASE_URL = "https://open.feishu.cn"
    UPLOAD_ENDPOINT = "/open-apis/im/v1/images"

    def __init__(self, app_id: str, app_secret: str):
        self.app_id = app_id
        self.app_secret = app_secret
        self._access_token: Optional[str] = None

    def get_access_token(self) -> str:
        """Get app_access_token"""
        if self._access_token:
            return self._access_token

        url = f"{self.API_BASE_URL}/open-apis/auth/v3/app_access_token/internal"
        payload = {
            "app_id": self.app_id,
            "app_secret": self.app_secret
        }

        response = requests.post(url, json=payload)
        data = response.json()

        if data.get("code") != 0:
            raise RuntimeError(f"Failed to get access token: {data.get('msg')}")

        self._access_token = data.get("app_access_token")
        return self._access_token

    def upload_from_file(self, file_path: str, image_type: str = "message") -> str:
        """Upload image from local file"""
        with open(file_path, "rb") as f:
            return self.upload_from_reader(f, image_type)

    def upload_from_url(self, url: str, image_type: str = "message") -> str:
        """Upload image from URL"""
        response = requests.get(url)
        response.raise_for_status()
        return self.upload_from_reader(io.BytesIO(response.content), image_type)

    def upload_from_base64(self, data: str, image_type: str = "message") -> str:
        """Upload image from base64 string"""
        decoded = base64.b64decode(data)
        return self.upload_from_reader(io.BytesIO(decoded), image_type)

    def upload_from_reader(self, reader: io.IOBase, image_type: str = "message") -> str:
        """Upload image from any reader (file-like object)"""
        # Check file size (10MB limit)
        reader.seek(0, os.SEEK_END)
        size = reader.tell()
        if size > 10 * 1024 * 1024:
            raise ValueError("Image size exceeds 10MB limit")
        reader.seek(0)

        # Prepare multipart/form-data request
        url = f"{self.API_BASE_URL}{self.UPLOAD_ENDPOINT}"
        headers = {
            "Authorization": f"Bearer {self.get_access_token()}"
        }

        # Reset reader position before reading
        files = {
            "image": reader,
            "image_type": (None, image_type)
        }

        response = requests.post(url, headers=headers, files=files)
        data = response.json()

        if data.get("code") != 0:
            raise RuntimeError(f"Upload failed: {data.get('msg')}")

        return data["data"]["image_key"]


def load_config() -> tuple[str, str]:
    """Load Feishu credentials from goclaw config file (~/.goclaw/config.json)"""
    # Try to load from goclaw config file ~/.goclaw/config.json
    config_path = Path.home() / ".goclaw" / "config.json"
    if config_path.exists():
        with open(config_path) as f:
            config = json.load(f)
            # Read from channels.feishu section
            feishu_config = config.get("channels", {}).get("feishu", {})
            app_id = feishu_config.get("app_id")
            app_secret = feishu_config.get("app_secret")

            if app_id and app_secret:
                return app_id, app_secret

    # Fallback to environment variables
    app_id = os.environ.get("FEISHU_APP_ID")
    app_secret = os.environ.get("FEISHU_APP_SECRET")

    if not app_id or not app_secret:
        print("Error: Feishu credentials not found.")
        print("Configure in ~/.goclaw/config.json under channels.feishu:")
        print('  {')
        print('    "channels": {')
        print('      "feishu": {')
        print('        "app_id": "your_app_id",')
        print('        "app_secret": "your_app_secret"')
        print('      }')
        print('    }')
        print('  }')
        sys.exit(1)

    return app_id, app_secret


def main():
    parser = argparse.ArgumentParser(
        description="Upload images to Feishu/Lark",
        prog="feishu-upload-image"
    )
    parser.add_argument(
        "input",
        nargs="?",
        help="Image file path, URL, or use --base64 flag"
    )
    parser.add_argument(
        "-t", "--type",
        choices=["message", "avatar"],
        default="message",
        help="Image type (default: message)"
    )
    parser.add_argument(
        "--image-type",
        choices=["message", "avatar"],
        dest="image_type",
        help="Same as --type (for compatibility)"
    )
    parser.add_argument(
        "-u", "--url",
        help="Upload from URL"
    )
    parser.add_argument(
        "-b", "--base64",
        action="store_true",
        help="Treat input as base64 string"
    )
    parser.add_argument(
        "-o", "--output",
        choices=["key", "json"],
        default="key",
        help="Output format (default: key)"
    )
    parser.add_argument(
        "-q", "--quiet",
        action="store_true",
        help="Only output the image_key"
    )

    args = parser.parse_args()

    # Resolve image_type
    image_type = args.image_type or args.type

    # Load credentials
    app_id, app_secret = load_config()

    # Create uploader
    uploader = FeishuImageUploader(app_id, app_secret)

    try:
        if args.url:
            # Upload from URL
            image_key = uploader.upload_from_url(args.url, image_type)
        elif args.base64:
            # Upload from base64
            if not args.input:
                print("Error: base64 string required when using --base64", file=sys.stderr)
                sys.exit(1)
            image_key = uploader.upload_from_base64(args.input, image_type)
        elif args.input:
            # Detect if input is a URL
            if args.input.startswith(("http://", "https://")):
                image_key = uploader.upload_from_url(args.input, image_type)
            else:
                # Upload from local file
                image_key = uploader.upload_from_file(args.input, image_type)
        else:
            parser.print_help()
            sys.exit(1)

        # Output result
        if args.output == "json":
            result = {"code": 0, "data": {"image_key": image_key}}
            print(json.dumps(result, ensure_ascii=False))
        elif args.quiet:
            print(image_key)
        else:
            print(f"image_key: {image_key}")

    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        if args.output == "json":
            err_json = {"code": -1, "msg": str(e)}
            print(json.dumps(err_json, ensure_ascii=False))
        sys.exit(1)


if __name__ == "__main__":
    main()
