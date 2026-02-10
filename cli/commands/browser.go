package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/protocol/dom"
	"github.com/mafredri/cdp/protocol/emulation"
	"github.com/mafredri/cdp/protocol/input"
	"github.com/mafredri/cdp/protocol/log"
	"github.com/mafredri/cdp/protocol/page"
	"github.com/mafredri/cdp/protocol/runtime"
	"github.com/mafredri/cdp/protocol/target"
	"github.com/smallnest/dogclaw/goclaw/agent/tools"
	"github.com/spf13/cobra"
)

// BrowserCommandRegistry Browser commands registry
type BrowserCommandRegistry struct {
	sessionMgr *tools.BrowserSessionManager
	homeDir    string
}

// NewBrowserCommandRegistry Create browser command registry
func NewBrowserCommandRegistry() *BrowserCommandRegistry {
	homeDir, _ := os.UserHomeDir()
	return &BrowserCommandRegistry{
		sessionMgr: tools.GetBrowserSession(),
		homeDir:    homeDir,
	}
}

// RegisterCommands Register browser commands with the command registry
func (r *BrowserCommandRegistry) RegisterCommands(registry *CommandRegistry) {
	// browser status - Show status
	registry.Register(&Command{
		Name:        "browser",
		Usage:       "/browser status",
		Description: "Show browser status",
		Handler:     r.browserStatus,
	})

	// browser start - Start browser
	registry.Register(&Command{
		Name:        "browser-start",
		Usage:       "/browser start",
		Description: "Start browser session",
		Handler:     r.browserStart,
	})

	// browser stop - Stop browser
	registry.Register(&Command{
		Name:        "browser-stop",
		Usage:       "/browser stop",
		Description: "Stop browser session",
		Handler:     r.browserStop,
	})

	// browser reset-profile - Reset profile
	registry.Register(&Command{
		Name:        "browser-reset-profile",
		Usage:       "/browser reset-profile",
		Description: "Reset browser profile",
		Handler:     r.browserResetProfile,
	})

	// browser tabs - List tabs
	registry.Register(&Command{
		Name:        "browser-tabs",
		Usage:       "/browser tabs",
		Description: "List browser tabs",
		Handler:     r.browserTabs,
	})

	// browser open - Open URL
	registry.Register(&Command{
		Name:  "browser-open",
		Usage: "/browser open <url>",
		Description: "Open URL in browser",
		ArgsSpec: []ArgSpec{
			{Name: "url", Description: "URL to open", Type: "enum", EnumValues: []string{"https://", "http://"}},
		},
		Handler: r.browserOpen,
	})

	// browser focus - Focus tab
	registry.Register(&Command{
		Name:  "browser-focus",
		Usage: "/browser focus <targetId>|--list",
		Description: "Focus browser tab or list all tabs",
		Handler: r.browserFocus,
	})

	// browser close - Close tab
	registry.Register(&Command{
		Name:  "browser-close",
		Usage: "/browser close [targetId]",
		Description: "Close browser tab (current if no ID specified)",
		Handler: r.browserClose,
	})

	// browser profiles - List profiles
	registry.Register(&Command{
		Name:        "browser-profiles",
		Usage:       "/browser profiles",
		Description: "List browser profiles",
		Handler:     r.browserProfiles,
	})

	// browser screenshot - Take screenshot
	registry.Register(&Command{
		Name:  "browser-screenshot",
		Usage: "/browser screenshot [targetId]",
		Description: "Take screenshot of current tab",
		Handler: r.browserScreenshot,
	})

	// browser snapshot - Take snapshot
	registry.Register(&Command{
		Name:        "browser-snapshot",
		Usage:       "/browser snapshot",
		Description: "Take page snapshot (HTML + screenshot)",
		Handler:     r.browserSnapshot,
	})

	// Browser Actions
	// browser navigate - Navigate to URL
	registry.Register(&Command{
		Name:  "browser-navigate",
		Usage: "/browser navigate <url>",
		Description: "Navigate to URL",
		Handler: r.browserNavigate,
	})

	// browser resize - Resize viewport
	registry.Register(&Command{
		Name:  "browser-resize",
		Usage: "/browser resize <width> <height>",
		Description: "Resize browser viewport",
		Handler: r.browserResize,
	})

	// browser click - Click element
	registry.Register(&Command{
		Name:  "browser-click",
		Usage: "/browser click <selector>",
		Description: "Click element using CSS selector",
		Handler: r.browserClick,
	})

	// browser type - Type text
	registry.Register(&Command{
		Name:  "browser-type",
		Usage: "/browser type <selector> <text>",
		Description: "Type text into element",
		Handler: r.browserType,
	})

	// browser press - Press key
	registry.Register(&Command{
		Name:  "browser-press",
		Usage: "/browser press <key>",
		Description: "Press keyboard key (Enter, Escape, etc.)",
		Handler: r.browserPress,
	})

	// browser hover - Hover element
	registry.Register(&Command{
		Name:  "browser-hover",
		Usage: "/browser hover <selector>",
		Description: "Hover over element",
		Handler: r.browserHover,
	})

	// browser select - Select option
	registry.Register(&Command{
		Name:  "browser-select",
		Usage: "/browser select <selector> <value>",
		Description: "Select option from dropdown",
		Handler: r.browserSelect,
	})

	// browser upload - Upload file
	registry.Register(&Command{
		Name:  "browser-upload",
		Usage: "/browser upload <selector> <filepath>",
		Description: "Upload file to input",
		Handler: r.browserUpload,
	})

	// browser fill - Fill form
	registry.Register(&Command{
		Name:  "browser-fill",
		Usage: "/browser fill <field> <value>",
		Description: "Fill form field",
		Handler: r.browserFill,
	})

	// browser dialog - Handle dialog
	registry.Register(&Command{
		Name:  "browser-dialog",
		Usage: "/browser dialog <accept|dismiss> [promptText]",
		Description: "Handle JavaScript dialog",
		Handler: r.browserDialog,
	})

	// browser wait - Wait for condition
	registry.Register(&Command{
		Name:  "browser-wait",
		Usage: "/browser wait <selector|timeout> [timeout]",
		Description: "Wait for element or timeout",
		Handler: r.browserWait,
	})

	// browser evaluate - Evaluate JavaScript
	registry.Register(&Command{
		Name:  "browser-evaluate",
		Usage: "/browser evaluate <javascript>",
		Description: "Evaluate JavaScript code",
		Handler: r.browserEvaluate,
	})

	// browser console - Get console logs
	registry.Register(&Command{
		Name:  "browser-console",
		Usage: "/browser console [--errors-only|--warnings-only|--info-only] [--max=N]",
		Description: "Get browser console logs with optional filters",
		Handler:     r.browserConsole,
	})

	// browser pdf - Save as PDF
	registry.Register(&Command{
		Name:  "browser-pdf",
		Usage: "/browser pdf [filename]",
		Description: "Save page as PDF",
		Handler: r.browserPDF,
	})
}

