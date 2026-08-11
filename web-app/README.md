# Stapler Squad Session Management UI

This is the web-based session management interface for Stapler Squad. It's a separate Next.js application from the main marketing website.

## Architecture

- **Framework**: Next.js 15 with TypeScript
- **API Client**: ConnectRPC (TypeScript client for Go backend)
- **Styling**: CSS Modules
- **Build Output**: Static export (embedded in Go binary)

## Development

```bash
# Install dependencies
npm install

# Run development server (port 3001)
npm run dev

# Build for production
npm run build
```

## Generated Code

The `src/gen/` directory contains TypeScript types and RPC clients generated from Protocol Buffer definitions:

```bash
# Regenerate from project root
cd ..
buf generate
```

## Production Deployment

The built application is automatically embedded into the Go server:

1. Build: `npm run build` (creates `out/` directory)
2. Copy to Go server: `cp -r out/ ../server/web/dist/`
3. Go embeds `server/web/dist/` at build time
4. Served at `http://localhost:8543/` when server runs

## Features

- Real-time session monitoring via ConnectRPC streaming
- Session filtering and search
- Session lifecycle management (create, pause, resume, delete)
- Category-based organization
- Status indicators and metadata display

## API Integration

The UI connects to the ConnectRPC API at `http://localhost:8543`:

- `SessionService.ListSessions` - Get all sessions
- `SessionService.WatchSessions` - Real-time updates via streaming
- `SessionService.CreateSession` - Create new session
- `SessionService.UpdateSession` - Update session properties
- `SessionService.DeleteSession` - Remove session

## Related Projects

- **`../web/`** - Marketing website (separate Next.js app)
- **`../server/`** - Go backend with ConnectRPC API
