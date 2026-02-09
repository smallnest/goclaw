package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/dom"
	"github.com/mafredri/cdp/protocol/page"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"go.uber.org/zap"
)

// SmartSearch Smart search tool supporting web search and browser fallback
type SmartSearch struct {
	webTool    *WebTool
	timeout    time.Duration
	webEnabled bool
}

// NewSmartSearch Create smart search tool
func NewSmartSearch(webTool *WebTool, webEnabled bool, timeout int) *SmartSearch {
	var t time.Duration
	if timeout > 0 {
		t = time.Duration(timeout) * time.Second
	} else {
		t = 30 * time.Second
	}

	return &SmartSearch{
		webTool:    webTool,
		timeout:    t,
		webEnabled: webEnabled,
	}
}

// SmartSearchResult Smart search
func (s *SmartSearch) SmartSearchResult(ctx context.Context, params map[string]interface{}) (string, error) {
	query, ok := params["query"].(string)
	if !ok {
		return "Error: query parameter is required", nil
	}

	// Try web_search first
	if s.webEnabled {
		webResults, webErr := s.webTool.WebSearch(ctx, map[string]interface{}{"query": query})
		logger.Info("Web search returned",
			zap.String("query", query),
			zap.Int("result_length", len(webResults)),
			zap.Error(webErr),
			zap.String("result_preview", func() string {
				if len(webResults) > 200 {
					return webResults[:200] + "..."
				}
				return webResults
			}()))

		if webErr == nil && webResults != "" {
			// Check if warning message (no API key) or Mock result
			if !s.isWebSearchResultValid(webResults) {
				// web search unavailable, fallback to browser
				logger.Info("Web search result invalid, falling back to browser search",
					zap.String("reason", s.getInvalidReason(webResults)),
					zap.String("query", query))
				return s.fallbackToCrawl4AI(ctx, query)
			}
			// web search successful
			return webResults, nil
		} else {
			// web search failed, fallback to browser
			logger.Info("Web search failed, falling back to browser search",
				zap.String("query", query),
				zap.Error(webErr))
			return s.fallbackToCrawl4AI(ctx, query)
		}
	}

	// web search not enabled, use browser directly
	logger.Info("Web search not enabled, using browser search", zap.String("query", query))
	return s.fallbackToCrawl4AI(ctx, query)
}

// isWebSearchResultValid Check if web search result is valid
func (s *SmartSearch) isWebSearchResultValid(results string) bool {
	if results == "" {
		return false
	}

	// Check if warning message
	if strings.Contains(results, "[Warning:") {
		return false
	}

	// Check if Mock result
	if strings.Contains(results, "Mock") {
		return false
	}

	// Check if actual content (at least Title or URL)
	if strings.Contains(results, "Title:") || strings.Contains(results, "http") {
		return true
	}

	// Simple check: if result too short and no URL, maybe invalid
	if len(results) < 50 && !strings.Contains(results, "http") {
		return false
	}

	return true
}

// getInvalidReason Get result invalid reason (for debugging)
func (s *SmartSearch) getInvalidReason(results string) string {
	if results == "" {
		return "empty result"
	}
	if strings.Contains(results, "[Warning:") {
		return "contains warning"
	}
	if strings.Contains(results, "Mock") {
		return "contains mock"
	}
	if len(results) < 50 && !strings.Contains(results, "http") {
		return fmt.Sprintf("too short (%d chars)", len(results))
	}
	if !strings.Contains(results, "Title:") && !strings.Contains(results, "http") {
		return "no Title or URL found"
	}
	return "unknown"
}

