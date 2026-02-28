package tools

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/smallnest/goclaw/config"
	"go.uber.org/zap"
)

// ShellTool Shell 工具
type ShellTool struct {
	enabled       bool
	allowedCmds   []string
	deniedCmds    []string
	timeout       time.Duration
	workingDir    string
	sandboxConfig config.SandboxConfig
	dockerClient  *client.Client
}

// NewShellTool 创建 Shell 工具
func NewShellTool(
	enabled bool,
	allowedCmds, deniedCmds []string,
	timeout int,
	workingDir string,
	sandboxConfig config.SandboxConfig,
) *ShellTool {
	var t time.Duration
	if timeout > 0 {
		t = time.Duration(timeout) * time.Second
	} else {
		t = 120 * time.Second
	}

	st := &ShellTool{
		enabled:       enabled,
		allowedCmds:   allowedCmds,
		deniedCmds:    deniedCmds,
		timeout:       t,
		workingDir:    workingDir,
		sandboxConfig: sandboxConfig,
	}

	// 如果启用沙箱，初始化 Docker 客户端
	if sandboxConfig.Enabled {
		if cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation()); err == nil {
			st.dockerClient = cli
		} else {
			zap.L().Warn("Failed to initialize Docker client, sandbox disabled", zap.Error(err))
			st.sandboxConfig.Enabled = false
		}
	}

	return st
}

// Exec 执行 Shell 命令
func (t *ShellTool) Exec(ctx context.Context, params map[string]interface{}) (string, error) {
	if !t.enabled {
		return "", fmt.Errorf("shell tool is disabled")
	}

	command, ok := params["command"].(string)
	if !ok {
		return "", fmt.Errorf("command parameter is required")
	}

	// 检查危险命令
	if t.isDenied(command) {
		return "", fmt.Errorf("command is not allowed: %s", command)
	}

	// 根据是否启用沙箱选择执行方式
	if t.sandboxConfig.Enabled && t.dockerClient != nil {
		return t.execInSandbox(ctx, command)
	}
	return t.execDirect(ctx, command)
}

// execDirect 直接执行命令
func (t *ShellTool) execDirect(ctx context.Context, command string) (string, error) {
	// 创建带超时的上下文
	cmdCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// 执行命令
	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	if t.workingDir != "" {
		cmd.Dir = t.workingDir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// execInSandbox 在 Docker 容器中执行命令
func (t *ShellTool) execInSandbox(ctx context.Context, command string) (string, error) {
	containerName := fmt.Sprintf("goclaw-%d", time.Now().UnixNano())

	// 准备工作目录
	workdir := t.workingDir
	if workdir == "" {
		workdir = "."
	}

	// 准备挂载点
	binds := []string{
		workdir + ":" + t.sandboxConfig.Workdir,
	}

	// 创建并运行容器
	resp, err := t.dockerClient.ContainerCreate(ctx, &container.Config{
		Image:      t.sandboxConfig.Image,
		Cmd:        []string{"sh", "-c", command},
		WorkingDir: t.sandboxConfig.Workdir,
		Tty:        false,
	}, &container.HostConfig{
		Binds:       binds,
		NetworkMode: container.NetworkMode(t.sandboxConfig.Network),
		Privileged:  t.sandboxConfig.Privileged,
		AutoRemove:  t.sandboxConfig.Remove,
	}, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// 确保容器被清理
	if !t.sandboxConfig.Remove {
		defer func() {
			_ = t.dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{
				Force: true,
			})
		}()
	}

	// 启动容器
	if err := t.dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	// 等待容器完成
	statusCh, errCh := t.dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return "", fmt.Errorf("container wait error: %w", err)
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return "", fmt.Errorf("container exited with code %d", status.StatusCode)
		}
	case <-ctx.Done():
		return "", ctx.Err()
	}

	// 获取日志
	out, err := t.dockerClient.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %w", err)
	}
	defer out.Close()

	// 读取输出
	logs, err := io.ReadAll(out)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return string(logs), nil
}

// isDenied 检查命令是否被拒绝
func (t *ShellTool) isDenied(command string) bool {
	// 检查明确拒绝的命令
	for _, denied := range t.deniedCmds {
		if strings.Contains(command, denied) {
			return true
		}
	}

	// 如果有允许列表，检查是否在允许列表中
	if len(t.allowedCmds) > 0 {
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return true
		}
		cmdName := parts[0]

		for _, allowed := range t.allowedCmds {
			if cmdName == allowed {
				return false
			}
		}
		return true
	}

	return false
}

// GetTools 获取所有 Shell 工具
func (t *ShellTool) GetTools() []Tool {
	var desc strings.Builder
	desc.WriteString("Execute a shell command")

	if t.sandboxConfig.Enabled {
		desc.WriteString(" inside a Docker sandbox container. Commands run in a containerized environment with network isolation.")
	} else {
		desc.WriteString(" on the host system")
	}

	desc.WriteString(". Use this for file operations, running scripts (Python, Node.js, etc.), installing dependencies, HTTP requests (curl), system diagnostics and more. Commands run in a non-interactive shell.")

	return []Tool{
		NewBaseTool(
			"run_shell",
			desc.String(),
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Shell command to execute",
					},
				},
				"required": []string{"command"},
			},
			t.Exec,
		),
	}
}

// Close 关闭工具
func (t *ShellTool) Close() error {
	if t.dockerClient != nil {
		return t.dockerClient.Close()
	}
	return nil
}
