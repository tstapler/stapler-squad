# Terminal Rendering

**ID**: `terminal-render`  
**Status**: stable  
**Since**: v1.0.0

Streams terminal output to the browser with RAF batching.

## RPCs

- `session:stream-terminal`

## Components

- `components/terminal/Terminal.tsx`

## Tests

- Terminal Flickering Fix > should maintain 60fps with incremental rendering
