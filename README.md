# Claude Hub

WebSocket hub for multi-client Claude Code session synchronization.

## Phase 1: Core Hub + Basic WebSocket (COMPLETE)

Basic WebSocket server that accepts connections and broadcasts messages between clients.

### Running

```bash
# Build
cd ~/.claude-hub
go build -o bin/claude-hub main.go

# Run
./bin/claude-hub
```

The hub listens on port 9090.

### Testing with sprite-mobile

Enable the proxy in sprite-mobile:

```bash
cd ~/.sprite-mobile
USE_GO_HUB=true bun server.ts
```

Then open multiple browser tabs to the same session and send messages. They should broadcast to all tabs.

## Architecture

```
Web Browser → sprite-mobile:8081 → Go Hub:9090
```

## Current Features

- WebSocket connections on port 9090
- Multi-client broadcasting per session
- Client registration/unregistration
- Basic message echo (Phase 1)

## Next Phases

- Phase 2: Claude process management
- Phase 3: File watching + terminal detection
- Phase 4: Message queue + interrupts
- Phase 5: Service integration