// fallbackToCrawl4AI Use crawl4ai script to search Google
func (s *SmartSearch) fallbackToCrawl4AI(ctx context.Context, query string) (string, error) {
	logger.Info("Using crawl4ai for Google search", zap.String("query", query))

	// Find the script path
	scriptPath := s.findCrawl4AIScript()
	if scriptPath == "" {
		return s.fallbackToBrowser(ctx, query)
	}

	logger.Info("Found crawl4ai script", zap.String("path", scriptPath))

	// Build command
	var cmd *exec.Cmd
	pythonCmd := s.findPythonCommand()
	if pythonCmd == "" {
		return fmt.Sprintf("Google Search for: %s\n\nPython 3 is required but not found. Please install Python 3 to use crawl4ai search.", query), nil
	}

	logger.Info("Using Python", zap.String("command", pythonCmd))

	maxResults := 10
	cmdArgs := []string{scriptPath, query, fmt.Sprintf("%d", maxResults)}
	cmd = exec.CommandContext(ctx, pythonCmd, cmdArgs...)

	logger.Info("Executing crawl4ai script", zap.Strings("args", cmdArgs))

	// Set output to capture stdout
	output, err := cmd.CombinedOutput()

	logger.Info("Script output",
		zap.Int("output_length", len(output)),
		zap.Error(err))

	if err != nil {
		logger.Warn("Crawl4ai script failed",
			zap.Error(err),
			zap.String("output", string(output)))
		// Fallback to original browser method
		return s.fallbackToBrowser(ctx, query)
	}

	// Parse JSON output
	// The script outputs debug info, need to extract the JSON part
	// JSON is after "FULL JSON OUTPUT:" marker
	jsonOutput := string(output)
	if idx := strings.Index(jsonOutput, "FULL JSON OUTPUT:"); idx != -1 {
		// Find the JSON object after the marker
		jsonStart := strings.Index(jsonOutput[idx:], "{")
		if jsonStart != -1 {
			// Find the matching closing brace
			braceCount := 0
			inString := false
			jsonEnd := -1
			for i := idx + jsonStart; i < len(jsonOutput); i++ {
				c := jsonOutput[i]
				if c == '"' && (i == 0 || jsonOutput[i-1] != '\\') {
					inString = !inString
				} else if c == '{' && !inString {
					braceCount++
				} else if c == '}' && !inString {
					braceCount--
					if braceCount == 0 {
						jsonEnd = i + 1
						break
					}
				}
			}
			if jsonEnd != -1 {
				jsonOutput = jsonOutput[idx+jsonStart : jsonEnd]
			}
		}
	}

	var searchResult struct {
		Query        string `json:"query"`
		TotalResults int    `json:"total_results"`
		Results      []struct {
			Title       string `json:"title"`
			Link        string `json:"link"`
			Description string `json:"description"`
			SiteName    string `json:"site_name"`
		} `json:"results"`
	}

	if err := json.Unmarshal([]byte(jsonOutput), &searchResult); err != nil {
		logger.Warn("Failed to parse crawl4ai JSON output",
			zap.Error(err),
			zap.String("extracted_json", jsonOutput))
		// Fallback to original browser method
		return s.fallbackToBrowser(ctx, query)
	}

	// Check if we got results
	if len(searchResult.Results) == 0 {
		return fmt.Sprintf("Google Search for: %s\n\nNo results found. The query may not have matching results or Google blocked the request.", query), nil
	}

	// Format results
	var formattedResults []string
	for i, r := range searchResult.Results {
		formatted := fmt.Sprintf("%d. Title: %s\n   URL: %s", i+1, r.Title, r.Link)
		if r.Description != "" {
			formatted += fmt.Sprintf("\n   Description: %s", r.Description)
		}
		if r.SiteName != "" {
			formatted += fmt.Sprintf("\n   Site: %s", r.SiteName)
		}
		formattedResults = append(formattedResults, formatted)
	}

	logger.Info("Successfully extracted search results",
		zap.Int("result_count", len(formattedResults)))

	return fmt.Sprintf("Google Search Results for: %s\n\n%s",
		query,
		strings.Join(formattedResults, "\n\n---\n\n")), nil
}

