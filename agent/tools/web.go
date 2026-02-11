package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebTool Web 工具
type WebTool struct {
	searchAPIKey string
	searchEngine string
	timeout      time.Duration
	client       *http.Client
}

// NewWebTool 创建 Web 工具
func NewWebTool(searchAPIKey, searchEngine string, timeout int) *WebTool {
	var t time.Duration
	if timeout > 0 {
		t = time.Duration(timeout) * time.Second
	} else {
		t = 10 * time.Second
	}

	return &WebTool{
		searchAPIKey: searchAPIKey,
		searchEngine: searchEngine,
		timeout:      t,
		client: &http.Client{
			Timeout: t,
		},
	}
}

// WebSearch 网络搜索
func (t *WebTool) WebSearch(ctx context.Context, params map[string]interface{}) (string, error) {
	query, ok := params["query"].(string)
	if !ok {
		return "", fmt.Errorf("query parameter is required")
	}

	if t.searchAPIKey == "" {
		return fmt.Sprintf("Search results for: %s\n\n[Warning: Search API Key not configured. Please set search_api_key in config to get actual results.]", query), nil
	}

	// 默认使用 travily
	if t.searchEngine == "travily" || t.searchEngine == "tavily" || t.searchEngine == "" {
		return t.searchTavily(ctx, query)
	}

	if t.searchEngine == "serper" {
		return t.searchSerper(ctx, query)
	}

	if t.searchEngine == "google" {
		return t.searchGoogle(ctx, query)
	}

	return fmt.Sprintf("Search results for: %s\n\n[Warning: Search engine '%s' is not fully implemented. Using mock results.]", query, t.searchEngine), nil
}

func (t *WebTool) searchTavily(ctx context.Context, query string) (string, error) {
	apiURL := "https://api.tavily.com/search"
	maxResults := 5 // Default limit

	requestBody, err := json.Marshal(map[string]any{
		"query":          query,
		"search_depth":   "basic",
		"max_results":    maxResults,
		"include_images": true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.searchAPIKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform Tavily search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Tavily API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
		Images []string `json:"images"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode Tavily response: %w", err)
	}

	var sb bytes.Buffer
	for _, item := range result.Results {
		sb.WriteString(fmt.Sprintf("Title: %s\nURL: %s\nContent: %s\n\n", item.Title, item.URL, item.Content))
	}

	if len(result.Images) > 0 {
		sb.WriteString("\nRelevant Images:\n")
		for _, imgURL := range result.Images {
			sb.WriteString(fmt.Sprintf("- Image URL: %s\n", imgURL))
		}
		sb.WriteString("\n")
	}

	if sb.Len() == 0 {
		return "No results found.", nil
	}

	return sb.String(), nil
}

func (t *WebTool) searchGoogle(ctx context.Context, query string) (string, error) {
	// TODO: 实现 Google Custom Search API 调用
	if t.searchAPIKey == "" {
		return "", fmt.Errorf("google search api key is required")
	}
	return fmt.Sprintf("Google Search results for: %s\n\n1. Example Result (Mock)\n2. Another Result (Mock)", query), nil
}

func (t *WebTool) searchSerper(ctx context.Context, query string) (string, error) {
	url := "https://google.serper.dev/search"
	payload := strings.NewReader(fmt.Sprintf(`{"q": "%s"}`, query))

	req, err := http.NewRequestWithContext(ctx, "POST", url, payload)
	if err != nil {
		return "", err
	}

	req.Header.Add("X-API-KEY", t.searchAPIKey)
	req.Header.Add("Content-Type", "application/json")

	res, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search api returned status: %s", res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	// 这里简单返回 JSON，实际应该解析并格式化
	return string(body), nil
}

// WebFetch 抓取网页
func (t *WebTool) WebFetch(ctx context.Context, params map[string]interface{}) (string, error) {
	urlStr, ok := params["url"].(string)
	if !ok {
		return "", fmt.Errorf("url parameter is required")
	}

	// 验证 URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("only http and https URLs are supported")
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// 设置 User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; goclaw/1.0)")

	// 发送请求
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// 读取内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// 简化实现：返回原始 HTML
	// 实际应该使用 Readability 转换为 Markdown
	return t.htmlToMarkdown(string(body)), nil
}

// htmlToMarkdown 简单的 HTML 到 Markdown 转换
func (t *WebTool) htmlToMarkdown(html string) string {
	// 移除脚本和样式
	html = removeHTMLTags(html, "script")
	html = removeHTMLTags(html, "style")

	// 简单转换
	content := strings.TrimSpace(html)
	if len(content) > 10000 {
		content = content[:10000] + "\n\n... (truncated)"
	}

	return content
}

// removeHTMLTags 移除指定的 HTML 标签
func removeHTMLTags(html, tag string) string {
	// 简化实现
	startTag := "<" + tag
	endTag := "</" + tag + ">"

	result := html
	inTag := false
	var sb strings.Builder

	for i := 0; i < len(result); i++ {
		if i+len(startTag) <= len(result) && result[i:i+len(startTag)] == startTag {
			inTag = true
			i += len(startTag) - 1
			continue
		}

		if inTag && i+len(endTag) <= len(result) && result[i:i+len(endTag)] == endTag {
			inTag = false
			i += len(endTag) - 1
			continue
		}

		if !inTag {
			sb.WriteByte(result[i])
		}
	}

	return sb.String()
}

// GetTools 获取所有 Web 工具
func (t *WebTool) GetTools() []Tool {
	return []Tool{
		NewBaseTool(
			"web_search",
			"Search the web for information",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query",
					},
				},
				"required": []string{"query"},
			},
			t.WebSearch,
		),
		NewBaseTool(
			"web_fetch",
			"Fetch a web page and convert to markdown",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "URL to fetch",
					},
				},
				"required": []string{"url"},
			},
			t.WebFetch,
		),
	}
}
