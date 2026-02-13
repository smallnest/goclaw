package skills

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

// Constants for skill command formatting
const (
	SkillCommandMaxLength            = 32
	SkillCommandFallback             = "skill"
	SkillCommandDescriptionMaxLength = 100
)

// BuildWorkspaceSkillSnapshot creates a skill snapshot for an agent
func BuildWorkspaceSkillSnapshot(
	workspaceDir string,
	opts BuildSkillSnapshotOptions,
) (*SkillSnapshot, error) {
	// Load skill entries if not provided
	var entries []*SkillEntry
	if opts.Entries == nil {
		loadedEntries, err := LoadSkillEntries(workspaceDir, LoadSkillsOptions{
			ManagedSkillsDir: opts.ManagedSkillsDir,
			BundledSkillsDir: opts.BundledSkillsDir,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to load skill entries: %w", err)
		}
		entries = loadedEntries
	} else {
		entries = opts.Entries
	}

	// Filter eligible entries
	eligible, err := filterSkillEntries(entries, FilterSkillEntriesOptions{
		Config:      opts.ConfigMap,
		SkillConfig: opts.SkillsConfig,
		SkillFilter: opts.SkillFilter,
		Eligibility: opts.Eligibility,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to filter skill entries: %w", err)
	}

	// Extract resolved skills (all eligible skills)
	resolvedSkills := make([]*Skill, 0, len(eligible))
	for _, entry := range eligible {
		resolvedSkills = append(resolvedSkills, entry.Skill)
	}

	// Filter out skills with disable-model-invocation for LLM prompt
	promptEntries := FilterPromptEntries(eligible)

	// Generate prompt from prompt entries only
	// Build list of skills for prompt
	promptSkills := make([]*Skill, 0, len(promptEntries))
	for _, entry := range promptEntries {
		promptSkills = append(promptSkills, entry.Skill)
	}

	// Generate prompt
	prompt, err := FormatSkillsForPrompt(promptSkills)
	if err != nil {
		return nil, fmt.Errorf("failed to format skills for prompt: %w", err)
	}

	// Add remote note if provided
	if opts.Eligibility != nil && opts.Eligibility.Remote != nil && opts.Eligibility.Remote.Note != "" {
		prompt = fmt.Sprintf("%s\n%s", opts.Eligibility.Remote.Note, prompt)
	}

	// Create skills summary
	skills := make([]SkillSummary, 0, len(eligible))
	for _, entry := range eligible {
		primaryEnv := ""
		if entry.Metadata != nil {
			primaryEnv = entry.Metadata.PrimaryEnv
		}
		skills = append(skills, SkillSummary{
			Name:       entry.Skill.Name,
			PrimaryEnv: primaryEnv,
		})
	}

	return &SkillSnapshot{
		Prompt:         prompt,
		Skills:         skills,
		ResolvedSkills: resolvedSkills,
		Version:        opts.SnapshotVersion,
	}, nil
}

// BuildSkillSnapshotOptions configures snapshot building
type BuildSkillSnapshotOptions struct {
	ConfigMap        map[string]interface{} // Allow raw config for compatibility
	SkillsConfig     *SkillsConfig          // Use typed config for easier access
	ManagedSkillsDir string
	BundledSkillsDir string
	Entries          []*SkillEntry
	SkillFilter      []string
	Eligibility      *SkillEligibilityContext
	SnapshotVersion  int
}

// FilterSkillEntriesOptions configures skill entry filtering
type FilterSkillEntriesOptions struct {
	Config      map[string]interface{} // Allow raw config for compatibility
	SkillConfig *SkillsConfig          // Use typed config when available
	SkillFilter []string
	Eligibility *SkillEligibilityContext
}

// filterSkillEntries filters skill entries based on eligibility criteria
func filterSkillEntries(
	entries []*SkillEntry,
	opts FilterSkillEntriesOptions,
) ([]*SkillEntry, error) {
	var filtered []*SkillEntry

	for _, entry := range entries {
		// Check if skill should be included
		if shouldIncludeSkill(entry, opts) {
			// Apply skill filter if provided
			if len(opts.SkillFilter) > 0 {
				found := false
				for _, filterName := range opts.SkillFilter {
					if filterName == entry.Skill.Name {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			filtered = append(filtered, entry)
		}
	}

	return filtered, nil
}

// shouldIncludeSkill determines if a skill should be included based on various criteria
func shouldIncludeSkill(entry *SkillEntry, opts FilterSkillEntriesOptions) bool {
	// Check if skill is explicitly disabled in config
	if opts.SkillConfig != nil {
		if len(opts.SkillConfig.Disabled) > 0 {
			if disabled, ok := opts.SkillConfig.Disabled[entry.Skill.Name]; ok && disabled {
				return false
			}
		}

		// Check bundled skills allowlist
		if entry.Skill.Source == "bundled" && len(opts.SkillConfig.AllowBundled) > 0 {
			allowed := false
			for _, allowedSkill := range opts.SkillConfig.AllowBundled {
				if allowedSkill == entry.Skill.Name {
					allowed = true
					break
				}
			}
			if !allowed {
				return false
			}
		}
	}

	// Check OS compatibility
	if entry.Metadata != nil && len(entry.Metadata.OS) > 0 {
		currentOS := runtime.GOOS
		compatible := false
		for _, osName := range entry.Metadata.OS {
			if osName == currentOS {
				compatible = true
				break
			}
		}
		if !compatible {
			return false
		}
	}

	// Always include if marked as always
	if entry.Metadata != nil && entry.Metadata.Always {
		return true
	}

	// Check binary requirements
	if entry.Metadata != nil && entry.Metadata.Requires != nil && len(entry.Metadata.Requires.Bins) > 0 {
		for _, bin := range entry.Metadata.Requires.Bins {
			if !hasBinary(bin, opts) {
				return false
			}
		}
	}

	// Check any-binary requirements
	if entry.Metadata != nil && entry.Metadata.Requires != nil && len(entry.Metadata.Requires.AnyBins) > 0 {
		foundAny := false
		for _, bin := range entry.Metadata.Requires.AnyBins {
			if hasBinary(bin, opts) {
				foundAny = true
				break
			}
		}
		if !foundAny {
			return false
		}
	}

	// Check environment variable requirements
	if entry.Metadata != nil && entry.Metadata.Requires != nil && len(entry.Metadata.Requires.Env) > 0 {
		for _, envVar := range entry.Metadata.Requires.Env {
			if os.Getenv(envVar) == "" {
				return false
			}
		}
	}

	// Check config requirements
	if entry.Metadata != nil && entry.Metadata.Requires != nil && len(entry.Metadata.Requires.Config) > 0 {
		for _, configPath := range entry.Metadata.Requires.Config {
			if !hasConfigValue(configPath, opts) {
				return false
			}
		}
	}

	return true
}

// hasBinary checks if a binary is available
func hasBinary(bin string, opts FilterSkillEntriesOptions) bool {
	if opts.Eligibility != nil && opts.Eligibility.Remote != nil {
		return opts.Eligibility.Remote.HasBin(bin)
	}

	// Check local binary availability
	path, err := exec.LookPath(bin)
	return err == nil && path != ""
}

// hasConfigValue checks if a config value is present
func hasConfigValue(configPath string, opts FilterSkillEntriesOptions) bool {
	// TODO: Implement config path resolution
	return false
}

// FormatSkillsForPrompt formats skills as XML for AI context
func FormatSkillsForPrompt(skills []*Skill) (string, error) {
	if len(skills) == 0 {
		return "", nil
	}

	var lines []string
	lines = append(lines,
		"",
		"The following skills provide specialized instructions for specific tasks.",
		"Use the read tool to load a skill's file when the task matches its description.",
		"When a skill file references a relative path, resolve it against the skill directory",
		"(parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.",
		"",
		"<available_skills>",
	)

	// Sort skills by name for consistent output
	sortedSkills := make([]*Skill, len(skills))
	copy(sortedSkills, skills)
	sort.Slice(sortedSkills, func(i, j int) bool {
		return sortedSkills[i].Name < sortedSkills[j].Name
	})

	for _, skill := range sortedSkills {
		lines = append(lines,
			"  <skill>",
			fmt.Sprintf("    <name>%s</name>", escapeXML(skill.Name)),
			fmt.Sprintf("    <description>%s</description>", escapeXML(skill.Description)),
			fmt.Sprintf("    <location>%s</location>", escapeXML(skill.FilePath)),
			"  </skill>",
		)
	}

	lines = append(lines, "</available_skills>")

	return strings.Join(lines, "\n"), nil
}

// escapeXML escapes special XML characters
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// BuildSkillCommandSpecs generates command specifications
func BuildSkillCommandSpecs(
	workspaceDir string,
	opts BuildCommandSpecsOptions,
) ([]*SkillCommandSpec, error) {
	// Load skill entries if not provided
	var entries []*SkillEntry
	if opts.Entries == nil {
		loadedEntries, err := LoadSkillEntries(workspaceDir, LoadSkillsOptions{
			ManagedSkillsDir: opts.ManagedSkillsDir,
			BundledSkillsDir: opts.BundledSkillsDir,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to load skill entries: %w", err)
		}
		entries = loadedEntries
	} else {
		entries = opts.Entries
	}

	// Filter eligible entries
	eligible, err := filterSkillEntries(entries, FilterSkillEntriesOptions{
		Config:      opts.ConfigMap,
		SkillConfig: opts.SkillsConfig,
		SkillFilter: opts.SkillFilter,
		Eligibility: opts.Eligibility,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to filter skill entries: %w", err)
	}

	// Filter user-invocable skills
	var userInvocable []*SkillEntry
	for _, entry := range eligible {
		if entry.InvocationPolicy == nil || entry.InvocationPolicy.UserInvocable {
			userInvocable = append(userInvocable, entry)
		}
	}

	// Build command specs
	usedNames := make(map[string]bool)
	for _, reserved := range opts.ReservedNames {
		usedNames[strings.ToLower(reserved)] = true
	}

	var specs []*SkillCommandSpec
	for _, entry := range userInvocable {
		rawName := entry.Skill.Name
		baseName := sanitizeSkillCommandName(rawName)
		uniqueName := resolveUniqueSkillCommandName(baseName, usedNames)

		usedNames[strings.ToLower(uniqueName)] = true

		// Build description
		description := entry.Skill.Description
		if strings.TrimSpace(description) == "" {
			description = rawName
		}
		if len(description) > SkillCommandDescriptionMaxLength {
			description = description[:SkillCommandDescriptionMaxLength-1] + "…"
		}

		// Build dispatch specification
		dispatch := buildSkillCommandDispatch(entry, uniqueName)

		specs = append(specs, &SkillCommandSpec{
			Name:        uniqueName,
			SkillName:   rawName,
			Description: description,
			Dispatch:    dispatch,
		})
	}

	return specs, nil
}

// BuildCommandSpecsOptions configures command specification building
type BuildCommandSpecsOptions struct {
	ConfigMap        map[string]interface{}
	SkillsConfig     *SkillsConfig
	ManagedSkillsDir string
	BundledSkillsDir string
	Entries          []*SkillEntry
	SkillFilter      []string
	Eligibility      *SkillEligibilityContext
	ReservedNames    []string
}

// sanitizeSkillCommandName sanitizes a skill name for use as a command name
func sanitizeSkillCommandName(raw string) string {
	normalized := strings.ToLower(raw)

	// Replace spaces and special characters (except dashes) with underscores
	nonAlphaNumDash := regexp.MustCompile(`[^a-z0-9-]+`)
	normalized = nonAlphaNumDash.ReplaceAllString(normalized, "_")

	// Trim leading/trailing underscores
	normalized = strings.Trim(normalized, "_")

	if len(normalized) > SkillCommandMaxLength {
		normalized = normalized[:SkillCommandMaxLength]
	}

	if normalized == "" {
		return SkillCommandFallback
	}
	return normalized
}

// resolveUniqueSkillCommandName ensures a command name is unique
func resolveUniqueSkillCommandName(base string, used map[string]bool) string {
	baseLower := strings.ToLower(base)
	if !used[baseLower] {
		return base
	}

	for i := 2; i < 1000; i++ {
		suffix := fmt.Sprintf("_%d", i)
		maxBaseLength := max(1, SkillCommandMaxLength-len(suffix))
		candidate := base[:min(len(base), maxBaseLength)] + suffix
		candidateLower := strings.ToLower(candidate)

		if !used[candidateLower] {
			return candidate
		}
	}

	fallback := base[:min(len(base), SkillCommandMaxLength-2)] + "_x"
	return fallback
}

// buildSkillCommandDispatch builds the dispatch specification for a skill command
func buildSkillCommandDispatch(entry *SkillEntry, commandName string) *SkillCommandDispatch {
	// Look for command-dispatch in frontmatter
	kindRaw := strings.TrimSpace(strings.ToLower(entry.Frontmatter["command-dispatch"]))
	if kindRaw == "" {
		kindRaw = strings.TrimSpace(strings.ToLower(entry.Frontmatter["command_dispatch"]))
	}

	if kindRaw != "tool" {
		return nil
	}

	// Get tool name
	toolName := strings.TrimSpace(entry.Frontmatter["command-tool"])
	if toolName == "" {
		toolName = strings.TrimSpace(entry.Frontmatter["command_tool"])
	}

	if toolName == "" {
		logger.Debug("Skill command requested tool dispatch but no tool name provided",
			zap.String("skill", entry.Skill.Name),
			zap.String("command", commandName),
		)
		return nil
	}

	// Get argument mode
	argModeRaw := strings.TrimSpace(strings.ToLower(entry.Frontmatter["command-arg-mode"]))
	if argModeRaw == "" {
		argModeRaw = strings.TrimSpace(strings.ToLower(entry.Frontmatter["command_arg_mode"]))
	}

	argMode := "raw"
	if argModeRaw != "" && argModeRaw != "raw" {
		logger.Debug("Skill command has unknown argument mode, defaulting to raw",
			zap.String("skill", entry.Skill.Name),
			zap.String("command", commandName),
			zap.String("argMode", argModeRaw),
		)
	}

	return &SkillCommandDispatch{
		Kind:     "tool",
		ToolName: toolName,
		ArgMode:  argMode,
	}
}

// FilterPromptEntries filters out skills that should not be included in the LLM prompt
func FilterPromptEntries(entries []*SkillEntry) []*SkillEntry {
	var filtered []*SkillEntry
	for _, entry := range entries {
		if entry.InvocationPolicy == nil || !entry.InvocationPolicy.DisableModelInvocation {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
