package skills

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// installation timeout limits
const (
	MinInstallTimeout = 1 * time.Second
	MaxInstallTimeout = 15 * time.Minute
)

// InstallRequest represents a skill installation request
type InstallRequest struct {
	WorkspaceDir string
	SkillName    string
	InstallID    string
	Timeout      time.Duration
	Config       *SkillsConfig
}

// InstallResult represents the result of a skill installation
type InstallResult struct {
	Success   bool
	Message   string
	Stdout    string
	Stderr    string
	ExitCode  *int
	Warnings  []string
	Installed []string // Installed binaries
}

// Installer is the interface for installing skills
type Installer interface {
	// Install installs a skill using this installer
	Install(ctx context.Context, spec *SkillInstallSpec) InstallResult

	// CanInstall checks if this installer can be used on the current platform
	CanInstall(spec *SkillInstallSpec) bool
}

// WithWarnings adds warnings to an install result and returns a new result
func WithWarnings(result InstallResult, warnings []string) InstallResult {
	if len(warnings) == 0 {
		return result
	}
	// Copy the result and add warnings
	result.Warnings = make([]string, len(warnings))
	copy(result.Warnings, warnings)
	return result
}

// SummarizeInstallOutput extracts a concise summary from install output
func SummarizeInstallOutput(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	var nonEmptyLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	if len(nonEmptyLines) == 0 {
		return ""
	}

	// Look for error lines
	for _, line := range nonEmptyLines {
		if strings.HasPrefix(strings.ToLower(line), "error") ||
			strings.Contains(strings.ToLower(line), "err!") ||
			strings.Contains(strings.ToLower(line), "error:") ||
			strings.Contains(strings.ToLower(line), "failed") {
			return line
		}
	}

	// Return last line as summary
	lastLine := nonEmptyLines[len(nonEmptyLines)-1]
	maxLen := 200
	if len(lastLine) > maxLen {
		return lastLine[:maxLen-1] + "…"
	}
	return lastLine
}

// FormatInstallFailureMessage formats a failure message from a command result
func FormatInstallFailureMessage(stdout, stderr string, code *int) string {
	exitInfo := "unknown exit"
	if code != nil {
		exitInfo = fmt.Sprintf("exit %d", *code)
	}

	summary := SummarizeInstallOutput(stderr)
	if summary == "" {
		summary = SummarizeInstallOutput(stdout)
	}

	if summary == "" {
		return fmt.Sprintf("Install failed (%s)", exitInfo)
	}
	return fmt.Sprintf("Install failed (%s): %s", exitInfo, summary)
}

// FindInstallSpec finds an install spec by ID in a skill entry
func FindInstallSpec(entry *SkillEntry, installID string) *SkillInstallSpec {
	if entry.Metadata == nil || len(entry.Metadata.Install) == 0 {
		return nil
	}

	for index, spec := range entry.Metadata.Install {
		specID := spec.ID
		if specID == "" {
			specID = fmt.Sprintf("%s-%d", spec.Kind, index)
		}
		if specID == installID {
			return &spec
		}
	}
	return nil
}

// ResolveInstallPreferences resolves installation preferences from config
func ResolveInstallPreferences(config *SkillsConfig) InstallConfig {
	if config == nil {
		return InstallConfig{
			PreferBrew:  true,
			NodeManager: "npm",
		}
	}
	return config.Install
}

// GetInstaller returns the appropriate installer for a spec
func GetInstaller(spec *SkillInstallSpec, prefs InstallConfig) (Installer, error) {
	switch spec.Kind {
	case "brew":
		return &BrewInstaller{}, nil
	case "node":
		return &NodeInstaller{NodeManager: prefs.NodeManager}, nil
	case "go":
		return &GoInstaller{}, nil
	case "uv":
		return &UVInstaller{}, nil
	case "download":
		return &DownloadInstaller{}, nil
	default:
		return nil, fmt.Errorf("unsupported installer kind: %s", spec.Kind)
	}
}

