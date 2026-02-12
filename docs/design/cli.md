# GoClaw CLI Cheatsheet

GoClaw 是一个 Go 语言实现的 AI Agent 框架，提供完整的命令行工具来管理 agents、channels、gateway 和系统功能。

## 目录

- [基本命令](#基本命令)
- [Agent 管理](#agent-管理)
- [Channel 管理](#channel-管理)
- [Gateway 管理](#gateway-管理)
- [Cron 定时任务](#cron-定时任务)
- [Browser 自动化](#browser-自动化)
- [System 控制](#system-控制)
- [Memory 管理](#memory-管理)
- [Sessions 管理](#sessions-管理)
- [Skills 管理](#skills-管理)
- [Approvals 审批](#approvals-审批)
- [Logs 日志](#logs-日志)
- [Health & Status](#health--status)

---

## 基本命令

```bash
# 显示帮助
goclaw --help
goclaw [command] --help

# 启动 goclaw agent 服务（后台运行）
goclaw start

# 交互式终端 UI
goclaw tui

# 单次执行
goclaw agent --message "你好"

# 配置管理
goclaw config show
```

---

## Agent 管理

### 运行单个 Agent 交互

```bash
# 基本用法
goclaw agent --message "你好"

# 指定 channel
goclaw agent --message "测试" --channel telegram

# 指定 session
goclaw agent --message "继续" --session-id abc123

# 本地模式（不连接 channels）
goclaw agent --message "本地测试" --local

# 显示思考过程
goclaw agent --message "解释这段代码" --thinking

# 设置超时（秒）
goclaw agent --message "长任务" --timeout 300

# JSON 输出
goclaw agent --message "测试" --json
```

### 管理 Isolated Agents

```bash
# 列出所有 agents
goclaw agents list

# 添加新 agent（交互式）
goclaw agents add

# 添加新 agent（非交互式）
goclaw agents add my-agent --workspace /path/to/workspace --model claude-3-5-sonnet-20241022

# 删除 agent
goclaw agents delete my-agent

# 删除 agent（强制）
goclaw agents delete my-agent --force
```

---

## Channel 管理

```bash
# 列出所有 channels
goclaw channels list

# 检查 channel 状态
goclaw channels status

# 添加 channel
goclaw channels add --channel telegram --account mybot --name "My Bot" --token $TELEGRAM_BOT_TOKEN

# 添加 Discord channel
goclaw channels add --channel discord --account work --name "Work Bot" --token $DISCORD_BOT_TOKEN

# 删除 channel
goclaw channels remove --channel discord --account work

# 删除 channel（包括配置）
goclaw channels remove --channel discord --account work --delete

# 登录到 channel
goclaw channels login --channel whatsapp

# 从 channel 登出
goclaw channels logout --channel whatsapp

# 查看 channel 日志
goclaw channels logs --channel telegram --lines 100

# Channel 状态探测
goclaw channels status --probe
```

---

## Gateway 管理

### Gateway 服务管理

```bash
# 运行 WebSocket Gateway
goclaw gateway run

# 自定义端口运行
goclaw gateway run --port 8080

# 绑定地址
goclaw gateway run --bind 0.0.0.0 --port 28789

# 使用 Tailscale
goclaw gateway run --tailscale

# 开发模式
goclaw gateway run --dev
```

### Gateway 系统服务

```bash
# 安装为系统服务
goclaw gateway install

# 安装服务（自定义端口）
goclaw gateway install --port 8080

# 启动服务
goclaw gateway start

# 停止服务
goclaw gateway stop

# 重启服务
goclaw gateway restart

# 卸载服务
goclaw gateway uninstall
```

### Gateway 状态检查

```bash
# 查看 gateway 状态
goclaw gateway status

# 深度检查
goclaw gateway status --deep

# 探测连接性
goclaw gateway probe

# 健康检查
goclaw gateway health

# RPC 调用
goclaw gateway call config.get
goclaw gateway call skills.list --params '{"limit": 10}'
```

---

## Cron 定时任务

```bash
# 查看调度器状态
goclaw cron status

# JSON 格式输出
goclaw cron status --json

# 列出所有任务
goclaw cron list

# 列出所有任务（包括禁用的）
goclaw cron list --all

# JSON 格式输出
goclaw cron list --json
```

### 添加定时任务

```bash
# 添加任务（交互式）
goclaw cron add

# 定时执行（每天 14:30）
goclaw cron add --name "Daily Report" --at "14:30" --message "生成日报"

# 间隔执行（每小时）
goclaw cron add --name "Hourly Check" --every "1h" --system-event "health_check"

# 使用 cron 表达式
goclaw cron add --name "Weekly Backup" --cron "0 2 * * 0" --message "执行备份"

# 每天早上 9 点（工作日）
goclaw cron add --name "Morning Briefing" --cron "0 9 * * 1-5" --message "早报"
```

### 编辑定时任务

```bash
# 编辑任务名称
goclaw cron edit job-1234567890 --name "New Name"

# 修改调度时间
goclaw cron edit job-1234567890 --at "10:00"

# 修改为间隔执行
goclaw cron edit job-1234567890 --every "2h"

# 修改为 cron 表达式
goclaw cron edit job-1234567890 --cron "0 */6 * * *"

# 修改消息
goclaw cron edit job-1234567890 --message "更新后的消息"

# 启用任务
goclaw cron edit job-1234567890 --enable

# 禁用任务
goclaw cron edit job-1234567890 --disable

# 组合修改
goclaw cron edit job-1234567890 --name "Updated" --at "10:00" --enable
```

### 管理定时任务

```bash
# 立即运行任务
goclaw cron run job-1234567890

# 强制运行（即使禁用）
goclaw cron run job-1234567890 --force

# 查看运行历史
goclaw cron runs --id job-1234567890

# 查看最近 20 次运行
goclaw cron runs --id job-1234567890 --limit 20

# 启用任务
goclaw cron enable job-1234567890

# 禁用任务
goclaw cron disable job-1234567890

# 删除任务
goclaw cron rm job-1234567890
```

---

## Browser 自动化

### Browser 管理

```bash
# 查看浏览器状态
goclaw browser status

# 启动浏览器
goclaw browser start

# 停止浏览器
goclaw browser stop

# 重置浏览器配置
goclaw browser reset-profile

# 列出所有标签页
goclaw browser tabs

# 列出所有标签页（新方法）
goclaw browser focus --list

# 切换到指定标签
goclaw browser focus <targetId>
```

### Browser 操作

```bash
# 打开 URL
goclaw browser open https://example.com

# 导航到 URL
goclaw browser navigate https://example.com

# 截图
goclaw browser screenshot

# 截图（指定标签）
goclaw browser screenshot <targetId>

# 页面快照（HTML + 截图）
goclaw browser snapshot

# 调整视口大小
goclaw browser resize 1920 1080
```

### Browser 交互

```bash
# 点击元素
goclaw browser click "#submit-button"

# 输入文本
goclaw browser type "#username" "myuser"

# 按键
goclaw browser press "Enter"
goclaw browser press "Escape"
goclaw browser press "Tab"

# 悬停
goclaw browser hover "#menu-item"

# 选择下拉选项
goclaw browser select "#country" "China"

# 上传文件
goclaw browser upload "#file-input" /path/to/file.pdf

# 填充表单字段
goclaw browser fill "#email" "user@example.com"

# 处理对话框
goclaw browser dialog accept
goclaw browser dialog dismiss
goclaw browser dialog accept "提示文本"

# 等待元素
goclaw browser wait "#loaded-element"
goclaw browser wait "#element" 30

# 评估 JavaScript
goclaw browser evaluate "document.title"

# 获取控制台日志
goclaw browser console
goclaw browser console --errors-only
goclaw browser console --warnings-only
goclaw browser console --info-only
goclaw browser console --max=50

# 保存为 PDF
goclaw browser pdf
goclaw browser pdf output.pdf

# 关闭标签
goclaw browser close
goclaw browser close <targetId>

# 管理配置文件
goclaw browser profiles
```

---

## System 控制

```bash
# 发送系统事件
goclaw system event --text "系统重启"

# 指定事件模式
goclaw system event --text "测试" --mode test

# 心跳控制
goclaw system heartbeat last
goclaw system heartbeat enable
goclaw system heartbeat disable

# 列出在线连接
goclaw system presence
```

---

## Memory 管理

```bash
# 查看内存状态
goclaw memory status

# 重新索引内存文件
goclaw memory index

# 语义搜索
goclaw memory search "如何配置 API"

# 搜索并限制结果
goclaw memory search "配置" --limit 5
```

---

## Sessions 管理

```bash
# 列出所有会话
goclaw sessions list

# 详细输出
goclaw sessions list --verbose

# JSON 输出
goclaw sessions list --json

# 只显示活动会话
goclaw sessions list --active

# 指定存储目录
goclaw sessions list --store /path/to/sessions
```

---

## Skills 管理

```bash
# 列出所有技能
goclaw skills list

# 只列出已就绪的技能
goclaw skills list --eligible

# 详细输出
goclaw skills list -v

# 查看技能详情
goclaw skills info <skill-name>

# 检查技能状态
goclaw skills check

# 安装技能
goclaw skills install <skill-url>

# 安装本地技能
goclaw skills install /path/to/skill

# 更新技能
goclaw skills update <skill-name>

# 卸载技能
goclaw skills uninstall <skill-name>

# 验证技能依赖
goclaw skills validate <skill-name>

# 测试技能
goclaw skills test <skill-name> --prompt "测试提示"
```

### Skills 配置

```bash
# 技能配置管理
goclaw skills config list <skill-name>
goclaw skills config get <skill-name> <key>
goclaw skills config set <skill-name> <key> <value>
goclaw skills config unset <skill-name> <key>
```

---

## Approvals 审批

```bash
# 获取审批设置
goclaw approvals get

# 设置审批行为
goclaw approvals set auto
goclaw approvals set manual
goclaw approvals set prompt

# 允许列表管理
goclaw approvals allowlist add web_search
goclaw approvals allowlist add browser
goclaw approvals allowlist add shell_command

# 移除允许列表项
goclaw approvals allowlist remove web_search

# 查看允许列表
goclaw approvals get
```

---

## Logs 日志

```bash
# 查看日志（默认 100 行）
goclaw logs

# 实时跟踪日志
goclaw logs -f

# 指定行数
goclaw logs -n 500

# 指定日志文件
goclaw logs -f /var/log/goclaw/gateway.log

# JSON 输出
goclaw logs --json

# 禁用颜色
goclaw logs --no-color

# 纯文本输出
goclaw logs --plain
```

---

## Health & Status

```bash
# 健康检查
goclaw health

# JSON 输出
goclaw health --json

# 设置超时
goclaw health --timeout 10

# 详细输出
goclaw health -v
```

### 状态查看

```bash
# 基本状态
goclaw status

# 所有会话
goclaw status --all

# 深度扫描
goclaw status --deep

# 显示资源使用
goclaw status --usage

# JSON 输出
goclaw status --json

# 调试输出
goclaw status --debug

# 详细输出
goclaw status -v
```

---

## 高级用法

### 环境变量

```bash
# 设置 API Key
export ANTHROPIC_API_KEY="your-key"
export OPENAI_API_KEY="your-key"

# 设置 Gateway URL
export GOCRAW_GATEWAY_URL="ws://localhost:28789"
export GOCRAW_GATEWAY_TOKEN="your-token"
```

### 配置文件

```bash
# 默认配置位置
~/.goclaw/config.yaml

# 工作区
~/.goclaw/workspace/

# 会话存储
~/.goclaw/sessions/

# 日志目录
~/.goclaw/logs/
```

### 组合命令

```bash
# 启动 agent 并指定模型
goclaw start --model claude-3-5-sonnet-20241022

# 添加 agent 并绑定 channel
goclaw agents add myagent --workspace /path --bind telegram --bind discord

# 安装 gateway 服务并自定义端口
goclaw gateway install --port 8080 && goclaw gateway start

# Cron 任务组合
goclaw cron add --name "Daily" --at "09:00" --message "日报" && goclaw cron enable $(goclaw cron list --json | jq -r '.[0].id')
```

---

## 故障排查

```bash
# 检查配置
goclaw config show

# 检查 gateway 连接
goclaw gateway probe

# 深度健康检查
goclaw gateway status --deep

# 查看 channel 日志
goclaw channels logs --channel all --lines 200

# 检查技能依赖
goclaw skills check

# 验证配置
goclay doctor
```

---

## 参考资源

- [项目文档](https://docs.openclaw.ai)
- [GitHub 仓库](https://github.com/smallnest/goclaw)
- [问题反馈](https://github.com/smallnest/goclaw/issues)