// browserStatus Show browser status
func (r *BrowserCommandRegistry) browserStatus(args []string) (string, bool) {
	if r.sessionMgr.IsReady() {
		return "Browser is running and ready", false
	}
	return "Browser is not running. Use '/browser start' to start", false
}

// browserStart Start browser
func (r *BrowserCommandRegistry) browserStart(args []string) (string, bool) {
	if r.sessionMgr.IsReady() {
		return "Browser is already running", false
	}

	timeout := 30 * time.Second
	if len(args) > 0 {
		if seconds, err := strconv.Atoi(args[0]); err == nil {
			timeout = time.Duration(seconds) * time.Second
		}
	}

	if err := r.sessionMgr.Start(timeout); err != nil {
		return fmt.Sprintf("Failed to start browser: %v", err), false
	}

	return "Browser started successfully", false
}

// browserStop Stop browser
func (r *BrowserCommandRegistry) browserStop(args []string) (string, bool) {
	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	r.sessionMgr.Stop()
	return "Browser stopped", false
}

// browserResetProfile Reset browser profile
func (r *BrowserCommandRegistry) browserResetProfile(args []string) (string, bool) {
	// Stop browser first
	if r.sessionMgr.IsReady() {
		r.sessionMgr.Stop()
	}

	// Clean up profile directory (if stored)
	profileDir := filepath.Join(r.homeDir, ".goclaw", "browser-profile")
	if _, err := os.Stat(profileDir); err == nil {
		os.RemoveAll(profileDir)
	}

	return "Browser profile reset. Restart with '/browser start'", false
}

// browserTabs List tabs
func (r *BrowserCommandRegistry) browserTabs(args []string) (string, bool) {
	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get current page info
	frameTree, err := client.Page.GetFrameTree(ctx)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	return fmt.Sprintf("Current tab:\n  URL: %s\n  Frame ID: %s",
		frameTree.FrameTree.Frame.URL, frameTree.FrameTree.Frame.ID), false
}

// browserOpen Open URL
func (r *BrowserCommandRegistry) browserOpen(args []string) (string, bool) {
	if len(args) == 0 {
		return "Usage: /browser open <url>", false
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running. Use '/browser start' first", false
	}

	url := args[0]
	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.Page.Navigate(ctx, page.NewNavigateArgs(url))
	if err != nil {
		return fmt.Sprintf("Failed to navigate: %v", err), false
	}

	// Wait for page load
	domContentLoaded, err := client.Page.DOMContentEventFired(ctx)
	if err == nil {
		defer domContentLoaded.Close()
		domContentLoaded.Recv()
	}

	return fmt.Sprintf("Opened: %s", url), false
}

