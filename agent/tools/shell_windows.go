//go:build windows

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// killProcessTree 在Windows上终止进程树
func killProcessTree(pid int) error {
	// 使用taskkill命令终止进程树
	// /F - 强制终止
	// /T - 终止进程树（包括子进程）
	// /PID - 指定进程ID
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	return cmd.Run()
}

// waitForCommandResult 等待命令执行结果并处理超时和取消 (Windows版本)
// 在Windows平台上，使用taskkill命令终止进程树来替代Unix的信号机制
// 当命令执行完成、超时或上下文被取消时，返回相应的结果或错误
func (t *ShellTool) waitForCommandResult(ctx context.Context, cmd *exec.Cmd, resultCh chan result) (string, error) {
	select {
	case res := <-resultCh:
		if res.err != nil {
			return "", fmt.Errorf("command failed: %w, output: %s", res.err, string(res.output))
		}
		return string(res.output), nil
	case <-time.After(t.timeout):
		// 超时：强制杀死进程树
		if cmd.Process != nil {
			// Windows使用taskkill命令终止进程树
			if err := killProcessTree(cmd.Process.Pid); err != nil {
				// 如果taskkill失败，尝试直接杀死进程
				cmd.Process.Kill()
			}
		}
		return "", fmt.Errorf("command timed out after %v", t.timeout)
	case <-ctx.Done():
		// 父 context 被取消
		if cmd.Process != nil {
			// Windows使用taskkill命令终止进程树
			if err := killProcessTree(cmd.Process.Pid); err != nil {
				// 如果taskkill失败，尝试直接杀死进程
				cmd.Process.Kill()
			}
		}
		return "", ctx.Err()
	}
}
