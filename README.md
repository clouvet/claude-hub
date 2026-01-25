# Claude Hub

WebSocket hub for multi-client Claude Code session synchronization. Enables seamless switching between sprite-mobile web UI and `claude --resume` terminal sessions.

## Architecture

```
Web Browsers → sprite-mobile:8081 (WebSocket proxy)
                    ↓
              claude-hub:9090 (this service)
                    ↓
         Claude processes + ~/.claude/projects/ session files
```

## Features

### ✅ Phase 1: Core Hub + Basic WebSocket
- WebSocket server on port 9090
- Multi-client broadcasting per session
- Client registration/unregistration
- Connection pooling

### ✅ Phase 2: Claude Process Management
- Spawns headless Claude processes (`claude --print --output-format stream-json`)
- Routes user messages to Claude via stdin
- Parses stream-json protocol responses
- Broadcasts Claude output to all connected clients in real-time
- Interrupt support (kill process mid-generation)
- Auto-spawn on first client connection

### ✅ Phase 3: File Watching + Terminal Detection
- fsnotify watcher for Claude session files (`~/.claude/projects/-home-sprite/*.jsonl`)
- Detects terminal Claude processes via /proc scanning
- State machine: `IDLE → WEB_ONLY ⇄ TERMINAL_ONLY`
- Auto-kills headless process when terminal session detected
- Tails .jsonl files and broadcasts terminal messages to web clients
- Auto-respawns headless when terminal exits

### ✅ Phase 5: Service Integration
- Service YAML configuration (`~/.sprite/services/claude-hub.yaml`)
- Startup script with env sourcing
- Graceful shutdown (SIGTERM/SIGINT handling)
- Health check endpoint (`/health`)
- Structured logging

### 🚧 Phase 4: Message Queue + Interrupts (basic version working)
- Basic sequential processing via headless process
- Interrupt support (works, could be enhanced)
- Future: Persistent queue for crash recovery

## Installation

### On a New Sprite

The recommended way to install claude-hub is via the sprite setup script:

```bash
cd /home/sprite
./sprite-setup.sh 12
```

This will:
- Clone/update the claude-hub repository to `~/.claude-hub/`
- Build the Go binary
- Create the service configuration
- Start the claude-hub service on port 9090

### Manual Installation

```bash
# Clone
git clone https://github.com/clouvet/claude-hub.git ~/.claude-hub
cd ~/.claude-hub

# Build
go build -o bin/claude-hub main.go

# Create service configuration
mkdir -p ~/.sprite/services
cat > ~/.sprite/services/claude-hub.yaml << EOF
name: claude-hub
port: 9090
command: $HOME/.claude-hub/scripts/start-service.sh
workdir: $HOME/.claude-hub
description: WebSocket hub for multi-client Claude Code session synchronization
EOF

# Start service
sprite-env services start claude-hub
```

The hub listens on port 9090 (internal only - accessed via sprite-mobile proxy).

## Usage with sprite-mobile

Enable the WebSocket proxy in sprite-mobile:

```bash
cd ~/.sprite-mobile
USE_GO_HUB=true bun server.ts
```

Then:
1. Open sprite-mobile in browser
2. Start a session and send messages (works via Go hub)
3. Run `claude --resume <claude-session-uuid>` in terminal
4. Hub auto-detects terminal, kills headless, starts watching file
5. Messages from terminal appear in web UI in real-time
6. Exit terminal - hub respawns headless process

## Key Components

### internal/hub/
- `hub.go` - WebSocket hub (broadcast, routing)
- `client.go` - Client connection management

### internal/process/
- `manager.go` - Process lifecycle management
- `headless.go` - Claude process wrapper
- `detector.go` - Terminal process detection

### internal/session/
- `session.go` - Session state tracking
- `state.go` - State machine (IDLE, WEB_ONLY, TERMINAL_ONLY, TRANSITIONING)

### internal/watcher/
- `watcher.go` - fsnotify file watching
- `parser.go` - JSONL parsing and content extraction

### pkg/claude/
- `protocol.go` - stream-json protocol types

## Configuration

Environment variables:
- `PORT` - WebSocket port (default: 9090)
- `HOME` - User home directory for session file paths

## Testing

**Start the hub:**
```bash
cd ~/.claude-hub
./bin/claude-hub
```

**Test multi-client sync:**
1. Open multiple browser tabs to same session
2. Send messages from any tab
3. Verify all tabs see messages and responses

**Test terminal sync:**
1. Start session in web UI
2. Get Claude session UUID from logs
3. Run `claude --resume <uuid>` in terminal
4. Send messages in terminal
5. Verify web UI shows terminal messages in real-time
6. Exit terminal
7. Verify web UI returns to web mode

## Development

```bash
# Build
go build -o bin/claude-hub main.go

# Run with logs
./bin/claude-hub 2>&1 | tee hub.log

# Check for terminal processes
ps aux | grep claude

# Monitor session files
watch -n 1 'ls -lh ~/.claude/projects/-home-sprite/*.jsonl | tail -5'
```

## Dependencies

- `github.com/gorilla/websocket` - WebSocket server
- `github.com/fsnotify/fsnotify` - File watching

## Related Projects

- [sprite-mobile](https://github.com/clouvet/sprite-mobile) - Web UI that proxies to this hub

## License

MIT
