package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// LogsCmd 日志查看命令
var LogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View goclaw logs",
	Long:  `View and follow goclaw application logs with color formatting and filtering.`,
	Run:   runLogs,
}

var (
	logsFollow  bool
	logsLimit   int
	logsPlain   bool
	logsJSON    bool
	logsNoColor bool
	logsFile    string
)

func init() {
	LogsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output (like tail -f)")
	LogsCmd.Flags().IntVarP(&logsLimit, "limit", "n", 100, "Number of lines to show")
	LogsCmd.Flags().BoolVar(&logsPlain, "plain", false, "Plain text output (no colors/formatting)")
	LogsCmd.Flags().BoolVar(&logsJSON, "json", false, "Output in JSON format (line-delimited)")
	LogsCmd.Flags().BoolVar(&logsNoColor, "no-color", false, "Disable colored output")
	LogsCmd.Flags().StringVarP(&logsFile, "file", "l", "", "Log file path (default: auto-detect)")
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Time    string                 `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"msg"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// runLogs 执行日志查看命令
func runLogs(cmd *cobra.Command, args []string) {
	// Determine log file path
	logPath := logsFile
	if logPath == "" {
		logPath = detectLogPath()
	}

	if logPath == "" {
		fmt.Fprintf(os.Stderr, "No log file found. Specify with --file or ensure logs are being written.\n")
		os.Exit(1)
	}

	// Check if file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Log file not found: %s\n", logPath)
		os.Exit(1)
	}

	// Configure colors
	// When --no-color, --plain, or --json is set, we'll avoid color functions
	// Color functions themselves check the condition before applying ANSI codes

	// Open log file
	file, err := os.Open(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	if logsFollow {
		followLogs(file)
	} else {
		viewLogs(file, logsLimit)
	}
}

// detectLogPath 自动检测日志文件路径
func detectLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Common log file locations
	candidates := []string{
		filepath.Join(home, ".goclaw", "logs", "goclaw.log"),
		filepath.Join(home, ".goclaw", "goclaw.log"),
		filepath.Join("/var", "log", "goclaw.log"),
		"goclaw.log",
		"logs/goclaw.log",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Return default path even if it doesn't exist
	return filepath.Join(home, ".goclaw", "logs", "goclaw.log")
}

// viewLogs 查看日志（不跟踪）
func viewLogs(file *os.File, limit int) {
	// Read all lines
	scanner := bufio.NewScanner(file)
	lines := make([]string, 0)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading log file: %v\n", err)
		os.Exit(1)
	}

	// Show last N lines
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}

	displayLines(lines[start:])
}

// followLogs 跟踪日志输出
func followLogs(file *os.File) {
	// Seek to end of file
	if _, err := file.Seek(0, 2); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to seek to end of file: %v\n", err)
		os.Exit(1)
	}

	// Setup signal handling for graceful exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create scanner for new lines
	scanner := bufio.NewScanner(file)

	fmt.Printf("Following log file (Ctrl+C to exit)...\n\n")

	for {
		select {
		case <-sigChan:
			fmt.Println("\nStopped following logs.")
			return
		default:
			if scanner.Scan() {
				line := scanner.Text()
				displayLine(line)
			} else {
				// Wait a bit before trying again
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

// displayLines 显示多行日志
func displayLines(lines []string) {
	for _, line := range lines {
		displayLine(line)
	}
}

// displayLine 显示单行日志
func displayLine(line string) {
	if logsPlain {
		fmt.Println(line)
		return
	}

	if logsJSON {
		// Try to parse as JSON and re-emit as line-delimited JSON
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			// Re-marshal to ensure consistent formatting
			if data, err := json.Marshal(entry); err == nil {
				fmt.Println(string(data))
				return
			}
		}
		// If not valid JSON, just output as-is
		fmt.Println(line)
		return
	}

	// Colored structured output
	displayColoredLine(line)
}

// displayColoredLine 显示带颜色的日志行
func displayColoredLine(line string) {
	// Try to parse as structured log
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		// Not JSON, output as-is
		fmt.Println(line)
		return
	}

	// Extract fields
	timestamp := getTimestamp(entry)
	level := getLevel(entry)
	msg := getMessage(entry)
	extraFields := getExtraFields(entry)

	// Build output
	var output strings.Builder

	// Timestamp
	if timestamp != "" {
		if logsNoColor || logsPlain {
			output.WriteString(fmt.Sprintf("[%s] ", timestamp))
		} else {
			output.WriteString(colorGray(fmt.Sprintf("[%s] ", timestamp)))
		}
	}

	// Level (colored by severity)
	levelStr := colorizeLevel(level)
	output.WriteString(levelStr)
	output.WriteString(" ")

	// Message
	if msg != "" {
		output.WriteString(msg)
	}

	// Extra fields
	if len(extraFields) > 0 {
		output.WriteString(" ")
		for k, v := range extraFields {
			if logsNoColor || logsPlain {
				output.WriteString(fmt.Sprintf("%s=%v ", k, v))
			} else {
				output.WriteString(colorCyan(fmt.Sprintf("%s=%v ", k, v)))
			}
		}
	}

	fmt.Println(output.String())
}

// ANSI color codes
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"

	ansiGray   = "\033[90m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiCyan   = "\033[36m"
	ansiWhite  = "\033[37m"

	ansiHighRed   = "\033[91m"
	ansiHighGreen = "\033[92m"
	ansiHighYellow = "\033[93m"
)

// Helper functions to apply colors
func applyColor(colorCode, text string) string {
	return colorCode + text + ansiReset
}

func colorGray(s string) string    { return applyColor(ansiGray, s) }
func colorRed(s string) string     { return applyColor(ansiRed, s) }
func colorGreen(s string) string   { return applyColor(ansiGreen, s) }
func colorYellow(s string) string  { return applyColor(ansiYellow, s) }
func colorBlue(s string) string    { return applyColor(ansiBlue, s) }
func colorCyan(s string) string    { return applyColor(ansiCyan, s) }
func colorWhite(s string) string   { return applyColor(ansiWhite, s) }
func colorHiRed(s string) string   { return applyColor(ansiHighRed, s) }
func colorHiGreen(s string) string { return applyColor(ansiHighGreen, s) }
func colorHiYellow(s string) string { return applyColor(ansiHighYellow, s) }

// getTimestamp 从日志条目获取时间戳
func getTimestamp(entry map[string]interface{}) string {
	// Try common timestamp keys
	for _, key := range []string{"T", "time", "timestamp", "@timestamp"} {
		if val, ok := entry[key]; ok {
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}

// getLevel 从日志条目获取日志级别
func getLevel(entry map[string]interface{}) string {
	// Try common level keys
	for _, key := range []string{"L", "level", "severity", "lvl"} {
		if val, ok := entry[key]; ok {
			return strings.ToUpper(fmt.Sprintf("%v", val))
		}
	}
	return "INFO"
}

// getMessage 从日志条目获取消息
func getMessage(entry map[string]interface{}) string {
	// Try common message keys
	for _, key := range []string{"M", "msg", "message", "text"} {
		if val, ok := entry[key]; ok {
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}

// getExtraFields 获取额外字段
func getExtraFields(entry map[string]interface{}) map[string]interface{} {
	exclude := map[string]bool{
		"T": true, "time": true, "timestamp": true, "@timestamp": true,
		"L": true, "level": true, "severity": true, "lvl": true,
		"M": true, "msg": true, "message": true, "text": true,
	}

	extra := make(map[string]interface{})
	for k, v := range entry {
		if !exclude[k] {
			extra[k] = v
		}
	}
	return extra
}

// colorizeLevel 为日志级别着色
func colorizeLevel(level string) string {
	formattedLevel := fmt.Sprintf("[%s]", level)
	if logsNoColor || logsPlain {
		return formattedLevel
	}

	switch level {
	case "DEBUG":
		return colorBlue(formattedLevel)
	case "INFO":
		return colorGreen(formattedLevel)
	case "WARN", "WARNING":
		return colorYellow(formattedLevel)
	case "ERROR":
		return colorRed(formattedLevel)
	case "FATAL", "PANIC":
		return colorHiRed(formattedLevel)
	default:
		return colorWhite(formattedLevel)
	}
}
