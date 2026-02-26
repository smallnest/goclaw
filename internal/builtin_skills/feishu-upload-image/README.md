# Feishu Upload Image

Upload images to Feishu/Lark and get the `image_key` for use in messages.

## Configuration

The script reads Feishu credentials from your goclaw config file: `~/.goclaw/config.json`

Configure under `channels.feishu`:

```json
{
  "channels": {
    "feishu": {
      "app_id": "your_app_id",
      "app_secret": "your_app_secret"
    }
  }
}
```

## Usage

### Upload local file

```bash
python3 ./scripts/upload_image.py /path/to/image.jpg
# Output: image_key: img_v2_xxx
```

### Upload from URL

```bash
python3 ./scripts/upload_image.py --url https://example.com/image.jpg
```

### Upload from base64

```bash
python3 ./scripts/upload_image.py --base64 "iVBORw0KGgoAAAANS..."
```

### Output formats

Just the image_key (quiet mode):

```bash
python3 ./scripts/upload_image.py image.png --quiet
img_v2_xxx
```

Full JSON response:

```bash
python3 ./scripts/upload_image.py image.png --output json
{"code":0,"data":{"image_key":"img_v2_xxx"}}
```

## Using image_key in Feishu Messages

After uploading, use the `image_key` to send images:

```json
{
  "msg_type": "image",
  "content": {
    "image_key": "img_v2_xxx"
  }
}
```