// findCrawl4AIScript Find the crawl4ai google_search.py script
func (s *SmartSearch) findCrawl4AIScript() string {
	// Possible locations for the script
	possiblePaths := []string{
		"./skills/crawl4ai-skill/scripts/google_search.py",
		"../skills/crawl4ai-skill/scripts/google_search.py",
		"skills/crawl4ai-skill/scripts/google_search.py",
	}

	// Get current working directory
	cwd, err := exec.Command("pwd").Output()
	if err == nil {
		workingDir := strings.TrimSpace(string(cwd))
		possiblePaths = append(possiblePaths,
			filepath.Join(workingDir, "skills/crawl4ai-skill/scripts/google_search.py"),
			filepath.Join(workingDir, "..", "skills", "crawl4ai-skill", "scripts", "google_search.py"),
		)
	}

	for _, path := range possiblePaths {
		if _, err := exec.Command("test", "-f", path).CombinedOutput(); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}

	logger.Warn("crawl4ai script not found")
	return ""
}

// findPythonCommand Find python3 or python command
func (s *SmartSearch) findPythonCommand() string {
	// Try python3 first, then python
	commands := []string{"python3", "python"}

	for _, cmd := range commands {
		if _, err := exec.Command(cmd, "--version").CombinedOutput(); err == nil {
			return cmd
		}
	}

	return ""
}

// fallbackToBrowser Fallback to original browser search method (CDP)
func (s *SmartSearch) fallbackToBrowser(ctx context.Context, query string) (string, error) {
	logger.Info("Falling back to CDP browser search", zap.String("query", query))

	// Check if python is available for crawl4ai
	if s.findPythonCommand() == "" {
		return fmt.Sprintf("Google Search for: %s\n\n[⚠️ FALLBACK METHOD UNAVAILABLE]\n\nBoth crawl4ai (Python) and CDP (Chrome) methods require:\n- crawl4ai: Python 3\n- CDP: Chrome browser with remote debugging\n\nPlease install either:\n1. Python 3 with crawl4ai: pip install crawl4ai\n2. Chrome browser\n\nThen try again.", query), nil
	}

	// Get or create browser session
	sessionMgr := GetBrowserSession()
	if !sessionMgr.IsReady() {
		if err := sessionMgr.Start(s.timeout); err != nil {
			return fmt.Sprintf("Google Search for: %s\n\n[⚠️ CDP METHOD FAILED]\n\nFailed to start browser session: %v\n\nPlease ensure Chrome browser is installed and accessible.", query, err), nil
		}
	}

	// Get CDP client
	client, err := sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Google Search for: %s\n\n[⚠️ CDP METHOD FAILED]\n\nFailed to get browser client: %v", query, err), nil
	}

	// Build Google search URL
	googleURL := fmt.Sprintf("https://www.google.com/search?q=%s", urlEncode(query))

	logger.Info("Navigating to Google search", zap.String("url", googleURL))

	// Navigate to Google search
	nav, err := client.Page.Navigate(ctx, page.NewNavigateArgs(googleURL))
	if err != nil {
		return fmt.Sprintf("Google Search for: %s\n\n[⚠️ CDP METHOD FAILED]\n\nFailed to navigate: %v", query, err), nil
	}

	// Wait for page load
	domContentLoaded, err := client.Page.DOMContentEventFired(ctx)
	if err != nil {
		logger.Warn("DOMContentEventFired failed", zap.Error(err))
	} else {
		defer domContentLoaded.Close()
		_, _ = domContentLoaded.Recv()
	}

	// Get page content
	doc, err := client.DOM.GetDocument(ctx, nil)
	if err != nil {
		return fmt.Sprintf("Google Search for: %s\n\n[⚠️ CDP METHOD FAILED]\n\nFailed to get document: %v", query, err), nil
	}

	html, err := client.DOM.GetOuterHTML(ctx, &dom.GetOuterHTMLArgs{
		NodeID: &doc.Root.NodeID,
	})
	if err != nil {
		return fmt.Sprintf("Google Search for: %s\n\n[⚠️ CDP METHOD FAILED]\n\nFailed to get page content: %v", query, err), nil
	}

	content := html.OuterHTML

	logger.Info("Page content retrieved", zap.Int("content_length", len(content)), zap.String("frame_id", string(nav.FrameID)))

	// Check if blocked by Google (verify page)
	captchaKeywords := []string{
		"unusual traffic",
		"CAPTCHA",
		"verify you are human",
		"I'm not a robot",
		"若要继续",
		"系统检测到",
		"异常流量",
		"请启用 JavaScript",
	}

	blocked := false
	for _, keyword := range captchaKeywords {
		if strings.Contains(content, keyword) {
			blocked = true
			logger.Warn("Google detected automated traffic",
				zap.String("keyword", keyword),
				zap.Int("content_length", len(content)))
			break
		}
	}

	if blocked {
		return fmt.Sprintf("Google Search for: %s\n\n[⚠️ BLOCKED BY GOOGLE: CAPTCHA/Anti-bot verification required]\n\nGoogle has detected automated traffic from your IP address and requires human verification (CAPTCHA).\n\nPossible solutions:\n1. Wait 10-30 minutes and try again\n2. Try a different network (switch from VPN if using one)\n3. Use alternative search engine\n\nThe search page is showing a verification page instead of results.", query), nil
	}

	// Extract search results
	searchResults := s.extractGoogleSearchResults(content)

	if searchResults == "" {
		// Return partial content for debugging
		preview := content
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		return fmt.Sprintf("Google Search for: %s\n\n[⚠️ NO RESULTS EXTRACTED]\n\nNo results could be extracted from the page.\n\nPage preview:\n%s\n\nPossible reasons:\n- Google changed their HTML structure\n- CAPTCHA page was not detected\n- Empty search results\n\nTry:\n1. Waiting and retrying later\n2. Using a different search query", query, preview), nil
	}

	return fmt.Sprintf("Google Search Results for: %s\n\n%s", query, searchResults), nil
}

