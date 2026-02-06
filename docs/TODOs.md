# Goclaw Future Roadmap & TODOs

Inspired by advanced agent frameworks like `openclaw` and `nanobot`, here are the planned enhancements for `goclaw`.

## 🛡️ Security & Reliability
- [x] **Docker Sandboxing**: Implement a Docker-based executor for the `ShellTool` to isolate command execution from the host system.
- [ ] **Enhanced Whitelisting**: Granular permission controls for filesystem and network access based on the current task context.
- [ ] **Secret Scanning**: Prevent the agent from reading or accidentally leaking sensitive environment variables or configuration files.

## 🌐 Web & Browser Capabilities
- [ ] **Headless Browser Integration**: Integrate `chromedp` or `playwright-go` to handle SPA (Single Page Applications) and interactive web tasks.
- [ ] **Readability Mode**: Implement an improved HTML-to-Markdown converter using a readability-style algorithm for cleaner web content extraction.
- [ ] **Proxy Support**: Support for configurable proxies in web search and fetch tools.

## 🧠 Core Agent Intelligence
- [x] **Skill System Full Integration**: Wire up the `SkillsLoader` into the `Loop` and `ContextBuilder` to support dynamic skill activation via tools.
- [ ] **Long-term Memory (RAG)**: Implement a vector-database backed memory store for retrieving relevant historical context across different sessions.
- [ ] **Media Understanding (Audio/Video)**: Add dedicated tools and provider support for processing audio files (STT) and video analysis.

## 🛠️ Tooling & DX
- [ ] **TUI (Terminal User Interface)**: Build a rich, interactive terminal interface using `bubbletea` or `tview`.
- [ ] **Plugin System**: Allow users to add custom tools and providers as external shared libraries or via a defined plugin protocol.
- [ ] **OpenMCP Support**: Support the Model Context Protocol (MCP) for standard tool interoperability.

## 🔌 Providers & Channels
- [ ] **Streaming Support**: Implement full end-to-end streaming from LLM provider to messaging channels.
- [ ] **More Channels**: Add support for Slack, Discord, and Microsoft Teams.
- [ ] **Local LLM Support**: Better integration with `Ollama` and `vLLM` for offline usage.