// browserFocus Focus tab
func (r *BrowserCommandRegistry) browserFocus(args []string) (string, bool) {
	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if --list flag is provided
	if len(args) > 0 && args[0] == "--list" {
		// Get all targets using Target domain
		targets, err := client.Target.GetTargets(ctx)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), false
		}

		if len(targets.TargetInfos) == 0 {
			return "No tabs found", false
		}

		result := "Available tabs:\n"
		for i, t := range targets.TargetInfos {
			result += fmt.Sprintf("  [%d] ID: %s\n", i+1, t.TargetID)
			result += fmt.Sprintf("      URL: %s\n", t.URL)
			if t.Title != "" {
				result += fmt.Sprintf("      Title: %s\n", t.Title)
			}
			result += fmt.Sprintf("      Type: %s\n", t.Type)
		}
		return result, false
	}

	if len(args) == 0 {
		return "Usage: /browser focus <targetId> or /browser focus --list to list all tabs", false
	}

	targetID := target.ID(args[0])

	// Try to activate the target
	err = client.Target.ActivateTarget(ctx, target.NewActivateTargetArgs(targetID))
	if err != nil {
		return fmt.Sprintf("Failed to activate target: %v\n\nNote: Chrome DevTools Protocol has limitations with tab switching within a single connection. Each CDP connection is bound to one specific target. To work with multiple tabs, you would need to create separate CDP connections to each tab's WebSocket endpoint.\n\nTo list all available tabs, use: /browser focus --list", err), false
	}

	return fmt.Sprintf("Activated tab: %s", targetID), false
}

// browserClose Close tab
func (r *BrowserCommandRegistry) browserClose(args []string) (string, bool) {
	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Page.Close(ctx); err != nil {
		return fmt.Sprintf("Failed to close page: %v", err), false
	}

	return "Page closed. Browser session ended.", false
}

// browserProfiles List profiles
func (r *BrowserCommandRegistry) browserProfiles(args []string) (string, bool) {
	profileDir := filepath.Join(r.homeDir, ".goclaw", "browser-profile")

	entries, err := os.ReadDir(profileDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "No browser profiles found", false
		}
		return fmt.Sprintf("Error: %v", err), false
	}

	result := "Browser profiles:\n"
	for _, e := range entries {
		result += fmt.Sprintf("  - %s\n", e.Name())
	}

	return result, false
}

// browserScreenshot Take screenshot
func (r *BrowserCommandRegistry) browserScreenshot(args []string) (string, bool) {
	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	screenshotArgs := page.NewCaptureScreenshotArgs().SetFormat("png")
	screenshot, err := client.Page.CaptureScreenshot(ctx, screenshotArgs)
	if err != nil {
		return fmt.Sprintf("Failed to capture screenshot: %v", err), false
	}

	// Save screenshot
	screenshotDir := filepath.Join(r.homeDir, "goclaw-screenshots")
	os.MkdirAll(screenshotDir, 0755)

	filename := fmt.Sprintf("screenshot_%d.png", time.Now().Unix())
	screenshotPath := filepath.Join(screenshotDir, filename)

	if err := os.WriteFile(screenshotPath, screenshot.Data, 0644); err != nil {
		return fmt.Sprintf("Failed to save screenshot: %v", err), false
	}

	return fmt.Sprintf("Screenshot saved: %s", screenshotPath), false
}