// extractGoogleSearchResults Extract search results from Google search page HTML
func (s *SmartSearch) extractGoogleSearchResults(pageText string) string {
	// Convert HTML to plain text
	text := htmlToTextForSearch(pageText)
	lines := strings.Split(text, "\n")

	var results []string
	var currentResult strings.Builder
	resultCount := 0

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip Google UI elements
		if s.isGoogleUIElement(line) {
			continue
		}

		// Detect possible title
		if s.isResultTitle(line) {
			// If existing result, save it
			if currentResult.Len() > 0 {
				result := currentResult.String()
				if s.isValidResult(result) {
					results = append(results, result)
					resultCount++
					if resultCount >= 10 {
						break
					}
				}
				currentResult.Reset()
			}
			currentResult.WriteString(fmt.Sprintf("Title: %s", line))
			continue
		}

		// If building result, add content
		if currentResult.Len() > 0 {
			if s.isURL(line) {
				currentResult.WriteString(fmt.Sprintf("\nURL: %s", line))
			} else if len(line) > 20 {
				currentResult.WriteString(fmt.Sprintf("\nDescription: %s", line))
			}
		}
	}

	// Add last result
	if currentResult.Len() > 0 {
		result := currentResult.String()
		if s.isValidResult(result) {
			results = append(results, result)
		}
	}

	if len(results) == 0 {
		// No valid results found
		if strings.Contains(text, "No results found") ||
			strings.Contains(text, "did not match any documents") ||
			strings.Contains(text, "Your search -") && strings.Contains(text, "- did not match") {
			return "No results found for this search query."
		}
		return ""
	}

	return strings.Join(results, "\n\n---\n\n")
}

// isGoogleUIElement Check if Google UI element
func (s *SmartSearch) isGoogleUIElement(line string) bool {
	uiElements := []string{
		"Google", "Search", "Images", "Maps", "News", "Videos",
		"Shopping", "More", "Sign in", "Settings", "Privacy",
		"Terms", "About", "Advertising", "Business", "Cookies",
		"All", "Tools", "SafeSearch",
		"Related searches", "People also ask", "Top stories",
		"Page", "of", "Next", "Previous",
	}

	lowerLine := strings.ToLower(line)
	for _, elem := range uiElements {
		if lowerLine == strings.ToLower(elem) {
			return true
		}
	}

	return false
}

