package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/smallnest/dogclaw/goclaw/providers"
	"github.com/smallnest/dogclaw/goclaw/session"
	"go.uber.org/zap"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusBlocked    TaskStatus = "blocked"
)

// TaskReflection 任务反思结果
type TaskReflection struct {
	Status         TaskStatus `json:"status"`
	Confidence     float64    `json:"confidence"`
	CompletedSteps []string   `json:"completed_steps"`
	RemainingSteps []string   `json:"remaining_steps"`
	Reasoning      string     `json:"reasoning"`
	NextAction     string     `json:"next_action,omitempty"`
}

// ReflectionConfig 反思配置
type ReflectionConfig struct {
	Enabled         bool
	MinConfidence   float64
	MaxReflections  int
	ReflectInterval int // 每隔多少次迭代进行一次反思
}

// DefaultReflectionConfig 默认反思配置
func DefaultReflectionConfig() *ReflectionConfig {
	return &ReflectionConfig{
		Enabled:         true,
		MinConfidence:   0.7,
		MaxReflections:  5,
		ReflectInterval: 3, // 每隔 3 次迭代进行一次反思
	}
}

// Reflector 任务反思器
type Reflector struct {
	config    *ReflectionConfig
	provider  providers.Provider
	workspace string
}

// NewReflector 创建任务反思器
func NewReflector(config *ReflectionConfig, provider providers.Provider, workspace string) *Reflector {
	if config == nil {
		config = DefaultReflectionConfig()
	}
	return &Reflector{
		config:    config,
		provider:  provider,
		workspace: workspace,
	}
}

