package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/emulation"
	"github.com/mafredri/cdp/protocol/network"
	"github.com/mafredri/cdp/rpcc"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// BrowserSessionManager 浏览器会话管理器 (使用 Chrome DevTools Protocol)
type BrowserSessionManager struct {
	mu          sync.RWMutex
	devt        *devtool.DevTools
	client      *cdp.Client
	conn        *rpcc.Conn
	cmd         *exec.Cmd
	ready       bool
	chromePath  string
	userDataDir string
	remoteURL   string // 远程 Chrome 实例 URL
}

var sessionManager *BrowserSessionManager

// GetBrowserSession 获取浏览器会话管理器（单例）
func GetBrowserSession() *BrowserSessionManager {
	if sessionManager == nil {
		sessionManager = &BrowserSessionManager{}
	}
	return sessionManager
}

// Start 启动浏览器会话
func (b *BrowserSessionManager) Start(timeout time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ready {
		return nil
	}

	logger.Info("Starting persistent browser session with Chrome DevTools Protocol")

	// 首先尝试连接到已运行的 Chrome 实例
	if err := b.tryConnectToExisting(); err == nil {
		b.ready = true
		logger.Info("Connected to existing Chrome instance")
		return nil
	}

	logger.Info("No existing Chrome found, starting new instance")

	// 查找 Chrome 可执行文件
	chromePath, err := b.findChrome()
	if err != nil {
		return fmt.Errorf("failed to find Chrome: %w", err)
	}
	b.chromePath = chromePath

	// 创建用户数据目录
	userDataDir, err := os.MkdirTemp("", "goclaw-chrome-")
	if err != nil {
		return fmt.Errorf("failed to create user data dir: %w", err)
	}
	b.userDataDir = userDataDir

	// 启动 Chrome
	b.cmd = exec.Command(chromePath,
		"--headless=new",
		"--no-sandbox",
		"--disable-setuid-sandbox",
		"--disable-dev-shm-usage",
		"--disable-gpu",
		"--disable-software-rasterizer",
		"--remote-debugging-port=9222",
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	)

	if err := b.cmd.Start(); err != nil {
		os.RemoveAll(userDataDir)
		return fmt.Errorf("failed to start Chrome: %w", err)
	}

	// 等待 Chrome 启动
	select {
	case <-time.After(timeout):
		_ = b.cmd.Process.Kill()
		os.RemoveAll(userDataDir)
		return fmt.Errorf("Chrome did not start within timeout")
	case <-time.After(3 * time.Second):
		// 继续连接
	}

	// 连接到 Chrome
	if err := b.connect(9222); err != nil {
		_ = b.cmd.Process.Kill()
		os.RemoveAll(userDataDir)
		return fmt.Errorf("failed to connect to Chrome: %w", err)
	}

	b.ready = true
	logger.Info("Browser session started successfully with Chrome DevTools Protocol")
	return nil
}

// tryConnectToExisting 尝试连接到已运行的 Chrome 实例
func (b *BrowserSessionManager) tryConnectToExisting() error {
	// 尝试连接默认端口
	for _, port := range []int{9222, 9223, 9224} {
		if err := b.connect(port); err == nil {
			b.remoteURL = fmt.Sprintf("http://localhost:%d", port)
			return nil
		}
	}
	return fmt.Errorf("no existing Chrome instance found")
}

// connect 连接到指定端口的 Chrome 实例
func (b *BrowserSessionManager) connect(port int) error {
	// 使用 devtool 包
	b.devt = devtool.New(fmt.Sprintf("http://localhost:%d", port))

	// 列出可用的页面
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pt, err := b.devt.Get(ctx, devtool.Page)
	if err != nil {
		// 如果没有页面，创建新标签页
		pt, err = b.devt.Create(ctx)
		if err != nil {
			return fmt.Errorf("failed to create page: %w", err)
		}
	}

	// 连接到 WebSocket
	conn, err := rpcc.DialContext(ctx, pt.WebSocketDebuggerURL)
	if err != nil {
		return fmt.Errorf("failed to dial WebSocket: %w", err)
	}

	b.conn = conn

	// 创建 CDP 客户端
	b.client = cdp.NewClient(conn)

	// 启用需要的域
	if err := b.client.DOM.Enable(ctx); err != nil {
		return fmt.Errorf("failed to enable DOM: %w", err)
	}
	if err := b.client.Page.Enable(ctx); err != nil {
		return fmt.Errorf("failed to enable Page: %w", err)
	}
	if err := b.client.Runtime.Enable(ctx); err != nil {
		return fmt.Errorf("failed to enable Runtime: %w", err)
	}
	if err := b.client.Network.Enable(ctx, network.NewEnableArgs()); err != nil {
		return fmt.Errorf("failed to enable Network: %w", err)
	}

	// 设置真实的 User-Agent 以避免被检测为自动化工具
	// 使用最新 Chrome 的 User-Agent
	userAgent := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	if err := b.client.Emulation.SetUserAgentOverride(ctx, emulation.NewSetUserAgentOverrideArgs(userAgent)); err != nil {
		logger.Warn("Failed to set User-Agent", zap.Error(err))
	}

	return nil
}

// findChrome 查找 Chrome 可执行文件
func (b *BrowserSessionManager) findChrome() (string, error) {
	// 常见 Chrome 路径
	paths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"/mnt/c/Program Files/Google/Chrome/Application/chrome.exe", // WSL
		"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
		"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// 尝试通过 which/google-chrome 命令查找
	for _, cmd := range []string{"google-chrome", "google-chrome-stable", "chromium-browser", "chromium", "chrome"} {
		if path, err := exec.LookPath(cmd); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("Chrome not found in common locations")
}

// IsReady 检查会话是否就绪
func (b *BrowserSessionManager) IsReady() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.ready
}

// GetClient 获取 CDP 客户端
func (b *BrowserSessionManager) GetClient() (*cdp.Client, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.ready {
		return nil, fmt.Errorf("browser session not ready")
	}

	return b.client, nil
}

// Stop 停止浏览器会话
func (b *BrowserSessionManager) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.ready {
		logger.Info("Stopping browser session")

		// 关闭连接
		if b.conn != nil {
			_ = b.conn.Close()
		}

		// 停止 Chrome 进程
		if b.cmd != nil && b.cmd.Process != nil {
			_ = b.cmd.Process.Kill()
			_ = b.cmd.Wait()
		}

		// 清理临时目录
		if b.userDataDir != "" {
			_ = os.RemoveAll(b.userDataDir)
		}

		b.ready = false
		b.client = nil
		b.conn = nil
		b.cmd = nil
		b.userDataDir = ""
	}
}
