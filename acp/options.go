package acp

import (
	"fmt"
	"strings"
)

// SessionRuntimeOptions represents runtime options for an ACP session.
type SessionRuntimeOptions struct {
	RuntimeMode string `json:"runtime_mode"`
	Cwd         string `json:"cwd"`
}

// ValidateRuntimeOptionPatch validates a runtime option patch.
func ValidateRuntimeOptionPatch(patch map[string]any) (SessionRuntimeOptions, error) {
	options := SessionRuntimeOptions{}

	if cwd, ok := patch["cwd"].(string); ok {
		options.Cwd = cwd
	}

	if mode, ok := patch["runtime_mode"].(string); ok {
		options.RuntimeMode = mode
	}

	return options, nil
}

// NormalizeRuntimeOptions normalizes runtime options.
func NormalizeRuntimeOptions(options SessionRuntimeOptions) SessionRuntimeOptions {
	normalized := SessionRuntimeOptions{}

	if options.Cwd != "" {
		normalized.Cwd = strings.TrimSpace(options.Cwd)
	}

	if options.RuntimeMode != "" {
		normalized.RuntimeMode = strings.TrimSpace(options.RuntimeMode)
	}

	return normalized
}

// MergeRuntimeOptions merges a patch onto current options.
func MergeRuntimeOptions(current, patch SessionRuntimeOptions) SessionRuntimeOptions {
	merged := current

	if patch.Cwd != "" {
		merged.Cwd = patch.Cwd
	}

	if patch.RuntimeMode != "" {
		merged.RuntimeMode = patch.RuntimeMode
	}

	return NormalizeRuntimeOptions(merged)
}

// RuntimeOptionsEqual checks if two runtime options are equal.
func RuntimeOptionsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if bv, ok := b[k]; !ok || !optionValuesEqual(v, bv) {
			return false
		}
	}

	return true
}

// optionValuesEqual checks if two option values are equal.
func optionValuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle string types
	as, ok1 := a.(string)
	bs, ok2 := b.(string)
	if ok1 && ok2 {
		return as == bs
	}

	// Handle other types as string comparison
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// ResolveRuntimeOptionsFromMeta extracts runtime options from metadata.
func ResolveRuntimeOptionsFromMeta(meta map[string]any) map[string]any {
	options := make(map[string]any)

	if cwd, ok := meta["cwd"].(string); ok && cwd != "" {
		options["cwd"] = cwd
	}

	if runtimeMode, ok := meta["runtime_mode"].(string); ok && runtimeMode != "" {
		options["runtime_mode"] = runtimeMode
	}

	return options
}

// ValidateRuntimeModeInput validates a runtime mode input.
func ValidateRuntimeModeInput(mode string) string {
	return strings.TrimSpace(mode)
}

// ValidateRuntimeConfigOptionInput validates a config option input.
func ValidateRuntimeConfigOptionInput(key, value string) (normalizedKey, normalizedValue string) {
	return strings.TrimSpace(key), strings.TrimSpace(value)
}

// InferRuntimeOptionPatchFromConfigOption infers a runtime option patch from a config option.
func InferRuntimeOptionPatchFromConfigOption(key, value string) map[string]any {
	patch := make(map[string]any)

	switch strings.ToLower(key) {
	case "cwd", "working_directory", "working-dir":
		patch["cwd"] = value
	case "mode", "runtime_mode", "runtime-mode":
		patch["runtime_mode"] = value
	default:
		patch[key] = value
	}

	return patch
}
