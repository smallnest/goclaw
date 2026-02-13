package skills

import (
	"context"
	"fmt"
	"strings"
)

// NodeInstaller implements Node.js package installations
type NodeInstaller struct {
	// NodeManager is the package manager to use (npm, pnpm, yarn, or bun)
	NodeManager string
}

var (
	// Supported node managers
	nodeManagers = []string{"npm", "pnpm", "yarn", "bun"}
)

// CanInstall checks if the configured node manager is available
func (n *NodeInstaller) CanInstall(spec *SkillInstallSpec) bool {
	if spec.Kind != "node" {
		return false
	}
	manager := n.resolveManager()
	return manager != "" && spec.Package != ""
}

// Install installs a package using the configured node manager
func (n *NodeInstaller) Install(ctx context.Context, spec *SkillInstallSpec) InstallResult {
	manager := n.resolveManager()
	if manager == "" {
		return InstallResult{
			Success: false,
			Message: "node manager not available (install npm, pnpm, yarn, or bun)",
		}
	}

	// Validate package
	if spec.Package == "" {
		return InstallResult{
			Success: false,
			Message: "missing node package",
		}
	}

	// Check if already installed
	if !spec.Extract {
		isInstalled := n.isInstalled(ctx, manager, spec.Package)
		if isInstalled {
			return InstallResult{
				Success: true,
				Message: fmt.Sprintf("Already installed: %s", spec.Package),
			}
		}
	}

	// Build command
	argv := n.buildCommand(spec.Package)

	// Run installation
	stdout, stderr, exitCode, _ := RunCommandWithTimeout(ctx, argv, nil)

	success := exitCode != nil && *exitCode == 0
	var message string
	if success {
		message = fmt.Sprintf("Installed: %s (%s)", spec.Package, manager)
	} else {
		message = FormatInstallFailureMessage(stdout, stderr, exitCode)
	}

	return InstallResult{
		Success:  success,
		Message:  message,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
	}
}

// resolveManager returns the best available node manager
func (n *NodeInstaller) resolveManager() string {
	// Use configured manager if available
	if n.NodeManager != "" && HasBinary(n.NodeManager) {
		return n.NodeManager
	}

	// Try preferred managers in order
	for _, manager := range nodeManagers {
		if HasBinary(manager) {
			return manager
		}
	}

	return ""
}

// buildCommand builds the install command for a package
func (n *NodeInstaller) buildCommand(pkg string) []string {
	manager := n.resolveManager()

	switch manager {
	case "pnpm":
		return []string{"pnpm", "add", "-g", pkg}
	case "yarn":
		return []string{"yarn", "global", "add", pkg}
	case "bun":
		return []string{"bun", "add", "-g", pkg}
	default: // npm
		return []string{"npm", "install", "-g", pkg}
	}
}

// isInstalled checks if a package is installed globally
func (n *NodeInstaller) isInstalled(ctx context.Context, _, pkg string) bool {
	manager := n.resolveManager()

	var argv []string
	switch manager {
	case "pnpm":
		argv = []string{manager, "list", "-g", "--depth=0"}
	case "yarn":
		argv = []string{manager, "global", "list"}
	case "bun":
		argv = []string{manager, "pm", "ls", "-g"}
	default: // npm
		argv = []string{manager, "list", "-g", "--depth=0"}
	}

	stdout, _, _, _ := RunCommandWithTimeout(ctx, argv, nil)

	// Check if package appears in the list
	output := strings.ToLower(stdout)
	// For global packages, npm lists them as "package@version"
	// or just "package" depending on the npm version and flags
	search := strings.ToLower(pkg)
	return strings.Contains(output, search+"@") || strings.Contains(output, " "+search+" ") ||
		strings.Contains(output, search+"\n")
}