// browserSnapshot Take page snapshot
func (r *BrowserCommandRegistry) browserSnapshot(args []string) (string, bool) {
	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get HTML
	doc, err := client.DOM.GetDocument(ctx, nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	html, err := client.DOM.GetOuterHTML(ctx, &dom.GetOuterHTMLArgs{
		NodeID: &doc.Root.NodeID,
	})
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	// Get screenshot
	screenshot, err := client.Page.CaptureScreenshot(ctx, page.NewCaptureScreenshotArgs().SetFormat("png"))
	if err != nil {
		return fmt.Sprintf("Failed to capture screenshot: %v", err), false
	}

	// Save snapshot
	snapshotDir := filepath.Join(r.homeDir, "goclaw-snapshots")
	os.MkdirAll(snapshotDir, 0755)

	timestamp := time.Now().Unix()
	htmlPath := filepath.Join(snapshotDir, fmt.Sprintf("snapshot_%d.html", timestamp))
	imgPath := filepath.Join(snapshotDir, fmt.Sprintf("snapshot_%d.png", timestamp))

	os.WriteFile(htmlPath, []byte(html.OuterHTML), 0644)
	os.WriteFile(imgPath, screenshot.Data, 0644)

	return fmt.Sprintf("Snapshot saved:\n  HTML: %s\n  Image: %s", htmlPath, imgPath), false
}

// browserNavigate Navigate to URL
func (r *BrowserCommandRegistry) browserNavigate(args []string) (string, bool) {
	return r.browserOpen(args)
}

// browserResize Resize viewport
func (r *BrowserCommandRegistry) browserResize(args []string) (string, bool) {
	if len(args) < 2 {
		return "Usage: /browser resize <width> <height>", false
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	width, err1 := strconv.Atoi(args[0])
	height, err2 := strconv.Atoi(args[1])

	if err1 != nil || err2 != nil {
		return "Invalid width or height", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = client.Emulation.SetDeviceMetricsOverride(ctx, emulation.NewSetDeviceMetricsOverrideArgs(
		width, height, 1.0, false,
	))
	if err != nil {
		return fmt.Sprintf("Failed to resize: %v", err), false
	}

	return fmt.Sprintf("Viewport resized to %dx%d", width, height), false
}

// browserClick Click element
func (r *BrowserCommandRegistry) browserClick(args []string) (string, bool) {
	if len(args) == 0 {
		return "Usage: /browser click <selector>", false
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	selector := args[0]
	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nodeID, err := r.querySelector(ctx, client, selector)
	if err != nil {
		return fmt.Sprintf("Failed to find element: %v", err), false
	}

	box, err := client.DOM.GetBoxModel(ctx, &dom.GetBoxModelArgs{
		NodeID: &nodeID,
	})
	if err != nil {
		return fmt.Sprintf("Failed to get element: %v", err), false
	}

	if len(box.Model.Content) < 8 {
		return "Invalid element box model", false
	}

	x := (box.Model.Content[0] + box.Model.Content[4]) / 2
	y := (box.Model.Content[1] + box.Model.Content[5]) / 2

	client.Input.DispatchMouseEvent(ctx, input.NewDispatchMouseEventArgs(
		"mousePressed", float64(x), float64(y)))
	client.Input.DispatchMouseEvent(ctx, input.NewDispatchMouseEventArgs(
		"mouseReleased", float64(x), float64(y)))

	return fmt.Sprintf("Clicked: %s", selector), false
}

// browserType Type text into element
func (r *BrowserCommandRegistry) browserType(args []string) (string, bool) {
	if len(args) < 2 {
		return "Usage: /browser type <selector> <text>", false
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	selector := args[0]
	text := args[1]

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nodeID, err := r.querySelector(ctx, client, selector)
	if err != nil {
		return fmt.Sprintf("Failed to find element: %v", err), false
	}

	client.DOM.Focus(ctx, &dom.FocusArgs{NodeID: &nodeID})

	// Type using JavaScript to set value and trigger events
	script := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (el) {
				var nativeInputValueSetter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
				nativeInputValueSetter.call(el, %q);
				el.dispatchEvent(new Event('input', { bubbles: true }));
				el.dispatchEvent(new Event('change', { bubbles: true }));
			}
		})()
	`, selector, text)

	_, err = client.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(script))
	if err != nil {
		return fmt.Sprintf("Failed to type text: %v", err), false
	}

	return fmt.Sprintf("Typed into: %s", selector), false
}

// browserPress Press keyboard key
func (r *BrowserCommandRegistry) browserPress(args []string) (string, bool) {
	if len(args) == 0 {
		return "Usage: /browser press <key> (Enter, Escape, Tab, etc.)", false
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	key := args[0]
	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Key mapping - use JavaScript for key press
	// Note: For simplicity, we use JavaScript to dispatch keyboard events
	script := fmt.Sprintf(`
		(function() {
			var event = new KeyboardEvent('keydown', {
				key: %q,
				code: %q,
				bubbles: true
			});
			document.activeElement.dispatchEvent(event);
		})()
	`, key, key)

	_, err = client.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(script))
	if err != nil {
		return fmt.Sprintf("Failed to press key: %v", err), false
	}

	return fmt.Sprintf("Pressed: %s", key), false
}

// browserHover Hover over element
func (r *BrowserCommandRegistry) browserHover(args []string) (string, bool) {
	if len(args) == 0 {
		return "Usage: /browser hover <selector>", false
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	selector := args[0]
	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nodeID, err := r.querySelector(ctx, client, selector)
	if err != nil {
		return fmt.Sprintf("Failed to find element: %v", err), false
	}

	box, err := client.DOM.GetBoxModel(ctx, &dom.GetBoxModelArgs{
		NodeID: &nodeID,
	})
	if err != nil {
		return fmt.Sprintf("Failed to get element: %v", err), false
	}

	if len(box.Model.Content) < 8 {
		return "Invalid element box model", false
	}

	x := (box.Model.Content[0] + box.Model.Content[4]) / 2
	y := (box.Model.Content[1] + box.Model.Content[5]) / 2

	client.Input.DispatchMouseEvent(ctx, input.NewDispatchMouseEventArgs(
		"mouseMoved", float64(x), float64(y)))

	return fmt.Sprintf("Hovered over: %s", selector), false
}

// browserSelect Select option from dropdown
func (r *BrowserCommandRegistry) browserSelect(args []string) (string, bool) {
	if len(args) < 2 {
		return "Usage: /browser select <selector> <value>", false
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	selector := args[0]
	value := args[1]

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	script := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (el) {
				el.value = %q;
				el.dispatchEvent(new Event('change', { bubbles: true }));
			}
		})()
	`, selector, value)

	_, err = client.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(script).SetReturnByValue(true))
	if err != nil {
		return fmt.Sprintf("Failed: %v", err), false
	}

	return fmt.Sprintf("Selected option in: %s", selector), false
}

// browserUpload Upload file
func (r *BrowserCommandRegistry) browserUpload(args []string) (string, bool) {
	if len(args) < 2 {
		return "Usage: /browser upload <selector> <filepath>", false
	}

	selector := args[0]
	filePath := args[1]

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Sprintf("File not found: %s", filePath), false
	}

	absPath, _ := filepath.Abs(filePath)

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Set file input - use JavaScript since SetFileInputFiles requires backend node ID
	script := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (el && el.type === 'file') {
				// Create a DataTransfer object with the file
				var dt = new DataTransfer();
				var file = new File([''], %q, { type: 'text/plain' });
				dt.items.add(file);
				el.files = dt.files;
				el.dispatchEvent(new Event('change', { bubbles: true }));
			}
		})()
	`, selector, absPath)

	_, err = client.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(script))
	if err != nil {
		return fmt.Sprintf("Failed: %v", err), false
	}

	return fmt.Sprintf("Uploaded file to: %s", selector), false
}

// browserFill Fill form field
func (r *BrowserCommandRegistry) browserFill(args []string) (string, bool) {
	return r.browserType(args)
}

// browserDialog Handle JavaScript dialog
func (r *BrowserCommandRegistry) browserDialog(args []string) (string, bool) {
	if len(args) == 0 {
		return "Usage: /browser dialog <accept|dismiss> [promptText]", false
	}

	action := args[0]
	promptText := ""
	if len(args) > 1 {
		promptText = args[1]
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if action == "accept" {
		err = client.Page.HandleJavaScriptDialog(ctx, page.NewHandleJavaScriptDialogArgs(true).SetPromptText(promptText))
	} else {
		err = client.Page.HandleJavaScriptDialog(ctx, page.NewHandleJavaScriptDialogArgs(false))
	}

	if err != nil {
		return fmt.Sprintf("Failed: %v", err), false
	}

	return fmt.Sprintf("Dialog %sed", action), false
}

// browserWait Wait for element or timeout
func (r *BrowserCommandRegistry) browserWait(args []string) (string, bool) {
	if len(args) == 0 {
		return "Usage: /browser wait <selector> [timeoutSeconds]", false
	}

	selector := args[0]
	timeout := 10 * time.Second
	if len(args) > 1 {
		if seconds, err := strconv.Atoi(args[1]); err == nil {
			timeout = time.Duration(seconds) * time.Second
		}
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Poll for element
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Sprintf("Timeout waiting for: %s", selector), false
		case <-ticker.C:
			doc, err := client.DOM.GetDocument(ctx, nil)
			if err != nil {
				continue
			}

			result, err := client.DOM.QuerySelector(ctx, &dom.QuerySelectorArgs{
				NodeID:   doc.Root.NodeID,
				Selector: selector,
			})
			if err == nil && result.NodeID != 0 {
				return fmt.Sprintf("Element found: %s", selector), false
			}
		}
	}
}

// browserEvaluate Evaluate JavaScript
func (r *BrowserCommandRegistry) browserEvaluate(args []string) (string, bool) {
	if len(args) == 0 {
		return "Usage: /browser evaluate <javascript>", false
	}

	script := args[0]

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(script).SetReturnByValue(true))
	if err != nil {
		return fmt.Sprintf("Failed: %v", err), false
	}

	if result.Result.Value != nil {
		return string(result.Result.Value), false
	}

	if result.Result.Description != nil {
		return *result.Result.Description, false
	}

	return "Executed (no return value)", false
}

// browserConsole Get console logs
func (r *BrowserCommandRegistry) browserConsole(args []string) (string, bool) {
	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse filter flags
	errorsOnly := false
	warningsOnly := false
	infoOnly := false
	maxEntries := 100

	for _, arg := range args {
		switch arg {
		case "--errors-only":
			errorsOnly = true
		case "--warnings-only":
			warningsOnly = true
		case "--info-only":
			infoOnly = true
		}
		if strings.HasPrefix(arg, "--max=") {
			if val, err := strconv.Atoi(strings.TrimPrefix(arg, "--max=")); err == nil && val > 0 {
				maxEntries = val
			}
		}
	}

	// Enable Log domain to capture console logs
	if err := client.Log.Enable(ctx); err != nil {
		return fmt.Sprintf("Failed to enable Log domain: %v", err), false
	}

	// Listen for log entries
	entryAdded, err := client.Log.EntryAdded(ctx)
	if err != nil {
		return fmt.Sprintf("Failed to listen for log entries: %v", err), false
	}
	defer entryAdded.Close()

	// Also listen to Runtime console API calls for more detailed info
	consoleCalled, err := client.Runtime.ConsoleAPICalled(ctx)
	if err != nil {
		return fmt.Sprintf("Failed to listen for console API: %v", err), false
	}
	defer consoleCalled.Close()

	// Collect entries with timeout
	logEntries := []log.Entry{}
	consoleEntries := []runtime.ConsoleAPICalledReply{}

	collectDone := make(chan struct{})
	go func() {
		// Collect log entries for a short period
		timeout := time.After(500 * time.Millisecond)
		for {
			select {
			case <-timeout:
				close(collectDone)
				return
			default:
				// Try to receive log entry without blocking
				entryReply, err := entryAdded.Recv()
				if err == nil && entryReply != nil {
					logEntries = append(logEntries, entryReply.Entry)
				}

				// Try to receive console API call
				consoleReply, err := consoleCalled.Recv()
				if err == nil && consoleReply != nil {
					consoleEntries = append(consoleEntries, *consoleReply)
				}

				// Check if we've collected enough
				if len(logEntries)+len(consoleEntries) >= maxEntries {
					close(collectDone)
					return
				}
			}
		}
	}()

	// Wait for collection to complete
	<-collectDone

	// Format and return results
	if len(logEntries) == 0 && len(consoleEntries) == 0 {
		return "No console logs found. Note: This shows logs since the command was executed. For historical logs, the browser may not retain all past console entries.", false
	}

	result := fmt.Sprintf("Console Logs (%d entries):\n\n", len(logEntries)+len(consoleEntries))

	// Process log entries
	for _, entry := range logEntries {
		// Apply filters
		if errorsOnly && entry.Level != "error" {
			continue
		}
		if warningsOnly && entry.Level != "warning" {
			continue
		}
		if infoOnly && entry.Level != "info" {
			continue
		}

		timestamp := time.Unix(0, int64(entry.Timestamp)*1e6).Format("15:04:05.000")
		result += fmt.Sprintf("[%s] %s %-8s: %s\n", timestamp, entry.Source, strings.ToUpper(entry.Level), entry.Text)

		if entry.URL != nil {
			result += fmt.Sprintf("    Source: %s", *entry.URL)
			if entry.LineNumber != nil {
				result += fmt.Sprintf(":%d", *entry.LineNumber)
			}
			result += "\n"
		}
	}

	// Process console API entries
	for _, entry := range consoleEntries {
		// Map console types to log levels
		level := "info"
		switch entry.Type {
		case "error", "assert":
			level = "error"
		case "warning":
			level = "warning"
		case "log", "debug", "info":
			level = "info"
		}

		// Apply filters
		if errorsOnly && level != "error" {
			continue
		}
		if warningsOnly && level != "warning" {
			continue
		}
		if infoOnly && level != "info" {
			continue
		}

		timestamp := time.Unix(0, int64(entry.Timestamp)*1e6).Format("15:04:05.000")
		result += fmt.Sprintf("[%s] console.%-8s: ", timestamp, entry.Type)

		// Format args
		for i, arg := range entry.Args {
			if i > 0 {
				result += " "
			}
			// Value is a RawMessage (JSON), need to extract string value
			if arg.Value != nil && len(arg.Value) > 0 {
				// Try to get the string representation
				valueStr := string(arg.Value)
				// Remove quotes if it's a JSON string
				if strings.HasPrefix(valueStr, "\"") && strings.HasSuffix(valueStr, "\"") {
					valueStr = valueStr[1 : len(valueStr)-1]
				}
				result += valueStr
			} else if arg.Description != nil {
				result += *arg.Description
			} else {
				result += fmt.Sprintf("<%s>", arg.Type)
			}
		}
		result += "\n"
	}

	result += "\nFilters: --errors-only, --warnings-only, --info-only, --max=N"

	return result, false
}

// browserPDF Save page as PDF
func (r *BrowserCommandRegistry) browserPDF(args []string) (string, bool) {
	filename := fmt.Sprintf("page_%d.pdf", time.Now().Unix())
	if len(args) > 0 {
		filename = args[0]
	}

	if !r.sessionMgr.IsReady() {
		return "Browser is not running", false
	}

	client, err := r.sessionMgr.GetClient()
	if err != nil {
		return fmt.Sprintf("Error: %v", err), false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pdfResult, err := client.Page.PrintToPDF(ctx, page.NewPrintToPDFArgs())
	if err != nil {
		return fmt.Sprintf("Failed to generate PDF: %v", err), false
	}

	// Save PDF
	screenshotDir := filepath.Join(r.homeDir, "goclaw-screenshots")
	os.MkdirAll(screenshotDir, 0755)

	pdfPath := filepath.Join(screenshotDir, filename)
	if err := os.WriteFile(pdfPath, pdfResult.Data, 0644); err != nil {
		return fmt.Sprintf("Failed to save PDF: %v", err), false
	}

	return fmt.Sprintf("PDF saved: %s (%d bytes)", pdfPath, len(pdfResult.Data)), false
}

// querySelector Find element using CSS selector
func (r *BrowserCommandRegistry) querySelector(ctx context.Context, client *cdp.Client, selector string) (dom.NodeID, error) {
	doc, err := client.DOM.GetDocument(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get document: %w", err)
	}

	result, err := client.DOM.QuerySelector(ctx, &dom.QuerySelectorArgs{
		NodeID:   doc.Root.NodeID,
		Selector: selector,
	})
	if err != nil {
		return 0, fmt.Errorf("query selector failed: %w", err)
	}

	if result.NodeID == 0 {
		return 0, fmt.Errorf("element not found: %s", selector)
	}

	return result.NodeID, nil
}

// GetCommandPrompts Get browser command prompts for help
func (r *BrowserCommandRegistry) GetCommandPrompts() string {
	return `Browser Commands:
  /browser status       - Show browser status
  /browser start        - Start browser
  /browser stop         - Stop browser
  /browser open <url>   - Open URL
  /browser focus <id>|--list - Focus tab or list tabs
  /browser console      - Get console logs [filters: --errors-only, --warnings-only, --info-only, --max=N]
  /browser screenshot   - Take screenshot
  /browser click <sel>  - Click element
  /browser type <sel> <text> - Type text
  /browser evaluate <js> - Evaluate JavaScript`
}

// ============================================
// Cobra CLI Commands for Browser
// ============================================

var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Browser control and automation",
	Long:  `Control Chrome/Chromium browser via CDP for automation tasks.`,
}

var browserStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show browser status",
	Run:   runBrowserStatus,
}

var browserStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start browser session",
	Run:   runBrowserStart,
}

var browserStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop browser session",
	Run:   runBrowserStop,
}

var browserResetProfileCmd = &cobra.Command{
	Use:   "reset-profile",
	Short: "Reset browser profile",
	Run:   runBrowserResetProfile,
}

var browserTabsCmd = &cobra.Command{
	Use:   "tabs",
	Short: "List browser tabs",
	Run:   runBrowserTabs,
}

var browserOpenCmd = &cobra.Command{
	Use:   "open <url>",
	Short: "Open URL in browser",
	Args:  cobra.ExactArgs(1),
	Run:   runBrowserOpen,
}

var browserFocusCmd = &cobra.Command{
	Use:   "focus <targetId>|--list",
	Short: "Focus browser tab or list all tabs",
	Args:  cobra.ExactArgs(1),
	Run:   runBrowserFocus,
}

var browserCloseCmd = &cobra.Command{
	Use:   "close [targetId]",
	Short: "Close browser tab",
	Args:  cobra.MaximumNArgs(1),
	Run:   runBrowserClose,
}

var browserProfilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List browser profiles",
	Run:   runBrowserProfiles,
}

var browserScreenshotCmd = &cobra.Command{
	Use:   "screenshot [targetId]",
	Short: "Take screenshot",
	Args:  cobra.MaximumNArgs(1),
	Run:   runBrowserScreenshot,
}

var browserSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Take page snapshot",
	Run:   runBrowserSnapshot,
}

var browserNavigateCmd = &cobra.Command{
	Use:   "navigate <url>",
	Short: "Navigate to URL",
	Args:  cobra.ExactArgs(1),
	Run:   runBrowserNavigate,
}

var browserResizeCmd = &cobra.Command{
	Use:   "resize <width> <height>",
	Short: "Resize viewport",
	Args:  cobra.ExactArgs(2),
	Run:   runBrowserResize,
}

var browserClickCmd = &cobra.Command{
	Use:   "click <selector>",
	Short: "Click element",
	Args:  cobra.ExactArgs(1),
	Run:   runBrowserClick,
}

var browserTypeCmd = &cobra.Command{
	Use:   "type <selector> <text>",
	Short: "Type text into element",
	Args:  cobra.ExactArgs(2),
	Run:   runBrowserType,
}

var browserPressCmd = &cobra.Command{
	Use:   "press <key>",
	Short: "Press keyboard key",
	Args:  cobra.ExactArgs(1),
	Run:   runBrowserPress,
}

var browserHoverCmd = &cobra.Command{
	Use:   "hover <selector>",
	Short: "Hover over element",
	Args:  cobra.ExactArgs(1),
	Run:   runBrowserHover,
}

var browserSelectCmd = &cobra.Command{
	Use:   "select <selector> <value>",
	Short: "Select option from dropdown",
	Args:  cobra.ExactArgs(2),
	Run:   runBrowserSelect,
}

var browserUploadCmd = &cobra.Command{
	Use:   "upload <selector> <filepath>",
	Short: "Upload file to input",
	Args:  cobra.ExactArgs(2),
	Run:   runBrowserUpload,
}

var browserFillCmd = &cobra.Command{
	Use:   "fill <field> <value>",
	Short: "Fill form field",
	Args:  cobra.ExactArgs(2),
	Run:   runBrowserFill,
}

var browserDialogCmd = &cobra.Command{
	Use:   "dialog <accept|dismiss> [promptText]",
	Short: "Handle JavaScript dialog",
	Args:  cobra.MinimumNArgs(1),
	Run:   runBrowserDialog,
}

var browserWaitCmd = &cobra.Command{
	Use:   "wait <selector> [timeout]",
	Short: "Wait for element or timeout",
	Args:  cobra.MinimumNArgs(1),
	Run:   runBrowserWait,
}

var browserEvaluateCmd = &cobra.Command{
	Use:   "evaluate <javascript>",
	Short: "Evaluate JavaScript",
	Args:  cobra.ExactArgs(1),
	Run:   runBrowserEvaluate,
}

var browserConsoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Get console logs with optional filters",
	Run:   runBrowserConsole,
}

var browserPdfCmd = &cobra.Command{
	Use:   "pdf [filename]",
	Short: "Save page as PDF",
	Args:  cobra.MaximumNArgs(1),
	Run:   runBrowserPDF,
}

func init() {
	// Add all subcommands to browser
	browserCmd.AddCommand(browserStatusCmd)
	browserCmd.AddCommand(browserStartCmd)
	browserCmd.AddCommand(browserStopCmd)
	browserCmd.AddCommand(browserResetProfileCmd)
	browserCmd.AddCommand(browserTabsCmd)
	browserCmd.AddCommand(browserOpenCmd)
	browserCmd.AddCommand(browserFocusCmd)
	browserCmd.AddCommand(browserCloseCmd)
	browserCmd.AddCommand(browserProfilesCmd)
	browserCmd.AddCommand(browserScreenshotCmd)
	browserCmd.AddCommand(browserSnapshotCmd)
	browserCmd.AddCommand(browserNavigateCmd)
	browserCmd.AddCommand(browserResizeCmd)
	browserCmd.AddCommand(browserClickCmd)
	browserCmd.AddCommand(browserTypeCmd)
	browserCmd.AddCommand(browserPressCmd)
	browserCmd.AddCommand(browserHoverCmd)
	browserCmd.AddCommand(browserSelectCmd)
	browserCmd.AddCommand(browserUploadCmd)
	browserCmd.AddCommand(browserFillCmd)
	browserCmd.AddCommand(browserDialogCmd)
	browserCmd.AddCommand(browserWaitCmd)
	browserCmd.AddCommand(browserEvaluateCmd)
	browserCmd.AddCommand(browserConsoleCmd)
	browserCmd.AddCommand(browserPdfCmd)
}

// BrowserCommand returns the browser cobra command
func BrowserCommand() *cobra.Command {
	return browserCmd
}

// Cobra command runners - delegate to BrowserCommandRegistry methods
func runBrowserStatus(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserStatus(args)
	fmt.Println(result)
}

func runBrowserStart(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserStart(args)
	fmt.Println(result)
}

func runBrowserStop(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserStop(args)
	fmt.Println(result)
}

func runBrowserResetProfile(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserResetProfile(args)
	fmt.Println(result)
}

func runBrowserTabs(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserTabs(args)
	fmt.Println(result)
}

func runBrowserOpen(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserOpen(args)
	fmt.Println(result)
}

func runBrowserFocus(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserFocus(args)
	fmt.Println(result)
}

func runBrowserClose(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserClose(args)
	fmt.Println(result)
}

func runBrowserProfiles(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserProfiles(args)
	fmt.Println(result)
}

func runBrowserScreenshot(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserScreenshot(args)
	fmt.Println(result)
}

func runBrowserSnapshot(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserSnapshot(args)
	fmt.Println(result)
}

func runBrowserNavigate(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserNavigate(args)
	fmt.Println(result)
}

func runBrowserResize(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserResize(args)
	fmt.Println(result)
}

func runBrowserClick(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserClick(args)
	fmt.Println(result)
}

func runBrowserType(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserType(args)
	fmt.Println(result)
}

func runBrowserPress(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserPress(args)
	fmt.Println(result)
}

func runBrowserHover(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserHover(args)
	fmt.Println(result)
}

func runBrowserSelect(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserSelect(args)
	fmt.Println(result)
}

func runBrowserUpload(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserUpload(args)
	fmt.Println(result)
}

func runBrowserFill(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserFill(args)
	fmt.Println(result)
}

func runBrowserDialog(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserDialog(args)
	fmt.Println(result)
}

func runBrowserWait(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserWait(args)
	fmt.Println(result)
}

func runBrowserEvaluate(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserEvaluate(args)
	fmt.Println(result)
}

func runBrowserConsole(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserConsole(args)
	fmt.Println(result)
}

func runBrowserPDF(cmd *cobra.Command, args []string) {
	registry := NewBrowserCommandRegistry()
	result, _ := registry.browserPDF(args)
	fmt.Println(result)
}
