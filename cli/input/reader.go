package input

import (
	"fmt"

	"github.com/chzyer/readline"
)

// ReadLine 读取一行输入（支持中文）
func ReadLine(prompt string) (string, error) {
	return ReadLineWithHistory(prompt, nil)
}

// ReadLineWithHistory 读取一行输入（支持历史记录）
func ReadLineWithHistory(prompt string, history []string) (string, error) {
	cfg := &readline.Config{
		Prompt:          prompt,
		HistoryLimit:    1000,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		// 确保输入输出正确配置
		UniqueEditLine: true,
		// 禁用自动完成，避免干扰
		AutoComplete: nil,
	}

	rl, err := readline.NewEx(cfg)
	if err != nil {
		return "", err
	}
	defer rl.Close()

	// 添加历史记录
	for _, h := range history {
		if h != "" {
			_ = rl.SaveHistory(h)
		}
	}

	// 读取输入
	line, err := rl.Readline()
	if err != nil {
		if err == readline.ErrInterrupt {
			return "", fmt.Errorf("interrupted")
		}
		return "", err
	}

	return line, nil
}
