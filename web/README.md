# Claude Squad Web Client

Next.js web client for Claude Squad session management with ConnectRPC.

## Setup

Install dependencies:

```bash
npm install
```

## Development

Run the development server:

```bash
npm run dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

## ConnectRPC Client Generation

The TypeScript client code is automatically generated from Protocol Buffer definitions using Buf CLI.

### Prerequisites

- [Buf CLI](https://buf.build/docs/installation) installed
- Protocol Buffer definitions in `/proto` directory

### Generate Client Code

To regenerate the TypeScript ConnectRPC client after proto changes:

```bash
# From project root
buf generate
```

Generated files are written to `web/src/gen/session/v1/`:
- `session_pb.ts` - Message types (Session, CreateSessionRequest, etc.)
- `session_connect.ts` - RPC service definitions
- `events_pb.ts` - Event types (SessionEvent, etc.)
- `events_connect.ts` - Event streaming definitions

### Client Usage

Import the generated client and create a ConnectRPC transport:

```typescript
import { createConnectTransport } from "@connectrpc/connect-web";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_connect";

// Create ConnectRPC transport
const transport = createConnectTransport({
  baseUrl: "http://localhost:8080",
});

// Create client and make RPC calls
const client = createClient(SessionService, transport);
const response = await client.listSessions({});
```

## Project Structure

```
web/
├── src/
│   ├── app/           # Next.js app directory (pages, layouts)
│   ├── gen/           # Generated ConnectRPC client code (do not edit)
│   └── lib/           # Utility functions, hooks, and helpers
├── public/            # Static assets
├── package.json       # Dependencies and scripts
└── README.md          # This file
```

## Dependencies

- **Next.js 15** - React framework
- **ConnectRPC** - Type-safe RPC client
  - `@connectrpc/connect` - Core ConnectRPC library
  - `@connectrpc/connect-web` - Browser transport for ConnectRPC
- **Protobuf** - Protocol Buffers for TypeScript
  - `@bufbuild/protobuf` - Protobuf runtime

## Build

Build for production:

```bash
npm run build
npm run start
```

## Learn More

- [Next.js Documentation](https://nextjs.org/docs)
- [ConnectRPC Documentation](https://connectrpc.com/docs)
- [Buf Documentation](https://buf.build/docs)
- [Next.js GitHub Repository](https://github.com/vercel/next.js)