// Reflect 对当前任务状态进行反思
func (r *Reflector) Reflect(ctx context.Context, userRequest string, conversation []session.Message) (*TaskReflection, error) {
	if !r.config.Enabled {
		return &TaskReflection{
			Status:     TaskStatusInProgress,
			Confidence: 0.5,
			Reasoning:  "Reflection disabled",
		}, nil
	}

	// 构建反思提示词
	prompt := r.buildReflectionPrompt(userRequest, conversation)

	messages := []providers.Message{
		{
			Role:    "system",
			Content: r.getReflectionSystemPrompt(),
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}

	logger.Info("Starting task reflection",
		zap.Int("conversation_length", len(conversation)),
		zap.String("user_request", userRequest))

	// 调用 LLM 进行反思
	response, err := r.provider.Chat(ctx, messages, nil)
	if err != nil {
		logger.Warn("Reflection failed, returning in_progress", zap.Error(err))
		return &TaskReflection{
			Status:     TaskStatusInProgress,
			Confidence: 0.3,
			Reasoning:  "Reflection failed due to error",
		}, nil
	}

	// 解析反思结果
	reflection, err := r.parseReflectionResponse(response.Content)
	if err != nil {
		logger.Warn("Failed to parse reflection", zap.Error(err))
		// 解析失败时，使用默认状态
		return &TaskReflection{
			Status:     TaskStatusInProgress,
			Confidence: 0.4,
			Reasoning:  "Reflection parsing failed",
		}, nil
	}

	logger.Info("Task reflection completed",
		zap.String("status", string(reflection.Status)),
		zap.Float64("confidence", reflection.Confidence),
		zap.Int("completed_steps", len(reflection.CompletedSteps)),
		zap.Int("remaining_steps", len(reflection.RemainingSteps)))

	return reflection, nil
}

// getReflectionSystemPrompt 获取反思的系统提示词
func (r *Reflector) getReflectionSystemPrompt() string {
	return `You are a task evaluation assistant. Your job is to review the conversation between a user and an AI agent to determine if the user's request has been completed.

## Your Task

Analyze the conversation and determine:
1. **Status**: Is the task completed, in progress, failed, or blocked?
2. **Confidence**: How confident are you? (0.0 to 1.0)
3. **Completed Steps**: What has been accomplished?
4. **Remaining Steps**: What still needs to be done?
5. **Reasoning**: Explain your assessment
6. **Next Action** (optional): What should be done next if not completed?

## Task Status Definitions

- **completed**: The user's request has been fully satisfied. All requirements are met.
- **in_progress**: The task is being worked on. Progress has been made but not finished.
- **failed**: The task cannot be completed due to a persistent error or impossible requirement.
- **blocked**: The task is blocked and requires user intervention to continue.

## Completion Criteria

Consider the task completed when:
- All explicit requirements from the user have been addressed
- The agent has provided the requested information, result, or confirmation
- No further action is needed from the agent
- The user would not need to follow up for the same request

## Response Format

Respond in JSON format:
{
  "status": "completed|in_progress|failed|blocked",
  "confidence": 0.0-1.0,
  "completed_steps": ["step1", "step2"],
  "remaining_steps": ["step1", "step2"],
  "reasoning": "explanation",
  "next_action": "suggested next step (optional)"
}

## Important Guidelines

- Be conservative: mark as "completed" only when truly done
- If uncertain, prefer "in_progress" over "completed"
- Focus on the user's original intent, not just the literal words
- Consider if the user might have follow-up questions
- Partial completion = in_progress, not completed`
}

// buildReflectionPrompt 构建反思提示词
func (r *Reflector) buildReflectionPrompt(userRequest string, conversation []session.Message) string {
	var sb strings.Builder

	sb.WriteString("## User's Original Request\n\n")
	sb.WriteString(userRequest)
	sb.WriteString("\n\n")

	sb.WriteString("## Conversation History\n\n")

	// 只显示最近的 20 条消息，避免 token 过多
	startIdx := len(conversation) - 20
	if startIdx < 0 {
		startIdx = 0
	}

	for i := startIdx; i < len(conversation); i++ {
		msg := conversation[i]
		sb.WriteString(fmt.Sprintf("**%s**: %s\n\n", msg.Role, msg.Content))
	}

	sb.WriteString("## Your Analysis\n\n")
	sb.WriteString("Please evaluate the task status based on the conversation above. ")
	sb.WriteString("Is the user's request completed? If not, what remains to be done?")

	return sb.String()
}

// parseReflectionResponse 解析反思响应
func (r *Reflector) parseReflectionResponse(content string) (*TaskReflection, error) {
	// 尝试提取 JSON 内容
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")

	if jsonStart == -1 || jsonEnd == -1 {
		return &TaskReflection{
			Status:     TaskStatusInProgress,
			Confidence: 0.4,
			Reasoning:  "No JSON found in response",
		}, nil
	}

	jsonStr := content[jsonStart : jsonEnd+1]

	// 简单解析 JSON（实际应该使用 encoding/json）
	reflection := &TaskReflection{
		Status:     TaskStatusInProgress,
		Confidence: 0.5,
	}

	// 提取状态
	if strings.Contains(jsonStr, `"status":"completed"`) {
		reflection.Status = TaskStatusCompleted
	} else if strings.Contains(jsonStr, `"status":"in_progress"`) {
		reflection.Status = TaskStatusInProgress
	} else if strings.Contains(jsonStr, `"status":"failed"`) {
		reflection.Status = TaskStatusFailed
	} else if strings.Contains(jsonStr, `"status":"blocked"`) {
		reflection.Status = TaskStatusBlocked
	}

	// 提取 confidence
	if idx := strings.Index(jsonStr, `"confidence":`); idx != -1 {
		// 简单提取逻辑
		end := strings.Index(jsonStr[idx:], ",")
		if end == -1 {
			end = strings.Index(jsonStr[idx:], "}")
		}
		if end > 0 {
			numStr := strings.TrimSpace(jsonStr[idx+13 : idx+end])
			_, _ = fmt.Sscanf(numStr, "%f", &reflection.Confidence)
		}
	}

	// 提取 reasoning
	if idx := strings.Index(jsonStr, `"reasoning":`); idx != -1 {
		start := idx + 12
		if jsonStr[start] == '"' {
			start++
			end := strings.Index(jsonStr[start:], `"`)
			if end > 0 {
				reflection.Reasoning = jsonStr[start : start+end]
			}
		}
	}

	// 提取 next_action
	if idx := strings.Index(jsonStr, `"next_action":`); idx != -1 {
		start := idx + 14
		if start < len(jsonStr) && jsonStr[start] == '"' {
			start++
			end := strings.Index(jsonStr[start:], `"`)
			if end > 0 {
				reflection.NextAction = jsonStr[start : start+end]
			}
		}
	}

	return reflection, nil
}

// ShouldContinueIteration 判断是否应该继续迭代
func (r *Reflector) ShouldContinueIteration(reflection *TaskReflection, currentIteration, maxIteration int) bool {
	// 已完成或失败，不再继续
	if reflection.Status == TaskStatusCompleted || reflection.Status == TaskStatusFailed {
		return false
	}

	// 被阻塞，不再继续
	if reflection.Status == TaskStatusBlocked {
		return false
	}

	// 仍在进行中，检查迭代次数
	if currentIteration >= maxIteration {
		logger.Warn("Max iterations reached",
			zap.Int("iteration", currentIteration),
			zap.Int("max", maxIteration))
		return false
	}

	// 如果已经执行了足够多次迭代（比如5次），就强制停止
	// 避免简单任务因为 reflection 过于保守而无限循环
	if currentIteration >= 5 && reflection.Status == TaskStatusInProgress {
		logger.Warn("Forcing completion after multiple in-progress reflections",
			zap.Int("iteration", currentIteration),
			zap.String("reasoning", reflection.Reasoning))
		return false
	}

	return true
}

// GenerateContinuePrompt 生成继续执行的提示词
func (r *Reflector) GenerateContinuePrompt(reflection *TaskReflection) string {
	if reflection == nil {
		return ""
	}

	if reflection.NextAction != "" {
		return fmt.Sprintf("Continue with the task. Based on analysis: %s\n\nNext step: %s",
			reflection.Reasoning, reflection.NextAction)
	}

	if len(reflection.RemainingSteps) > 0 {
		return fmt.Sprintf("Continue with the task. %s\n\nRemaining steps:\n%s",
			reflection.Reasoning, strings.Join(reflection.RemainingSteps, "\n- "))
	}

	return fmt.Sprintf("Continue with the task. %s", reflection.Reasoning)
}
