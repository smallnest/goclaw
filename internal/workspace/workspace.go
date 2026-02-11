package workspace

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed templates/*.md
var templatesFS embed.FS

// BootstrapFiles 定义所有 bootstrap 文件
var BootstrapFiles = []string{
	"AGENTS.md",
	"SOUL.md",
	"IDENTITY.md",
	"USER.md",
	"TOOLS.md",
	"HEARTBEAT.md",
	"BOOT.md",
	"BOOTSTRAP.md",
}

// Manager 管理 workspace 目录
type Manager struct {
	workspaceDir string
}

// NewManager 创建 workspace 管理器
func NewManager(workspaceDir string) *Manager {
	return &Manager{
		workspaceDir: workspaceDir,
	}
}

// GetDefaultWorkspaceDir 获取默认 workspace 目录
func GetDefaultWorkspaceDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".goclaw", "workspace"), nil
}

// Ensure 确保 workspace 目录存在且包含所有必要的文件
func (m *Manager) Ensure() error {
	// 确保 workspace 目录存在
	if err := os.MkdirAll(m.workspaceDir, 0755); err != nil {
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// 复制缺失的 bootstrap 文件
	for _, filename := range BootstrapFiles {
		if err := m.ensureFile(filename); err != nil {
			return fmt.Errorf("failed to ensure %s: %w", filename, err)
		}
	}

	// 确保 memory 目录存在
	memoryDir := filepath.Join(m.workspaceDir, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return fmt.Errorf("failed to create memory directory: %w", err)
	}

	// 创建今日日志（如果不存在）
	todayFile := filepath.Join(memoryDir, time.Now().Format("2006-01-02")+".md")
	if _, err := os.Stat(todayFile); os.IsNotExist(err) {
		if err := m.createTodayLog(todayFile); err != nil {
			return fmt.Errorf("failed to create today's log: %w", err)
		}
	}

	// 创建心跳状态文件（如果不存在）
	heartbeatStateFile := filepath.Join(memoryDir, "heartbeat-state.json")
	if _, err := os.Stat(heartbeatStateFile); os.IsNotExist(err) {
		if err := m.createHeartbeatState(heartbeatStateFile); err != nil {
			return fmt.Errorf("failed to create heartbeat state: %w", err)
		}
	}

	return nil
}

// ensureFile 确保单个文件存在，不存在则从模板复制
func (m *Manager) ensureFile(filename string) error {
	targetPath := filepath.Join(m.workspaceDir, filename)

	// 检查文件是否已存在
	if _, err := os.Stat(targetPath); err == nil {
		// 文件存在，不需要处理
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// 从模板读取
	templatePath := filepath.Join("templates", filename)
	content, err := templatesFS.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %w", filename, err)
	}

	// 写入目标文件
	if err := os.WriteFile(targetPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filename, err)
	}

	return nil
}

// createTodayLog 创建今日日志文件
func (m *Manager) createTodayLog(path string) error {
	today := time.Now().Format("2006-01-02")
	content := fmt.Sprintf("# %s\n\nDaily log for this date.\n\n## Activities\n\n_(Add activities here as the day progresses)_\n\n## Notes\n\n_(Add notes here as the day progresses)_\n", today)
	return os.WriteFile(path, []byte(content), 0644)
}

// createHeartbeatState 创建心跳状态文件
func (m *Manager) createHeartbeatState(path string) error {
	content := `{
  "lastChecks": {
    "email": null,
    "calendar": null,
    "weather": null
  }
}`
	return os.WriteFile(path, []byte(content), 0644)
}

// ReadBootstrapFile 读取 bootstrap 文件内容
func (m *Manager) ReadBootstrapFile(filename string) (string, error) {
	path := filepath.Join(m.workspaceDir, filename)
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(content), nil
}

// ReadTodayLog 读取今日日志
func (m *Manager) ReadTodayLog() (string, error) {
	today := time.Now().Format("2006-01-02")
	path := filepath.Join(m.workspaceDir, "memory", today+".md")
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(content), nil
}

// AppendTodayLog 追加内容到今日日志
func (m *Manager) AppendTodayLog(content string) error {
	memoryDir := filepath.Join(m.workspaceDir, "memory")
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return err
	}

	today := time.Now().Format("2006-01-02")
	path := filepath.Join(memoryDir, today+".md")

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// 如果文件不为空，添加换行
	if info, err := file.Stat(); err == nil && info.Size() > 0 {
		if _, err := file.WriteString("\n\n"); err != nil {
			return err
		}
	}

	if _, err := file.WriteString(content); err != nil {
		return err
	}

	return nil
}

// ReadMemoryFile 读取 memory 目录下的文件
func (m *Manager) ReadMemoryFile(filename string) (string, error) {
	path := filepath.Join(m.workspaceDir, "memory", filename)
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(content), nil
}

// ListMemoryFiles 列出 memory 目录下的所有文件
func (m *Manager) ListMemoryFiles() ([]string, error) {
	memoryDir := filepath.Join(m.workspaceDir, "memory")
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

// GetWorkspaceDir 获取 workspace 目录路径
func (m *Manager) GetWorkspaceDir() string {
	return m.workspaceDir
}

// ListFiles 列出 workspace 目录下的所有文件
func (m *Manager) ListFiles() ([]string, error) {
	entries, err := os.ReadDir(m.workspaceDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

// CopyFromFS 从嵌入的文件系统复制所有模板文件到指定目录
func CopyFromFS(targetDir string) error {
	// 确保目标目录存在
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// 遍历嵌入的文件系统
	return fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if d.IsDir() {
			return nil
		}

		// 计算相对路径
		relPath, err := filepath.Rel("templates", path)
		if err != nil {
			return err
		}

		// 读取模板内容
		content, err := templatesFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		// 写入目标文件
		targetPath := filepath.Join(targetDir, relPath)
		if err := os.WriteFile(targetPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", relPath, err)
		}

		return nil
	})
}
