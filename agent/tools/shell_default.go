//go:build !windows

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// waitForCommandResult 等待命令执行结果并处理超时和取消 (Unix/Linux版本)
// 在Unix/Linux平台上，使用进程组和信号机制优雅地终止命令
// 当命令执行完成、超时或上下文被取消时，返回相应的结果或错误
func (t *ShellTool) waitForCommandResult(ctx context.Context, cmd *exec.Cmd, resultCh chan result) (string, error) {
	select {
	case res := <-resultCh:
		if res.err != nil {
			return "", fmt.Errorf("command failed: %w, output: %s", res.err, string(res.output))
		}
		return string(res.output), nil
	case <-time.After(t.timeout):
		// 超时：强制杀死进程组
		if cmd.Process != nil {
			// 先尝试优雅关闭（SIGTERM）
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			// 给进程一点时间清理
			time.Sleep(100 * time.Millisecond)
			// 再强制杀死（SIGKILL）
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return "", fmt.Errorf("command timed out after %v", t.timeout)
	case <-ctx.Done():
		// 父 context 被取消
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return "", ctx.Err()
	}
}
