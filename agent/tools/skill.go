package tools

import (
	"context"
	"encoding/json"
)

// useSkillResult 表示使用技能的结果
type useSkillResult struct {
	Success      bool   `json:"success"`
	SkillName    string `json:"skill_name"`
	SkillContent string `json:"skill_content,omitempty"`
	Message      string `json:"message"`
}

// NewUseSkillTool 创建使用技能的工具
// 这个工具用于让 LLM 选择要使用的技能，然后触发第二阶段的完整内容加载
func NewUseSkillTool() *BaseTool {
	return NewBaseTool(
		"use_skill",
		"Select a skill to use for the current task. This loads the full skill content into the context.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"skill_name": map[string]interface{}{
					"type":        "string",
					"description": "The name of the skill to use",
				},
			},
			"required": []string{"skill_name"},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			skillName, ok := params["skill_name"].(string)
			if !ok || skillName == "" {
				result := useSkillResult{
					Success: false,
					Message: "skill_name parameter is required",
				}
				data, _ := json.Marshal(result)
				return string(data), nil
			}

			// 返回结果，让 loop.go 处理第二阶段
			result := useSkillResult{
				Success:   true,
				SkillName: skillName,
				Message:   "Skill selected. The full skill content will be loaded.",
			}
			data, _ := json.Marshal(result)
			return string(data), nil
		},
	)
}