// isResultTitle Check if search result title
func (s *SmartSearch) isResultTitle(line string) bool {
	// Title usually shorter (10-100 chars)
	if len(line) < 5 || len(line) > 120 {
		return false
	}

	// Skip pure URL
	if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
		return false
	}

	// Skip common suffixes
	excludeSuffixes := []string{"... more", "cached", "similar", "translate"}
	for _, suffix := range excludeSuffixes {
		if strings.HasSuffix(strings.ToLower(line), suffix) {
			return false
		}
	}

	// Check if contains meaningful characters
	hasContent := false
	for _, r := range line {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || (r >= 0x4e00 && r <= 0x9fff) {
			hasContent = true
			break
		}
	}

	return hasContent
}

// isURL Check if URL
func (s *SmartSearch) isURL(line string) bool {
	return strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://")
}

// isValidResult Check if result is valid
func (s *SmartSearch) isValidResult(result string) bool {
	// Must contain title
	if !strings.Contains(result, "Title:") {
		return false
	}

	// Must contain a valid URL (http:// or https://)
	if !strings.Contains(result, "URL:") {
		return false
	}

	// Extract URL part and validate
	urlPart := ""
	if idx := strings.Index(result, "URL:"); idx != -1 {
		remaining := result[idx+4:]
		// URL goes until next Description or end
		if descIdx := strings.Index(remaining, "\nDescription:"); descIdx != -1 {
			urlPart = strings.TrimSpace(remaining[:descIdx])
		} else {
			urlPart = strings.TrimSpace(remaining)
		}
	}

	// URL must be a valid format
	if urlPart == "" {
		return false
	}

	// Skip if URL doesn't start with http
	if !strings.HasPrefix(urlPart, "http://") && !strings.HasPrefix(urlPart, "https://") {
		return false
	}

	// Skip if URL is too short (invalid)
	if len(urlPart) < 10 {
		return false
	}

	// Skip obvious Google internal URLs
	if strings.Contains(urlPart, "google.com/search") ||
		strings.Contains(urlPart, "google.com/url") ||
		strings.Contains(urlPart, "google.com/preferences") {
		return false
	}

	return true
}

// GetTool Get smart search tool
func (s *SmartSearch) GetTool() Tool {
	return NewBaseTool(
		"smart_search",
		"Intelligent search that automatically falls back to Google browser search if web search fails. Uses crawl4ai (Python) for better anti-bot protection.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query to search for",
				},
			},
			"required": []string{"query"},
		},
		s.SmartSearchResult,
	)
}

// urlEncode URL encoding
func urlEncode(s string) string {
	var result strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			result.WriteRune(c)
		} else if c == ' ' {
			result.WriteString("+")
		} else {
			result.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return result.String()
}

// htmlToTextForSearch Convert HTML to plain text (for search result extraction)
func htmlToTextForSearch(html string) string {
	text := ""
	inTag := false
	inScript := false
	inStyle := false
	tagName := ""

	i := 0
	for i < len(html) {
		if html[i] == '<' {
			inTag = true
			tagName = ""
			j := i + 1
			for j < len(html) && html[j] != '>' && html[j] != ' ' {
				tagName += string(html[j])
				j++
			}
			if strings.ToLower(tagName) == "script" {
				inScript = true
			}
			if strings.ToLower(tagName) == "style" {
				inStyle = true
			}
			if strings.ToLower(tagName) == "/script" {
				inScript = false
			}
			if strings.ToLower(tagName) == "/style" {
				inStyle = false
			}
			i = j
			continue
		}

		if html[i] == '>' {
			inTag = false
			i++
			continue
		}

		if !inTag && !inScript && !inStyle {
			text += string(html[i])
		}

		i++
	}

	// Clean extra whitespace
	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}
