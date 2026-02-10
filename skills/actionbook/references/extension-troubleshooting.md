# Extension Troubleshooting

Common issues and fixes for the Chrome Extension bridge.

## Bridge not running

```bash
actionbook extension serve              # Start the bridge
```

Keep `serve` running in a separate terminal — it is a foreground process.

## Extension not responding

```bash
actionbook extension ping               # Check connectivity
```

If ping fails, verify the extension is loaded in Chrome (`chrome://extensions`) and enabled.

## Token expired (idle > 30 min)

Restart the bridge and re-pair in the extension popup:

```bash
actionbook extension serve              # Prints new token
```

Copy the token from output → paste in extension popup → Save.

## Stale port/token files

**Symptoms:** bridge running but extension connects to wrong port, or "WebSocket handshake failed" errors in bridge output.

**Cause:** previous `serve` process was killed ungracefully.

**Fix:** restart `serve` — it auto-cleans stale files on startup:

```bash
actionbook extension serve
```

## Extension not installed

```bash
actionbook extension install            # Install extension files
actionbook extension path               # Get directory for Chrome "Load unpacked"
```

Then load in Chrome: `chrome://extensions` → Developer mode → Load unpacked → select the path.

## Uninstall extension

```bash
actionbook extension uninstall          # Remove extension files and native host registration
```
