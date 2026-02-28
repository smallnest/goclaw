package commands

import (
	"fmt"
	"strings"
	"unicode"
)

func renderResponse(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	return renderSimpleMarkdown(raw)
}

func renderSimpleMarkdown(raw string) string {
	// 先归一化缩进级别，确保所有列表项都没有前导空格
	raw = normalizeIndentLevel(raw)

	var builder strings.Builder
	inCodeBlock := false

	lines := strings.Split(raw, "\n")
	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			builder.WriteString(trimmed)
			builder.WriteString("\n")
			continue
		}

		if inCodeBlock || isIndentedCode(line) {
			builder.WriteString(line)
			builder.WriteString("\n")
			continue
		}

		if trimmed == "" {
			builder.WriteString("\n")
			continue
		}

		if heading := formatHeading(trimmed); heading != "" {
			builder.WriteString(heading)
			builder.WriteString("\n")
			continue
		}

		if content, level, ok := parseBullet(line); ok {
			builder.WriteString(strings.Repeat("  ", level))
			builder.WriteString("* ")
			builder.WriteString(content)
			builder.WriteString("\n")
			continue
		}

		if content, level, ok := parseOrdered(line); ok {
			builder.WriteString(strings.Repeat("  ", level))
			builder.WriteString(content)
			builder.WriteString("\n")
			continue
		}

		if blockquote := parseBlockquote(trimmed); blockquote != "" {
			builder.WriteString(blockquote)
			builder.WriteString("\n")
			continue
		}

		builder.WriteString(trimmed)
		builder.WriteString("\n")
	}

	return strings.TrimRight(builder.String(), "\n")
}

func isIndentedCode(line string) bool {
	spaceCount := 0
	for _, ch := range line {
		switch ch {
		case ' ':
			spaceCount++
			if spaceCount >= 4 {
				return true
			}
		case '\t':
			return true
		default:
			return false
		}
	}
	return false
}

func formatHeading(line string) string {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) {
		return ""
	}
	content := strings.TrimSpace(line[level:])
	if content == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", strings.Repeat("#", level), content)
}

func parseBullet(line string) (string, int, bool) {
	level := indentLevel(line)
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", 0, false
	}
	runes := []rune(trimmed)
	prefix := runes[0]
	if prefix != '-' && prefix != '*' && prefix != '+' && prefix != '•' {
		return "", 0, false
	}
	content := strings.TrimSpace(string(runes[1:]))
	return content, level, true
}

func parseOrdered(line string) (string, int, bool) {
	level := indentLevel(line)
	trimmed := strings.TrimSpace(line)
	dot := strings.Index(trimmed, ". ")
	if dot <= 0 {
		return "", 0, false
	}
	number := trimmed[:dot]
	for _, ch := range number {
		if !unicode.IsDigit(ch) {
			return "", 0, false
		}
	}
	content := strings.TrimSpace(trimmed[dot+2:])
	return fmt.Sprintf("%s. %s", number, content), level, true
}

func parseBlockquote(line string) string {
	if !strings.HasPrefix(line, ">") {
		return ""
	}
	content := strings.TrimSpace(strings.TrimPrefix(line, ">"))
	return fmt.Sprintf("> %s", content)
}

func indentLevel(line string) int {
	count := 0
	for _, ch := range line {
		switch ch {
		case ' ':
			count++
		case '\t':
			count += 4
		default:
			return count / 2
		}
	}
	return count / 2
}

// maxIndentLevel 检测所有列表项中的最大缩进级别
// 用于确保缩进一致性
func normalizeIndentLevel(raw string) string {
	lines := strings.Split(raw, "\n")
	var maxLevel int
	var levels []int

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			levels = append(levels, -1)
			continue
		}
		runes := []rune(trimmed)
		if len(runes) == 0 {
			levels = append(levels, -1)
			continue
		}
		prefix := runes[0]
		if prefix == '-' || prefix == '*' || prefix == '+' || prefix == '•' {
			level := indentLevel(line)
			levels = append(levels, level)
			if level > maxLevel {
				maxLevel = level
			}
		} else {
			levels = append(levels, -1)
		}
	}

	// 如果所有列表项的缩进级别都是 0，直接返回原字符串
	if maxLevel == 0 {
		return raw
	}

	// 否则，移除所有列表项的前导空格，使缩进级别归一化
	var result strings.Builder
	for i, line := range lines {
		if levels[i] < 0 {
			result.WriteString(line)
		} else {
			// 移除前导空格
			trimmed := strings.TrimLeft(line, " \t")
			result.WriteString(trimmed)
		}
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}
	return result.String()
}
