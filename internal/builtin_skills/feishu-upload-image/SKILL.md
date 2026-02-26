---
name: feishu-upload-image
description: Upload images to Feishu/Lark. Returns image_key for use in messages.
keywords: feishu, lark, upload, image, 图片上传, 飞书
---

# Feishu Upload Image

Upload images to Feishu/Lark and get the `image_key` for use in messages.

## Usage

### Upload from Local File

```bash
python3 ./scripts/upload_image.py /path/to/image.jpg
```

### Upload via HTTP

```bash
python3 ./scripts/upload_image.py --type message --image-type message https://example.com/image.jpg
```

### Upload from Base64

```bash
python3 ./scripts/upload_image.py --base64 "iVBORw0KGgoAAAANS..."
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `--type message|avatar` | Image usage type | `message` |
| `--image-type message|avatar` | Same as `--type` | `message` |

## Limits

- Max file size: 10MB
- Supported formats: JPEG, PNG, WEBP, GIF, TIFF, BMP, ICO
- GIF resolution ≤ 2000×2000
- Other formats ≤ 12000×12000

## Response

Returns `image_key` which can be used in Feishu messages:

```json
{
  "code": 0,
  "data": {
    "image_key": "img_v2_xxx"
  }
}
```

## 发送图片消息指南

### 获取必要信息

1. **飞书访问令牌** (tenant_access_token 或 app_access_token)
2. **接收者ID** (open_id, user_id, chat_id, 或 email)

### 发送图片消息API

**方式1：直接发送图片消息**
```bash
curl -X POST https://open.feishu.cn/open-apis/im/v1/messages \
  -H "Authorization: Bearer {access_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "receive_id": "{chat_id}",
    "msg_type": "image",
    "content": "{\"image_key\":\"{image_key}\"}"
  }'
```

**方式2：在富文本中嵌入图片**
```bash
curl -X POST https://open.feishu.cn/open-apis/im/v1/messages \
  -H "Authorization: Bearer {access_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "receive_id": "{chat_id}",
    "msg_type": "post",
    "content": "{\"post\":{\"zh_cn\":{\"title\":\"图片消息\",\"content\":[[{\"tag\":\"img\",\"image_key\":\"{image_key}\"}]]}}}"
  }'
```

### 获取群聊chat_id

1. 在飞书中打开群聊，查看URL中的chat_id
2. 或者通过API查询群聊列表：
```bash
curl -X GET https://open.feishu.cn/open-apis/im/v1/chats \
  -H "Authorization: Bearer {access_token}"
```

### 实践经验

- 上传图片后获得 `image_key`，有效期通常为7天
- 使用前需确保有正确的访问令牌权限
- 发送消息需要 `im:message:send_as_bot` 权限
- 建议先上传图片，再发送消息，两步操作分离