// InstallSkill installs a skill using the specified install ID
func InstallSkill(ctx context.Context, req InstallRequest) (*InstallResult, error) {
	// Validate request
	if req.WorkspaceDir == "" {
		return nil, fmt.Errorf("workspace directory is required")
	}
	if req.SkillName == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	if req.InstallID == "" {
		return nil, fmt.Errorf("install ID is required")
	}

	// Resolve timeout
	timeout := req.Timeout
	if timeout == 0 {
		timeout = time.Duration(DefaultInstallTimeout) * time.Second
	}
	if timeout < MinInstallTimeout {
		timeout = MinInstallTimeout
	}
	if timeout > MaxInstallTimeout {
		timeout = MaxInstallTimeout
	}

	// Create context with timeout
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Load skill entries
	entries, err := LoadSkillEntries(req.WorkspaceDir, LoadSkillsOptions{
		IncludeDefaults: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load skills: %w", err)
	}

	// Find the skill entry
	var entry *SkillEntry
	for _, e := range entries {
		if e.Skill.Name == req.SkillName {
			entry = e
			break
		}
	}
	if entry == nil {
		return &InstallResult{
			Success: false,
			Message: fmt.Sprintf("Skill not found: %s", req.SkillName),
		}, nil
	}

	// Find install spec
	spec := FindInstallSpec(entry, req.InstallID)
	warnings := checkSkillSecurity(entry)

	if spec == nil {
		result := InstallResult{
			Success: false,
			Message: fmt.Sprintf("Installer not found: %s", req.InstallID),
		}
		res := WithWarnings(result, warnings)
		return &res, nil
	}

	// Get installer
	prefs := ResolveInstallPreferences(req.Config)
	installer, err := GetInstaller(spec, prefs)
	if err != nil {
		res := WithWarnings(InstallResult{
			Success: false,
			Message: err.Error(),
		}, warnings)
		return &res, nil
	}

	// Check if installer can be used
	if !installer.CanInstall(spec) {
		res := WithWarnings(InstallResult{
			Success: false,
			Message: fmt.Sprintf("Installer not available: %s", spec.Kind),
		}, warnings)
		return &res, nil
	}

	// Run installation
	logger.Info("Installing skill",
		zap.String("skill", req.SkillName),
		zap.String("installer", spec.Kind),
		zap.String("installID", req.InstallID),
	)

	result := installer.Install(ctx, spec)

	// Add any binaries that were installed
	if result.Success && len(spec.Bins) > 0 {
		result.Installed = detectInstalledBinaries(spec.Bins)
	}

	res := WithWarnings(result, warnings)
	return &res, nil
}

// RunCommandWithTimeout runs a command with timeout
func RunCommandWithTimeout(ctx context.Context, argv []string, env map[string]string) (stdout, stderr string, code *int, err error) {
	if len(argv) == 0 {
		return "", "", nil, fmt.Errorf("empty command")
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)

	// Add environment variables
	if env != nil {
		cmd.Env = append(os.Environ(), envSlice(env)...)
	}

	// Capture output
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Run command
	err = cmd.Run()

	// Get exit code
	var exitCode *int
	if cmd.ProcessState != nil {
		exitCodeVal := cmd.ProcessState.ExitCode()
		exitCode = &exitCodeVal
	}

	// Check if command failed
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return stdoutBuf.String(), stderrBuf.String(), exitCode,
				fmt.Errorf("command timed out after %v", deadlineExceeded(ctx))
		}
		return stdoutBuf.String(), stderrBuf.String(), exitCode,
			fmt.Errorf("command failed: %w", err)
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode, nil
}

// envSlice converts env map to slice of "key=value" strings
func envSlice(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

// deadlineExceeded extracts the remaining deadline from context
func deadlineExceeded(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		return time.Until(deadline)
	}
	return 0
}

// HasBinary checks if a binary is available on PATH
func HasBinary(bin string) bool {
	path, err := exec.LookPath(bin)
	return err == nil && path != ""
}

// checkSkillSecurity performs simple security checks on a skill
func checkSkillSecurity(entry *SkillEntry) []string {
	var warnings []string

	// Check if skill has dependencies that require binaries or environment variables
	if entry.Metadata != nil && entry.Metadata.Requires != nil {
		if len(entry.Metadata.Requires.Bins) > 0 || len(entry.Metadata.Requires.AnyBins) > 0 {
			// This requires binary dependencies - user should verify trust
			warnings = append(warnings,
				fmt.Sprintf("Skill \"%s\" requires binary dependencies. "+
					"Verify the skill source before installing.", entry.Skill.Name))
		}
		if len(entry.Metadata.Requires.Env) > 0 {
			warnings = append(warnings,
				fmt.Sprintf("Skill \"%s\" requires environment variables. "+
					"Check the skill documentation for required setup.", entry.Skill.Name))
		}
	}

	return warnings
}

// detectInstalledBinaries checks which of the specified binaries are now available
func detectInstalledBinaries(bins []string) []string {
	var installed []string
	for _, bin := range bins {
		if HasBinary(bin) {
			installed = append(installed, bin)
		}
	}
	return installed
}

// ResolveUserPath expands user home directory (~) in paths
func ResolveUserPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	// Expand environment variables
	return os.ExpandEnv(path)
}

// EnsureDir creates directory if it doesn't exist
func EnsureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
