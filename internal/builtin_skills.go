package internal

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed builtin_skills config.example.json
var builtinSkillsFS embed.FS

// GetHomeDir 获取用户主目录
func GetHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.Getenv("HOME")
	}
	return home
}

// GetGoclawDir 获取 goclaw 配置目录
func GetGoclawDir() string {
	return filepath.Join(GetHomeDir(), ".goclaw")
}

// GetConfigPath 获取配置文件路径
func GetConfigPath() string {
	return filepath.Join(GetGoclawDir(), "config.json")
}

// EnsureBuiltinSkills 确保内置技能被复制到用户目录
// 支持增量复制：只复制缺失的技能，不会覆盖已存在的技能
func EnsureBuiltinSkills() error {
	goclawDir := GetGoclawDir()
	skillsDir := filepath.Join(goclawDir, "skills")

	// 确保目录存在
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}

	// 获取嵌入的技能列表
	entries, err := builtinSkillsFS.ReadDir("builtin_skills")
	if err != nil {
		return fmt.Errorf("failed to read builtin_skills directory: %w", err)
	}

	// 检查每个内置技能，如果缺失则复制
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		dstDir := filepath.Join(skillsDir, skillName)

		// 如果技能目录不存在，则复制
		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			if err := copySingleSkill(skillName, dstDir); err != nil {
				// 记录错误但继续处理其他技能
				continue
			}
		}
	}

	return nil
}

// copySingleSkill 复制单个技能
func copySingleSkill(skillName, dstDir string) error {
	srcDir := filepath.Join("builtin_skills", skillName)

	// 创建目标目录
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dstDir, err)
	}

	// 递归复制目录内容
	return fs.WalkDir(builtinSkillsFS, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 获取相对路径
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// 目标路径
		dstPath := filepath.Join(dstDir, relPath)

		if d.IsDir() {
			// 跳过已经处理过的根目录
			if relPath == "." {
				return nil
			}
			// 创建目录
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dstPath, err)
			}
		} else {
			// 复制文件
			data, err := builtinSkillsFS.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path, err)
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", dstPath, err)
			}
		}

		return nil
	})
}

// EnsureConfig 确保配置文件存在
// 如果 config.json 不存在，则从 config.example.json 复制
// 返回是否是新创建的配置文件，以及可能的错误
func EnsureConfig() (bool, error) {
	configPath := GetConfigPath()

	// 检查配置文件是否已存在
	if _, err := os.Stat(configPath); err == nil {
		return false, nil
	}

	// 确保目录存在
	goclawDir := GetGoclawDir()
	if err := os.MkdirAll(goclawDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create goclaw directory: %w", err)
	}

	// 从嵌入的文件读取示例配置
	data, err := builtinSkillsFS.ReadFile("config.example.json")
	if err != nil {
		return false, fmt.Errorf("failed to read config.example.json: %w", err)
	}

	// 写入配置文件
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return false, fmt.Errorf("failed to write config.json: %w", err)
	}

	return true, nil
}

// copyBuiltinSkills 复制内置技能到目标目录
//
//nolint:unused
func copyBuiltinSkills(targetDir string) error {
	// 遍历 builtin_skills 目录
	entries, err := builtinSkillsFS.ReadDir("builtin_skills")
	if err != nil {
		return fmt.Errorf("failed to read builtin_skills directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		srcDir := filepath.Join("builtin_skills", skillName)
		dstDir := filepath.Join(targetDir, skillName)

		// 创建目标目录
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dstDir, err)
		}

		// 递归复制目录内容
		if err := fs.WalkDir(builtinSkillsFS, srcDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// 获取相对路径
			relPath, err := filepath.Rel(srcDir, path)
			if err != nil {
				return err
			}

			// 目标路径
			dstPath := filepath.Join(dstDir, relPath)

			if d.IsDir() {
				// 跳过已经处理过的根目录
				if relPath == "." {
					return nil
				}
				// 创建目录
				if err := os.MkdirAll(dstPath, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", dstPath, err)
				}
			} else {
				// 复制文件
				data, err := builtinSkillsFS.ReadFile(path)
				if err != nil {
					return fmt.Errorf("failed to read file %s: %w", path, err)
				}
				if err := os.WriteFile(dstPath, data, 0644); err != nil {
					return fmt.Errorf("failed to write file %s: %w", dstPath, err)
				}
			}

			return nil
		}); err != nil {
			return fmt.Errorf("failed to copy skill %s: %w", skillName, err)
		}
	}

	return nil
}
