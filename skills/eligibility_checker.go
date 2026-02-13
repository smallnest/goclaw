package skills

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// DefaultEligibilityChecker implements the EligibilityChecker interface.
type DefaultEligibilityChecker struct{}

// NewEligibilityChecker creates a new eligibility checker.
func NewEligibilityChecker() EligibilityChecker {
	return &DefaultEligibilityChecker{}
}

// ShouldInclude determines if a skill should be included based on all eligibility checks.
func (c *DefaultEligibilityChecker) ShouldInclude(skill *SkillEntry, config SkillsConfig, ctx *SkillEligibilityContext) bool {
	// Check if skill is explicitly disabled in config
	if !skill.IsEnabled(config) {
		return false
	}

	// Check bundled skill allowlist
	if len(config.AllowBundled) > 0 && skill.Skill.Source == "bundled" {
		for _, allowed := range config.AllowBundled {
			if allowed == skill.Skill.Name {
				return true
			}
		}
		return false
	}

	// Check OS compatibility (blocking - if incompatible, skip entirely)
	if !c.CheckOSCompatibility(skill) {
		return false
	}

	// Check if skill is marked as always include (bypasses dependency checks)
	if skill.Metadata != nil && skill.Metadata.Always {
		return true
	}

	// For remote context, check remote eligibility
	if ctx != nil && ctx.Remote != nil {
		return c.checkRemoteEligibility(skill, ctx.Remote)
	}

	// Check binary availability
	missingBins := c.CheckBinaryAvailability(skill)
	if len(missingBins) > 0 {
		return false
	}

	// Check environment variables
	missingEnv := c.CheckEnvVariables(skill)
	if len(missingEnv) > 0 {
		return false
	}

	// Check config paths
	missingConfig := c.CheckConfigPaths(skill)
	return len(missingConfig) == 0
}

// CheckOSCompatibility checks if the skill is compatible with the current OS.
func (c *DefaultEligibilityChecker) CheckOSCompatibility(skill *SkillEntry) bool {
	if skill.Metadata == nil || len(skill.Metadata.OS) == 0 {
		return true // No OS restrictions
	}

	currentOS := runtime.GOOS
	for _, osName := range skill.Metadata.OS {
		if osName == currentOS {
			return true
		}
	}

	return false
}

// CheckBinaryAvailability checks which required binaries are missing.
func (c *DefaultEligibilityChecker) CheckBinaryAvailability(skill *SkillEntry) []string {
	var missing []string

	if skill.Metadata == nil || skill.Metadata.Requires == nil {
		return missing
	}

	// Check required binaries
	for _, bin := range skill.Metadata.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}

	// Check anyBins (at least one must be available)
	if len(skill.Metadata.Requires.AnyBins) > 0 {
		found := false
		for _, bin := range skill.Metadata.Requires.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, "any of: "+strings.Join(skill.Metadata.Requires.AnyBins, ", "))
		}
	}

	return missing
}

// CheckEnvVariables checks which required environment variables are missing.
func (c *DefaultEligibilityChecker) CheckEnvVariables(skill *SkillEntry) []string {
	var missing []string

	if skill.Metadata == nil || skill.Metadata.Requires == nil {
		return missing
	}

	for _, env := range skill.Metadata.Requires.Env {
		if os.Getenv(env) == "" {
			missing = append(missing, env)
		}
	}

	return missing
}

// CheckConfigPaths checks which required config paths are missing.
func (c *DefaultEligibilityChecker) CheckConfigPaths(skill *SkillEntry) []string {
	var missing []string

	if skill.Metadata == nil || skill.Metadata.Requires == nil {
		return missing
	}

	// For now, we'll check if config paths exist as files
	// In a real implementation, this would check against the actual config system
	for _, path := range skill.Metadata.Requires.Config {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			missing = append(missing, path)
		}
	}

	return missing
}

// checkRemoteEligibility checks skill eligibility in a remote context.
func (c *DefaultEligibilityChecker) checkRemoteEligibility(skill *SkillEntry, remote *RemoteContext) bool {
	// Check OS compatibility with remote platforms
	if skill.Metadata != nil && len(skill.Metadata.OS) > 0 {
		compatible := false
		for _, skillOS := range skill.Metadata.OS {
			for _, remoteOS := range remote.Platforms {
				if skillOS == remoteOS {
					compatible = true
					break
				}
			}
		}
		if !compatible {
			return false
		}
	}

	// Check binary availability in remote context
	if skill.Metadata != nil && skill.Metadata.Requires != nil {
		// Check required binaries
		for _, bin := range skill.Metadata.Requires.Bins {
			if remote.HasBin != nil && !remote.HasBin(bin) {
				return false
			}
		}

		// Check anyBins
		if len(skill.Metadata.Requires.AnyBins) > 0 {
			if remote.HasAnyBin != nil && !remote.HasAnyBin(skill.Metadata.Requires.AnyBins) {
				return false
			}
		}
	}

	return true
}

// CalculateMissingDependencies calculates all missing dependencies for a skill.
func (c *DefaultEligibilityChecker) CalculateMissingDependencies(skill *SkillEntry) *MissingDependencies {
	var missing MissingDependencies

	if skill.Metadata == nil || skill.Metadata.Requires == nil {
		return nil
	}

	// Check binary dependencies
	for _, bin := range skill.Metadata.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing.Bins = append(missing.Bins, bin)
		}
	}

	// Check anyBins (at least one must be available)
	if len(skill.Metadata.Requires.AnyBins) > 0 {
		found := false
		for _, bin := range skill.Metadata.Requires.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing.AnyBins = skill.Metadata.Requires.AnyBins
		}
	}

	// Check environment variables
	for _, env := range skill.Metadata.Requires.Env {
		if os.Getenv(env) == "" {
			missing.Env = append(missing.Env, env)
		}
	}

	// For now, we'll skip Python and Node.js package checking
	// This would require more sophisticated detection

	// Return nil if no dependencies are missing
	if len(missing.Bins) == 0 && len(missing.AnyBins) == 0 && len(missing.Env) == 0 {
		return nil
	}

	return &missing
}
