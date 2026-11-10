"use strict";
exports.id = 41;
exports.ids = [41];
exports.modules = {

/***/ 30041:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  TerminalStressTest: () => (/* binding */ TerminalStressTest)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
;// ./src/lib/test-generators/types.ts
/**
 * Types for terminal stress test data generators
 */ /**
 * Test presets for common scenarios
 */ const TEST_PRESETS = {
    // ASCII video at 30 FPS
    ASCII_30FPS: {
        type: 'ascii-video',
        rate: 30,
        duration: 10,
        width: 80,
        height: 24,
        pattern: 'bouncing-ball'
    },
    // ASCII video at 60 FPS
    ASCII_60FPS: {
        type: 'ascii-video',
        rate: 60,
        duration: 10,
        width: 80,
        height: 24,
        pattern: 'matrix-rain'
    },
    // Log flood at 1K lines/sec
    LOG_1K: {
        type: 'log-flood',
        rate: 1000,
        duration: 5,
        levels: {
            error: 5,
            warn: 15,
            info: 60,
            debug: 20
        },
        timestamps: true,
        colors: true
    },
    // Log flood at 5K lines/sec
    LOG_5K: {
        type: 'log-flood',
        rate: 5000,
        duration: 5,
        levels: {
            error: 5,
            warn: 15,
            info: 60,
            debug: 20
        },
        timestamps: true,
        colors: true
    },
    // Log flood at 10K lines/sec
    LOG_10K: {
        type: 'log-flood',
        rate: 10000,
        duration: 3,
        levels: {
            error: 5,
            warn: 15,
            info: 60,
            debug: 20
        },
        timestamps: true,
        colors: true
    },
    // Color stress test
    COLOR_RAINBOW: {
        type: 'color-stress',
        rate: 60,
        duration: 10,
        mode: 'gradient'
    },
    // Large payload 100KB
    PAYLOAD_100KB: {
        type: 'large-payload',
        rate: 1,
        duration: 5,
        size: 100 * 1024,
        pattern: 'structured'
    },
    // Large payload 1MB
    PAYLOAD_1MB: {
        type: 'large-payload',
        rate: 1,
        duration: 3,
        size: 1024 * 1024,
        pattern: 'structured'
    }
};

;// ./src/lib/test-generators/log-lines.ts
/**
 * Log Line Generator
 * Generates high-volume log output for terminal stress testing
 */ // Simple seeded random number generator
class SeededRandom {
    constructor(seed){
        this.seed = seed;
    }
    next() {
        this.seed = this.seed * 1103515245 + 12345 & 0x7fffffff;
        return this.seed / 0x7fffffff;
    }
    nextInt(max) {
        return Math.floor(this.next() * max);
    }
    choice(arr) {
        return arr[this.nextInt(arr.length)];
    }
}
// Simple checksum
function checksum(str) {
    let hash = 0;
    for(let i = 0; i < str.length; i++){
        const char = str.charCodeAt(i);
        hash = (hash << 5) - hash + char;
        hash = hash & hash;
    }
    return Math.abs(hash);
}
// ANSI color codes for log levels
const LOG_COLORS = {
    error: '\x1b[31m',
    warn: '\x1b[33m',
    info: '\x1b[32m',
    debug: '\x1b[36m'
};
const LOG_PREFIXES = {
    error: '[ERROR]',
    warn: '[WARN] ',
    info: '[INFO] ',
    debug: '[DEBUG]'
};
// Sample log messages for each level
const LOG_MESSAGES = {
    error: [
        'Connection refused: ECONNREFUSED',
        'Database query timeout after 30000ms',
        'Failed to parse JSON response: Unexpected token',
        'Authentication failed: Invalid credentials',
        'Out of memory: heap allocation failed',
        'File not found: /var/log/app.log',
        'Socket hang up: ECONNRESET',
        'TLS handshake failed: certificate expired'
    ],
    warn: [
        'High memory usage: 85% of available heap',
        'Slow query detected: 2500ms execution time',
        'Rate limit approaching: 90% of quota used',
        'Deprecated API endpoint called: /v1/legacy',
        'Cache miss rate increasing: 15%',
        'Connection pool near capacity: 95/100',
        'Disk space low: 10GB remaining',
        'Request retry attempt 2 of 3'
    ],
    info: [
        'Request processed successfully: 200 OK',
        'User authenticated: user@example.com',
        'Cache hit: key=session_12345',
        'Background job completed: export_data_123',
        'WebSocket connection established',
        'Configuration reloaded from file',
        'Health check passed: all services operational',
        'Batch processing started: 1000 items'
    ],
    debug: [
        'Entering function: processRequest()',
        'Variable state: count=42, status=active',
        'HTTP request: GET /api/v2/users?page=1',
        'Query executed: SELECT * FROM users LIMIT 10',
        'Response body: {"success":true,"data":[...]}',
        'Timer started: operationTimeout=5000ms',
        'Event emitted: user.created',
        'Middleware executed: authCheck (2ms)'
    ]
};
// Sample service names
const SERVICES = [
    'api-gateway',
    'user-service',
    'auth-service',
    'payment-service',
    'notification-service',
    'cache-service',
    'worker-service',
    'scheduler'
];
// Sample request IDs
function generateRequestId(rng) {
    const chars = 'abcdef0123456789';
    let id = '';
    for(let i = 0; i < 16; i++){
        id += chars[rng.nextInt(chars.length)];
    }
    return id;
}
/**
 * Log Line Generator class
 */ class LogLineGenerator {
    constructor(config){
        this.sequence = 0;
        this.config = config;
        this.rng = new SeededRandom(config.seed || Date.now());
        // Build weighted level distribution
        const { levels } = config;
        const total = levels.error + levels.warn + levels.info + levels.debug;
        let cumulative = 0;
        this.levelWeights = [];
        for (const [level, weight] of Object.entries(levels)){
            cumulative += weight / total;
            this.levelWeights.push({
                level: level,
                threshold: cumulative
            });
        }
    }
    /**
   * Select log level based on configured distribution
   */ selectLevel() {
        const r = this.rng.next();
        for (const { level, threshold } of this.levelWeights){
            if (r <= threshold) {
                return level;
            }
        }
        return 'info';
    }
    /**
   * Generate a single log line
   */ generateLine() {
        const level = this.selectLevel();
        const message = this.rng.choice(LOG_MESSAGES[level]);
        const service = this.rng.choice(SERVICES);
        const requestId = generateRequestId(this.rng);
        let line = '';
        // Timestamp
        if (this.config.timestamps) {
            const now = new Date();
            const ts = now.toISOString();
            line += `\x1b[90m${ts}\x1b[0m `;
        }
        // Log level with color
        if (this.config.colors) {
            line += `${LOG_COLORS[level]}${LOG_PREFIXES[level]}\x1b[0m `;
        } else {
            line += `${LOG_PREFIXES[level]} `;
        }
        // Service and request ID
        if (this.config.colors) {
            line += `\x1b[35m[${service}]\x1b[0m `;
            line += `\x1b[90m(${requestId})\x1b[0m `;
        } else {
            line += `[${service}] (${requestId}) `;
        }
        // Message
        line += message;
        return line;
    }
    /**
   * Generate a batch of log lines
   */ nextFrame(batchSize = 10) {
        const startTime = performance.now();
        const lines = [];
        for(let i = 0; i < batchSize; i++){
            lines.push(this.generateLine());
        }
        const content = lines.join('\n') + '\n';
        const frame = {
            sequence: this.sequence,
            content,
            timestamp: startTime,
            checksum: checksum(content)
        };
        this.sequence++;
        return frame;
    }
    /**
   * Generate continuous stream of log lines
   * Returns an async generator that yields frames at the configured rate
   */ async *stream(abortSignal) {
        const startTime = performance.now();
        const targetIntervalMs = 1000 / (this.config.rate / 10); // Batch 10 lines per frame
        const durationMs = this.config.duration * 1000;
        let frameCount = 0;
        while(performance.now() - startTime < durationMs){
            if (abortSignal?.aborted) break;
            const frameStartTime = performance.now();
            yield this.nextFrame(10);
            frameCount++;
            // Calculate sleep time to maintain rate
            const elapsed = performance.now() - frameStartTime;
            const sleepTime = Math.max(0, targetIntervalMs - elapsed);
            if (sleepTime > 0) {
                await new Promise((resolve)=>setTimeout(resolve, sleepTime));
            }
        }
    }
    /**
   * Reset generator state
   */ reset() {
        this.sequence = 0;
        this.rng = new SeededRandom(this.config.seed || Date.now());
    }
    /**
   * Get current sequence number
   */ getSequence() {
        return this.sequence;
    }
}

;// ./src/lib/test-generators/ascii-frames.ts
/**
 * ASCII Frame Generator
 * Generates animated ASCII art frames for terminal stress testing
 */ // Simple seeded random number generator for reproducibility
class ascii_frames_SeededRandom {
    constructor(seed){
        this.seed = seed;
    }
    next() {
        this.seed = this.seed * 1103515245 + 12345 & 0x7fffffff;
        return this.seed / 0x7fffffff;
    }
    nextInt(max) {
        return Math.floor(this.next() * max);
    }
}
// Simple checksum for frame validation
function ascii_frames_checksum(str) {
    let hash = 0;
    for(let i = 0; i < str.length; i++){
        const char = str.charCodeAt(i);
        hash = (hash << 5) - hash + char;
        hash = hash & hash;
    }
    return Math.abs(hash);
}
/**
 * Generate bouncing ball animation frame using PARTIAL REDRAWS
 * Instead of redrawing the entire screen, only update:
 * 1. Erase previous ball position
 * 2. Draw new ball position
 * 3. Update footer line
 *
 * This reduces output from ~2000+ chars to ~50 chars per frame
 */ function generateBouncingBall(sequence, width, height, state) {
    const ball = '\x1b[33m\x1b[1mO\x1b[0m'; // Yellow bold ball
    const period = 60; // frames per cycle
    // Calculate ball position using simple harmonic motion
    const t = sequence % period / period * Math.PI * 2;
    const x = Math.floor((Math.sin(t) + 1) / 2 * (width - 2));
    const y = Math.floor((Math.sin(t * 1.5 + 0.5) + 1) / 2 * (height - 3)) + 1;
    let output = '';
    // First frame: draw full screen
    if (!state.initialized) {
        state.initialized = true;
        state.prevX = -1;
        state.prevY = -1;
        const lines = [];
        // Header
        lines.push(`\x1b[36m${'='.repeat(width)}\x1b[0m`);
        // Empty body
        for(let row = 1; row < height - 1; row++){
            lines.push(' '.repeat(width));
        }
        // Footer placeholder
        lines.push(' '.repeat(width));
        output = '\x1b[H' + lines.join('\n');
    }
    // Erase previous ball position (if it changed)
    if (state.prevX >= 0 && state.prevY >= 0 && (state.prevX !== x || state.prevY !== y)) {
        // Move to previous position and erase (row is 1-indexed in ANSI, +1 for header)
        output += `\x1b[${state.prevY + 1};${state.prevX + 1}H `;
    }
    // Draw new ball position
    output += `\x1b[${y + 1};${x + 1}H${ball}`;
    // Update footer (only when sequence changes, which is always)
    const footer = `Frame: ${sequence.toString().padStart(6, '0')} | Ball: (${x},${y})`;
    output += `\x1b[${height};1H\x1b[36m${footer.padEnd(width)}\x1b[0m`;
    // Store current position for next frame
    state.prevX = x;
    state.prevY = y;
    return output;
}
/**
 * Generate scrolling text animation frame
 */ function generateScrollingText(sequence, width, height) {
    const messages = [
        '\x1b[32m[INFO]\x1b[0m Processing batch...',
        '\x1b[33m[WARN]\x1b[0m High memory usage detected',
        '\x1b[31m[ERROR]\x1b[0m Connection timeout',
        '\x1b[34m[DEBUG]\x1b[0m Cache hit ratio: 95%',
        '\x1b[35m[TRACE]\x1b[0m Request completed in 45ms'
    ];
    const lines = [];
    // Header
    lines.push(`\x1b[7m Scrolling Log Test - Frame ${sequence} \x1b[0m`.padEnd(width));
    // Generate scrolling log lines
    for(let row = 1; row < height - 1; row++){
        const lineIndex = (sequence + row) % messages.length;
        const offset = sequence % (width / 2);
        let line = messages[lineIndex];
        // Add timestamp
        const timestamp = new Date(Date.now() - (height - row) * 100).toISOString().slice(11, 23);
        line = `\x1b[90m${timestamp}\x1b[0m ${line}`;
        // Truncate to width
        lines.push(line.slice(0, width).padEnd(width));
    }
    // Footer
    lines.push(`\x1b[7m ${'Press Ctrl+C to stop'.padEnd(width - 2)} \x1b[0m`);
    return '\x1b[H' + lines.join('\n');
}
/**
 * Generate progress bar animation frame
 */ function generateProgressBar(sequence, width, height) {
    const lines = [];
    const numBars = Math.min(height - 4, 10);
    // Header
    lines.push(`\x1b[1m\x1b[36m${'='.repeat(width)}\x1b[0m`);
    lines.push(`\x1b[1m Multi-Progress Test - Frame ${sequence} \x1b[0m`.padEnd(width));
    lines.push(`\x1b[1m\x1b[36m${'='.repeat(width)}\x1b[0m`);
    // Progress bars with different speeds
    for(let i = 0; i < numBars; i++){
        const speed = (i + 1) * 0.5;
        const progress = sequence * speed % 100 / 100;
        const barWidth = width - 20;
        const filled = Math.floor(progress * barWidth);
        const empty = barWidth - filled;
        const colors = [
            '\x1b[32m',
            '\x1b[33m',
            '\x1b[34m',
            '\x1b[35m',
            '\x1b[36m'
        ];
        const color = colors[i % colors.length];
        const bar = `${color}${'█'.repeat(filled)}\x1b[90m${'░'.repeat(empty)}\x1b[0m`;
        const label = `Task ${(i + 1).toString().padStart(2, '0')}`;
        const percent = `${Math.floor(progress * 100).toString().padStart(3)}%`;
        lines.push(`${label} [${bar}] ${percent}`);
    }
    // Fill remaining space
    while(lines.length < height - 1){
        lines.push(' '.repeat(width));
    }
    // Footer
    const elapsed = Math.floor(sequence / 60);
    lines.push(`\x1b[90mElapsed: ${elapsed}s | FPS Target: 60\x1b[0m`.padEnd(width));
    return '\x1b[H' + lines.join('\n');
}
/**
 * Generate matrix rain animation frame
 */ function generateMatrixRain(sequence, width, height, rng) {
    const chars = 'ｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉﾊﾋﾌﾍﾎﾏﾐﾑﾒﾓﾔﾕﾖﾗﾘﾙﾚﾛﾜﾝ0123456789';
    const lines = [];
    // Initialize or update rain columns
    const columns = [];
    for(let col = 0; col < width; col++){
        // Each column has a different phase based on seed
        const phase = (rng.nextInt(height) + sequence) % (height + 10);
        columns.push(phase);
    }
    // Generate frame
    for(let row = 0; row < height; row++){
        let line = '';
        for(let col = 0; col < width; col++){
            const columnPhase = (columns[col] + col * 3 + sequence) % (height + 10);
            const distFromHead = row - columnPhase % height;
            if (distFromHead === 0) {
                // Head of rain drop (bright)
                const char = chars[rng.nextInt(chars.length)];
                line += `\x1b[97m\x1b[1m${char}\x1b[0m`;
            } else if (distFromHead > 0 && distFromHead < 8) {
                // Trail (fading green)
                const brightness = Math.max(22, 28 - distFromHead);
                const char = chars[rng.nextInt(chars.length)];
                line += `\x1b[38;5;${brightness}m${char}\x1b[0m`;
            } else if (rng.next() < 0.02) {
                // Random flicker
                const char = chars[rng.nextInt(chars.length)];
                line += `\x1b[38;5;22m${char}\x1b[0m`;
            } else {
                line += ' ';
            }
        }
        lines.push(line);
    }
    return '\x1b[H' + lines.join('\n');
}
/**
 * ASCII Frame Generator class
 */ class AsciiFrameGenerator {
    constructor(config){
        this.sequence = 0;
        this.bouncingBallState = {
            prevX: -1,
            prevY: -1,
            initialized: false
        };
        this.config = config;
        this.rng = new ascii_frames_SeededRandom(config.seed || Date.now());
    }
    /**
   * Generate next frame
   */ nextFrame() {
        const startTime = performance.now();
        let content;
        switch(this.config.pattern){
            case 'bouncing-ball':
                content = generateBouncingBall(this.sequence, this.config.width, this.config.height, this.bouncingBallState);
                break;
            case 'scrolling-text':
                content = generateScrollingText(this.sequence, this.config.width, this.config.height);
                break;
            case 'progress-bar':
                content = generateProgressBar(this.sequence, this.config.width, this.config.height);
                break;
            case 'matrix-rain':
                content = generateMatrixRain(this.sequence, this.config.width, this.config.height, this.rng);
                break;
            default:
                content = generateBouncingBall(this.sequence, this.config.width, this.config.height, this.bouncingBallState);
        }
        const frame = {
            sequence: this.sequence,
            content,
            timestamp: startTime,
            checksum: ascii_frames_checksum(content)
        };
        this.sequence++;
        return frame;
    }
    /**
   * Reset generator state
   */ reset() {
        this.sequence = 0;
        this.rng = new ascii_frames_SeededRandom(this.config.seed || Date.now());
        // Reset pattern-specific state
        this.bouncingBallState = {
            prevX: -1,
            prevY: -1,
            initialized: false
        };
    }
    /**
   * Get current sequence number
   */ getSequence() {
        return this.sequence;
    }
}

;// ./src/lib/test-generators/color-stress.ts
/**
 * Color Stress Generator
 * Generates terminal output with maximum ANSI color complexity for stress testing
 */ // Simple checksum
function color_stress_checksum(str) {
    let hash = 0;
    for(let i = 0; i < str.length; i++){
        const char = str.charCodeAt(i);
        hash = (hash << 5) - hash + char;
        hash = hash & hash;
    }
    return Math.abs(hash);
}
/**
 * Generate 16-color ANSI output
 */ function generate16Color(sequence, width, height) {
    const lines = [];
    const fgColors = [
        30,
        31,
        32,
        33,
        34,
        35,
        36,
        37,
        90,
        91,
        92,
        93,
        94,
        95,
        96,
        97
    ];
    const bgColors = [
        40,
        41,
        42,
        43,
        44,
        45,
        46,
        47
    ];
    // Header
    lines.push(`\x1b[1m\x1b[7m 16-Color Mode - Frame ${sequence} \x1b[0m`.padEnd(width + 20));
    // Color grid
    for(let row = 0; row < Math.min(height - 2, 16); row++){
        let line = '';
        for(let col = 0; col < Math.min(width, 64); col++){
            const fgIndex = (row + col + sequence) % fgColors.length;
            const bgIndex = (row + sequence) % bgColors.length;
            const char = String.fromCharCode(65 + (row + col) % 26);
            line += `\x1b[${fgColors[fgIndex]}m\x1b[${bgColors[bgIndex]}m${char}\x1b[0m`;
        }
        lines.push(line);
    }
    // Fill remaining lines
    while(lines.length < height){
        lines.push(' '.repeat(width));
    }
    return '\x1b[H' + lines.join('\n');
}
/**
 * Generate 256-color ANSI output
 */ function generate256Color(sequence, width, height) {
    const lines = [];
    // Header
    lines.push(`\x1b[1m\x1b[7m 256-Color Mode - Frame ${sequence} \x1b[0m`.padEnd(width + 20));
    // Color cube (6x6x6 = 216 colors, starting at 16)
    for(let row = 0; row < Math.min(height - 2, 36); row++){
        let line = '';
        for(let col = 0; col < Math.min(width, 72); col++){
            // Cycle through 256 colors with animation
            const colorIndex = 16 + (row * 6 + col + sequence * 3) % 216;
            const block = '█';
            line += `\x1b[38;5;${colorIndex}m${block}\x1b[0m`;
        }
        lines.push(line);
    }
    // Grayscale ramp
    if (lines.length < height - 1) {
        let grayLine = '';
        for(let i = 0; i < Math.min(width, 24); i++){
            const grayIndex = 232 + (i + sequence) % 24;
            grayLine += `\x1b[38;5;${grayIndex}m█\x1b[0m`;
        }
        lines.push(grayLine);
    }
    // Fill remaining lines
    while(lines.length < height){
        lines.push(' '.repeat(width));
    }
    return '\x1b[H' + lines.join('\n');
}
/**
 * Generate true-color (24-bit) output
 */ function generateTrueColor(sequence, width, height) {
    const lines = [];
    // Header
    lines.push(`\x1b[1m\x1b[7m True-Color Mode - Frame ${sequence} \x1b[0m`.padEnd(width + 20));
    // HSV to RGB conversion
    const hsvToRgb = (h, s, v)=>{
        const c = v * s;
        const x = c * (1 - Math.abs(h / 60 % 2 - 1));
        const m = v - c;
        let r = 0, g = 0, b = 0;
        if (h < 60) {
            r = c;
            g = x;
        } else if (h < 120) {
            r = x;
            g = c;
        } else if (h < 180) {
            g = c;
            b = x;
        } else if (h < 240) {
            g = x;
            b = c;
        } else if (h < 300) {
            r = x;
            b = c;
        } else {
            r = c;
            b = x;
        }
        return [
            Math.round((r + m) * 255),
            Math.round((g + m) * 255),
            Math.round((b + m) * 255)
        ];
    };
    // Generate colorful grid with animation
    for(let row = 0; row < Math.min(height - 2, 20); row++){
        let line = '';
        for(let col = 0; col < Math.min(width, 80); col++){
            const hue = (col * 5 + row * 15 + sequence * 10) % 360;
            const saturation = 0.7 + row / height * 0.3;
            const value = 0.8 + Math.sin((col + sequence) * 0.1) * 0.2;
            const [r, g, b] = hsvToRgb(hue, saturation, value);
            line += `\x1b[38;2;${r};${g};${b}m█\x1b[0m`;
        }
        lines.push(line);
    }
    // Fill remaining lines
    while(lines.length < height){
        lines.push(' '.repeat(width));
    }
    return '\x1b[H' + lines.join('\n');
}
/**
 * Generate smooth gradient animation
 */ function generateGradient(sequence, width, height) {
    const lines = [];
    // Header
    lines.push(`\x1b[1m\x1b[7m Gradient Mode - Frame ${sequence} \x1b[0m`.padEnd(width + 20));
    // Generate rainbow gradient with wave animation
    for(let row = 0; row < height - 1; row++){
        let line = '';
        for(let col = 0; col < width; col++){
            // Create wave effect
            const wave = Math.sin((col + row + sequence) * 0.1) * 0.5 + 0.5;
            const hue = (col + sequence * 5) % 360;
            // HSV to RGB (simplified)
            const h = hue / 60;
            const i = Math.floor(h);
            const f = h - i;
            const p = 0;
            const q = Math.floor(255 * (1 - f));
            const t = Math.floor(255 * f);
            const v = Math.floor(200 + wave * 55);
            let r = 0, g = 0, b = 0;
            switch(i % 6){
                case 0:
                    r = v;
                    g = t;
                    b = p;
                    break;
                case 1:
                    r = q;
                    g = v;
                    b = p;
                    break;
                case 2:
                    r = p;
                    g = v;
                    b = t;
                    break;
                case 3:
                    r = p;
                    g = q;
                    b = v;
                    break;
                case 4:
                    r = t;
                    g = p;
                    b = v;
                    break;
                case 5:
                    r = v;
                    g = p;
                    b = q;
                    break;
            }
            // Vertical gradient blend
            const brightness = 1 - row / height * 0.5;
            r = Math.floor(r * brightness);
            g = Math.floor(g * brightness);
            b = Math.floor(b * brightness);
            line += `\x1b[38;2;${r};${g};${b}m█\x1b[0m`;
        }
        lines.push(line);
    }
    return '\x1b[H' + lines.join('\n');
}
/**
 * Color Stress Generator class
 */ class ColorStressGenerator {
    constructor(config, width = 80, height = 24){
        this.sequence = 0;
        this.width = 80;
        this.height = 24;
        this.config = config;
        this.width = width;
        this.height = height;
    }
    /**
   * Generate next frame
   */ nextFrame() {
        const startTime = performance.now();
        let content;
        switch(this.config.mode){
            case '16-color':
                content = generate16Color(this.sequence, this.width, this.height);
                break;
            case '256-color':
                content = generate256Color(this.sequence, this.width, this.height);
                break;
            case 'true-color':
                content = generateTrueColor(this.sequence, this.width, this.height);
                break;
            case 'gradient':
                content = generateGradient(this.sequence, this.width, this.height);
                break;
            default:
                content = generateGradient(this.sequence, this.width, this.height);
        }
        const frame = {
            sequence: this.sequence,
            content,
            timestamp: startTime,
            checksum: color_stress_checksum(content)
        };
        this.sequence++;
        return frame;
    }
    /**
   * Reset generator state
   */ reset() {
        this.sequence = 0;
    }
    /**
   * Get current sequence number
   */ getSequence() {
        return this.sequence;
    }
    /**
   * Set terminal dimensions
   */ setDimensions(width, height) {
        this.width = width;
        this.height = height;
    }
}

;// ./src/lib/test-generators/large-payload.ts
/**
 * Large Payload Generator
 * Generates large terminal payloads for buffer stress testing
 */ // Simple seeded random number generator
class large_payload_SeededRandom {
    constructor(seed){
        this.seed = seed;
    }
    next() {
        this.seed = this.seed * 1103515245 + 12345 & 0x7fffffff;
        return this.seed / 0x7fffffff;
    }
    nextInt(max) {
        return Math.floor(this.next() * max);
    }
}
// Simple checksum
function large_payload_checksum(str) {
    let hash = 0;
    for(let i = 0; i < str.length; i++){
        const char = str.charCodeAt(i);
        hash = (hash << 5) - hash + char;
        hash = hash & hash;
    }
    return Math.abs(hash);
}
/**
 * Generate random printable content
 */ function generateRandomContent(size, rng) {
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()';
    let content = '';
    let lineLength = 0;
    for(let i = 0; i < size; i++){
        if (lineLength >= 80) {
            content += '\n';
            lineLength = 0;
        } else {
            content += chars[rng.nextInt(chars.length)];
            lineLength++;
        }
    }
    return content;
}
/**
 * Generate repeated pattern content
 */ function generateRepeatedContent(size, sequence) {
    const pattern = `[PAYLOAD-${sequence.toString().padStart(6, '0')}] `;
    const linePattern = pattern + '='.repeat(80 - pattern.length) + '\n';
    let content = '';
    while(content.length < size){
        content += linePattern;
    }
    return content.slice(0, size);
}
/**
 * Generate structured log-like content
 */ function generateStructuredContent(size, sequence, rng) {
    const lines = [];
    const colors = [
        '\x1b[32m',
        '\x1b[33m',
        '\x1b[34m',
        '\x1b[35m',
        '\x1b[36m'
    ];
    // Header
    lines.push(`\x1b[1m\x1b[7m Large Payload Test - Sequence ${sequence} - Size: ${(size / 1024).toFixed(1)}KB \x1b[0m`);
    lines.push(`${'='.repeat(80)}`);
    let currentSize = lines.join('\n').length + 2;
    // Generate structured lines until we reach target size
    let lineNum = 1;
    while(currentSize < size - 100){
        const color = colors[lineNum % colors.length];
        const timestamp = new Date().toISOString();
        // Vary line length for realistic output
        const dataLength = 20 + rng.nextInt(40);
        const data = 'x'.repeat(dataLength);
        const line = `${color}[${timestamp}]\x1b[0m Line ${lineNum.toString().padStart(6, '0')}: ${data}`;
        lines.push(line);
        currentSize += line.length + 1;
        lineNum++;
    }
    // Footer with checksum info
    lines.push(`${'='.repeat(80)}`);
    lines.push(`\x1b[1mEnd of payload - ${lineNum} lines, ${currentSize} bytes\x1b[0m`);
    return lines.join('\n') + '\n';
}
/**
 * Large Payload Generator class
 */ class LargePayloadGenerator {
    constructor(config){
        this.sequence = 0;
        this.config = config;
        this.rng = new large_payload_SeededRandom(config.seed || Date.now());
    }
    /**
   * Generate next payload
   */ nextFrame() {
        const startTime = performance.now();
        let content;
        switch(this.config.pattern){
            case 'random':
                content = generateRandomContent(this.config.size, this.rng);
                break;
            case 'repeated':
                content = generateRepeatedContent(this.config.size, this.sequence);
                break;
            case 'structured':
                content = generateStructuredContent(this.config.size, this.sequence, this.rng);
                break;
            default:
                content = generateStructuredContent(this.config.size, this.sequence, this.rng);
        }
        const frame = {
            sequence: this.sequence,
            content,
            timestamp: startTime,
            checksum: large_payload_checksum(content)
        };
        this.sequence++;
        return frame;
    }
    /**
   * Generate payload and split into chunks
   */ nextFrameChunked(chunkSize = 4096) {
        const frame = this.nextFrame();
        const chunks = [];
        for(let i = 0; i < frame.content.length; i += chunkSize){
            const chunk = frame.content.slice(i, i + chunkSize);
            chunks.push({
                sequence: frame.sequence * 1000 + Math.floor(i / chunkSize),
                content: chunk,
                timestamp: frame.timestamp,
                checksum: large_payload_checksum(chunk)
            });
        }
        return chunks;
    }
    /**
   * Reset generator state
   */ reset() {
        this.sequence = 0;
        this.rng = new large_payload_SeededRandom(this.config.seed || Date.now());
    }
    /**
   * Get current sequence number
   */ getSequence() {
        return this.sequence;
    }
    /**
   * Get configured payload size
   */ getPayloadSize() {
        return this.config.size;
    }
}

;// ./src/lib/test-generators/index.ts
/**
 * Terminal Stress Test Generators
 *
 * This module provides data generators for stress testing terminal streaming:
 * - ASCII video playback at various frame rates
 * - High-volume log output simulation
 * - Color stress testing (16/256/true color)
 * - Large payload testing
 */ 








/**
 * Factory function to create appropriate generator based on config
 */ function createGenerator(config) {
    switch(config.type){
        case 'ascii-video':
            return new AsciiFrameGenerator(config);
        case 'log-flood':
            return new LogLineGenerator(config);
        case 'color-stress':
            return new ColorStressGenerator(config);
        case 'large-payload':
            return new LargePayloadGenerator(config);
        default:
            throw new Error(`Unknown generator type: ${config.type}`);
    }
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+xterm@6.0.0/node_modules/@xterm/xterm/lib/xterm.mjs
var xterm = __webpack_require__(35986);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-fit@0.11.0/node_modules/@xterm/addon-fit/lib/addon-fit.mjs
var addon_fit = __webpack_require__(79212);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-webgl@0.19.0/node_modules/@xterm/addon-webgl/lib/addon-webgl.mjs
var addon_webgl = __webpack_require__(9382);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+xterm@6.0.0/node_modules/@xterm/xterm/css/xterm.css
var css_xterm = __webpack_require__(15577);
;// ./src/app/test/terminal-stress/TerminalStressTest.tsx
/* __next_internal_client_entry_do_not_use__ TerminalStressTest auto */ 
// analytics-exempt


// Static top-level imports, loaded only once this module's own chunk is pulled in by the
// lazyComponent() boundary in page.tsx (matching XtermTerminal.tsx's pattern). The previous
// `typeof window !== 'undefined'` + require() guard let this page's static import of
// @xterm/xterm's CSS side-effect module land in the route's own chunk, which Bun's bundler
// then failed to wire up correctly (ReferenceError: __commonJS is not defined) -- see
// project_plans/web-app-bun-build/research/spike-results.md, "### 9".




/**
 * Terminal Stress Test Page
 * Tests terminal streaming performance under high-volume data conditions
 */ function TerminalStressTest() {
    const terminalRef = (0,react.useRef)(null);
    const [terminal, setTerminal] = (0,react.useState)(null);
    const [fitAddon, setFitAddon] = (0,react.useState)(null);
    const [rendererType, setRendererType] = (0,react.useState)('unknown');
    // Test state
    const [status, setStatus] = (0,react.useState)('idle');
    const [selectedPreset, setSelectedPreset] = (0,react.useState)('ASCII_30FPS');
    const [customConfig, setCustomConfig] = (0,react.useState)(null);
    // Metrics
    const [renderMetrics, setRenderMetrics] = (0,react.useState)({
        framesRendered: 0,
        frameTimes: [],
        avgFrameTime: 0,
        p95FrameTime: 0,
        p99FrameTime: 0,
        maxFrameTime: 0,
        memoryUsage: [],
        peakMemory: 0,
        memoryGrowthRate: 0
    });
    const [progress, setProgress] = (0,react.useState)({
        elapsed: 0,
        framesGenerated: 0,
        bytesGenerated: 0
    });
    // Refs for test control
    const abortControllerRef = (0,react.useRef)(null);
    const frameTimesRef = (0,react.useRef)([]);
    const lastFrameTimeRef = (0,react.useRef)(0);
    const metricsRef = (0,react.useRef)(renderMetrics);
    // Update metrics ref
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        metricsRef.current = renderMetrics;
    }, [
        renderMetrics
    ]);
    // Initialize terminal
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!terminalRef.current) return;
        const term = new xterm/* Terminal */.B({
            cols: 80,
            rows: 24,
            scrollback: 10000,
            theme: {
                background: '#1e1e1e',
                foreground: '#d4d4d4',
                cursor: '#d4d4d4'
            },
            allowProposedApi: true
        });
        const fit = new addon_fit/* FitAddon */.Y();
        term.loadAddon(fit);
        term.open(terminalRef.current);
        fit.fit();
        // Try to load WebGL addon for hardware-accelerated rendering
        let webglLoaded = false;
        try {
            const webgl = new addon_webgl.WebglAddon();
            webgl.onContextLoss(()=>{
                console.warn('WebGL context lost, falling back to canvas');
                webgl.dispose();
                setRendererType('canvas');
            });
            term.loadAddon(webgl);
            webglLoaded = true;
            setRendererType('webgl');
            console.log('WebGL renderer loaded successfully');
        } catch (e) {
            console.warn('WebGL addon failed to load, using canvas renderer:', e);
            setRendererType('canvas');
        }
        setTerminal(term);
        setFitAddon(fit);
        // Expose for Playwright testing
        window.stressTestTerminal = term;
        window.stressTestMetrics = metricsRef;
        window.stressTestRendererType = webglLoaded ? 'webgl' : 'canvas';
        // Handle resize
        const resizeObserver = new ResizeObserver(()=>{
            fit.fit();
        });
        resizeObserver.observe(terminalRef.current);
        return ()=>{
            resizeObserver.disconnect();
            term.dispose();
        };
    }, []);
    // Calculate percentile
    const percentile = (0,react.useCallback)((arr, p)=>{
        if (arr.length === 0) return 0;
        const sorted = [
            ...arr
        ].sort((a, b)=>a - b);
        const index = Math.ceil(p / 100 * sorted.length) - 1;
        return sorted[Math.max(0, index)];
    }, []);
    // Update render metrics
    const updateMetrics = (0,react.useCallback)(()=>{
        const frameTimes = frameTimesRef.current;
        if (frameTimes.length === 0) return;
        const avg = frameTimes.reduce((a, b)=>a + b, 0) / frameTimes.length;
        const p95 = percentile(frameTimes, 95);
        const p99 = percentile(frameTimes, 99);
        const max = Math.max(...frameTimes);
        // Memory tracking (Chrome only)
        let memoryUsage = metricsRef.current.memoryUsage;
        let peakMemory = metricsRef.current.peakMemory;
        let memoryGrowthRate = metricsRef.current.memoryGrowthRate;
        if (performance.memory) {
            const currentMemory = performance.memory.usedJSHeapSize;
            memoryUsage = [
                ...memoryUsage,
                currentMemory
            ];
            peakMemory = Math.max(peakMemory, currentMemory);
            // Calculate growth rate (bytes/sec) over last 10 samples
            if (memoryUsage.length >= 10) {
                const recent = memoryUsage.slice(-10);
                const growth = recent[recent.length - 1] - recent[0];
                // Assuming ~1 sample per 100ms
                memoryGrowthRate = growth / (10 * 0.1);
            }
        }
        setRenderMetrics({
            framesRendered: frameTimes.length,
            frameTimes: [
                ...frameTimes
            ],
            avgFrameTime: avg,
            p95FrameTime: p95,
            p99FrameTime: p99,
            maxFrameTime: max,
            memoryUsage,
            peakMemory,
            memoryGrowthRate
        });
    }, [
        percentile
    ]);
    // Handle frame received
    const handleFrame = (0,react.useCallback)((frame)=>{
        if (!terminal) return;
        const now = performance.now();
        if (lastFrameTimeRef.current > 0) {
            const frameTime = now - lastFrameTimeRef.current;
            frameTimesRef.current.push(frameTime);
            // Keep last 1000 frame times
            if (frameTimesRef.current.length > 1000) {
                frameTimesRef.current = frameTimesRef.current.slice(-1000);
            }
        }
        lastFrameTimeRef.current = now;
        // Write frame to terminal
        terminal.write(frame.content);
        // Update metrics periodically
        if (frameTimesRef.current.length % 10 === 0) {
            updateMetrics();
        }
    }, [
        terminal,
        updateMetrics
    ]);
    // Start test
    const startTest = (0,react.useCallback)(async ()=>{
        if (!terminal || status === 'running') return;
        // Reset state
        setStatus('running');
        frameTimesRef.current = [];
        lastFrameTimeRef.current = 0;
        setProgress({
            elapsed: 0,
            framesGenerated: 0,
            bytesGenerated: 0
        });
        setRenderMetrics({
            framesRendered: 0,
            frameTimes: [],
            avgFrameTime: 0,
            p95FrameTime: 0,
            p99FrameTime: 0,
            maxFrameTime: 0,
            memoryUsage: [],
            peakMemory: 0,
            memoryGrowthRate: 0
        });
        // Clear terminal
        terminal.clear();
        // Create abort controller
        abortControllerRef.current = new AbortController();
        // Get config
        const config = customConfig || TEST_PRESETS[selectedPreset];
        // Create generator
        const generator = createGenerator(config);
        const startTime = performance.now();
        const durationMs = config.duration * 1000;
        const targetIntervalMs = 1000 / config.rate;
        // For log flood, use batching
        const batchSize = config.type === 'log-flood' ? Math.ceil(config.rate / 100) : 1;
        // Track last frame time for throttling
        let lastRenderTime = 0;
        let totalBytesGenerated = 0;
        // Use requestAnimationFrame for proper vsync and frame timing
        const runLoop = (currentTime)=>{
            if (abortControllerRef.current?.signal.aborted) {
                setStatus('completed');
                updateMetrics();
                return;
            }
            const elapsed = currentTime - startTime;
            if (elapsed >= durationMs) {
                setStatus('completed');
                updateMetrics();
                return;
            }
            // Check if enough time has passed since last frame (throttle to target rate)
            const timeSinceLastFrame = currentTime - lastRenderTime;
            if (timeSinceLastFrame >= targetIntervalMs) {
                lastRenderTime = currentTime;
                // Generate and render frame
                const frame = generator.nextFrame(batchSize);
                handleFrame(frame);
                totalBytesGenerated += frame.content.length;
                // Update progress (batch updates to reduce React overhead)
                if (frameTimesRef.current.length % 5 === 0) {
                    setProgress({
                        elapsed,
                        framesGenerated: generator.getSequence(),
                        bytesGenerated: totalBytesGenerated
                    });
                }
            }
            // Schedule next frame using requestAnimationFrame for proper vsync
            requestAnimationFrame(runLoop);
        };
        // Start the animation loop
        requestAnimationFrame(runLoop);
    }, [
        terminal,
        status,
        customConfig,
        selectedPreset,
        handleFrame,
        updateMetrics
    ]);
    // Stop test
    const stopTest = (0,react.useCallback)(()=>{
        if (abortControllerRef.current) {
            abortControllerRef.current.abort();
            abortControllerRef.current = null;
        }
        setStatus('completed');
        updateMetrics();
    }, [
        updateMetrics
    ]);
    // Reset test
    const resetTest = (0,react.useCallback)(()=>{
        stopTest();
        terminal?.clear();
        setStatus('idle');
        frameTimesRef.current = [];
        lastFrameTimeRef.current = 0;
        setProgress({
            elapsed: 0,
            framesGenerated: 0,
            bytesGenerated: 0
        });
        setRenderMetrics({
            framesRendered: 0,
            frameTimes: [],
            avgFrameTime: 0,
            p95FrameTime: 0,
            p99FrameTime: 0,
            maxFrameTime: 0,
            memoryUsage: [],
            peakMemory: 0,
            memoryGrowthRate: 0
        });
    }, [
        terminal,
        stopTest
    ]);
    // Export metrics as JSON
    const exportMetrics = (0,react.useCallback)(()=>{
        const data = {
            config: customConfig || TEST_PRESETS[selectedPreset],
            renderMetrics,
            progress,
            timestamp: new Date().toISOString()
        };
        const blob = new Blob([
            JSON.stringify(data, null, 2)
        ], {
            type: 'application/json'
        });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `stress-test-${Date.now()}.json`;
        a.click();
        URL.revokeObjectURL(url);
    }, [
        customConfig,
        selectedPreset,
        renderMetrics,
        progress
    ]);
    // Format bytes
    const formatBytes = (bytes)=>{
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
    };
    // Format time
    const formatTime = (ms)=>{
        if (ms < 1000) return `${ms.toFixed(0)}ms`;
        return `${(ms / 1000).toFixed(1)}s`;
    };
    // Get status color
    const getStatusColor = ()=>{
        switch(status){
            case 'running':
                return '#f59e0b';
            case 'completed':
                return '#10b981';
            case 'failed':
                return '#ef4444';
            default:
                return '#6b7280';
        }
    };
    // Get frame time status
    const getFrameTimeStatus = (ms)=>{
        if (ms <= 16.67) return {
            color: '#10b981',
            label: 'Excellent (60fps)'
        };
        if (ms <= 33.33) return {
            color: '#f59e0b',
            label: 'Good (30fps)'
        };
        return {
            color: '#ef4444',
            label: 'Poor (<30fps)'
        };
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        style: {
            padding: '20px',
            fontFamily: 'system-ui, sans-serif',
            maxWidth: '1400px',
            margin: '0 auto'
        },
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h1", {
                style: {
                    marginBottom: '10px'
                },
                children: "Terminal Streaming Stress Test"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                style: {
                    color: '#6b7280',
                    marginBottom: '20px'
                },
                children: "Test terminal performance under high-volume streaming conditions"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                style: {
                    display: 'flex',
                    gap: '20px',
                    marginBottom: '20px',
                    flexWrap: 'wrap',
                    alignItems: 'flex-end'
                },
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("label", {
                                style: {
                                    display: 'block',
                                    marginBottom: '5px',
                                    fontWeight: 500
                                },
                                children: "Test Preset"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("select", {
                                value: selectedPreset,
                                onChange: (e)=>setSelectedPreset(e.target.value),
                                disabled: status === 'running',
                                "data-testid": "preset-selector",
                                style: {
                                    padding: '8px 12px',
                                    borderRadius: '6px',
                                    border: '1px solid #d1d5db',
                                    minWidth: '200px'
                                },
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("optgroup", {
                                        label: "ASCII Video",
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                                value: "ASCII_30FPS",
                                                children: "ASCII Video @ 30 FPS"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                                value: "ASCII_60FPS",
                                                children: "ASCII Video @ 60 FPS (Matrix Rain)"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("optgroup", {
                                        label: "Log Flood",
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                                value: "LOG_1K",
                                                children: "Log Flood @ 1K lines/sec"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                                value: "LOG_5K",
                                                children: "Log Flood @ 5K lines/sec"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                                value: "LOG_10K",
                                                children: "Log Flood @ 10K lines/sec"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("optgroup", {
                                        label: "Color Stress",
                                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                            value: "COLOR_RAINBOW",
                                            children: "Rainbow Gradient"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("optgroup", {
                                        label: "Large Payload",
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                                value: "PAYLOAD_100KB",
                                                children: "Large Payload (100KB)"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                                value: "PAYLOAD_1MB",
                                                children: "Large Payload (1MB)"
                                            })
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        style: {
                            display: 'flex',
                            gap: '10px'
                        },
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                onClick: startTest,
                                disabled: status === 'running',
                                "data-testid": "start-test",
                                style: {
                                    padding: '8px 16px',
                                    borderRadius: '6px',
                                    border: 'none',
                                    background: status === 'running' ? '#9ca3af' : '#10b981',
                                    color: 'white',
                                    fontWeight: 500,
                                    cursor: status === 'running' ? 'not-allowed' : 'pointer'
                                },
                                children: status === 'running' ? 'Running...' : 'Start Test'
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                onClick: stopTest,
                                disabled: status !== 'running',
                                "data-testid": "stop-test",
                                style: {
                                    padding: '8px 16px',
                                    borderRadius: '6px',
                                    border: 'none',
                                    background: status !== 'running' ? '#9ca3af' : '#ef4444',
                                    color: 'white',
                                    fontWeight: 500,
                                    cursor: status !== 'running' ? 'not-allowed' : 'pointer'
                                },
                                children: "Stop"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                onClick: resetTest,
                                "data-testid": "reset-test",
                                style: {
                                    padding: '8px 16px',
                                    borderRadius: '6px',
                                    border: '1px solid #d1d5db',
                                    background: 'white',
                                    fontWeight: 500,
                                    cursor: 'pointer'
                                },
                                children: "Reset"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                onClick: exportMetrics,
                                disabled: renderMetrics.framesRendered === 0,
                                "data-testid": "export-metrics",
                                style: {
                                    padding: '8px 16px',
                                    borderRadius: '6px',
                                    border: '1px solid #d1d5db',
                                    background: renderMetrics.framesRendered === 0 ? '#f3f4f6' : 'white',
                                    fontWeight: 500,
                                    cursor: renderMetrics.framesRendered === 0 ? 'not-allowed' : 'pointer'
                                },
                                children: "Export JSON"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                style: {
                    display: 'grid',
                    gridTemplateColumns: '350px 1fr',
                    gap: '20px'
                },
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        "data-testid": "metrics-panel",
                        style: {
                            background: '#f9fafb',
                            borderRadius: '8px',
                            padding: '16px',
                            border: '1px solid #e5e7eb'
                        },
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("h3", {
                                style: {
                                    margin: '0 0 16px 0',
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: '8px',
                                    flexWrap: 'wrap'
                                },
                                children: [
                                    "Performance Metrics",
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        "data-testid": "test-status",
                                        style: {
                                            fontSize: '12px',
                                            padding: '2px 8px',
                                            borderRadius: '9999px',
                                            background: getStatusColor(),
                                            color: 'white',
                                            textTransform: 'capitalize'
                                        },
                                        children: status
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        "data-testid": "renderer-type",
                                        style: {
                                            fontSize: '12px',
                                            padding: '2px 8px',
                                            borderRadius: '9999px',
                                            background: rendererType === 'webgl' ? '#10b981' : rendererType === 'canvas' ? '#f59e0b' : '#6b7280',
                                            color: 'white',
                                            textTransform: 'uppercase'
                                        },
                                        children: rendererType
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                style: {
                                    marginBottom: '16px'
                                },
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        style: {
                                            display: 'flex',
                                            justifyContent: 'space-between',
                                            marginBottom: '4px'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                children: "Progress"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                "data-testid": "progress-elapsed",
                                                children: formatTime(progress.elapsed)
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                        style: {
                                            height: '4px',
                                            background: '#e5e7eb',
                                            borderRadius: '2px',
                                            overflow: 'hidden'
                                        },
                                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                            style: {
                                                height: '100%',
                                                width: `${Math.min(100, progress.elapsed / ((customConfig || TEST_PRESETS[selectedPreset]).duration * 1000) * 100)}%`,
                                                background: '#10b981',
                                                transition: 'width 0.1s'
                                            }
                                        })
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                style: {
                                    display: 'grid',
                                    gap: '12px'
                                },
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        style: {
                                            display: 'flex',
                                            justifyContent: 'space-between'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                children: "Frames Rendered"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                "data-testid": "frames-rendered",
                                                style: {
                                                    fontWeight: 600
                                                },
                                                children: renderMetrics.framesRendered.toLocaleString()
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        style: {
                                            display: 'flex',
                                            justifyContent: 'space-between'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                children: "Bytes Generated"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                "data-testid": "bytes-generated",
                                                style: {
                                                    fontWeight: 600
                                                },
                                                children: formatBytes(progress.bytesGenerated)
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("hr", {
                                        style: {
                                            border: 'none',
                                            borderTop: '1px solid #e5e7eb',
                                            margin: '4px 0'
                                        }
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                                style: {
                                                    display: 'flex',
                                                    justifyContent: 'space-between',
                                                    marginBottom: '4px'
                                                },
                                                children: [
                                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                        children: "Avg Frame Time"
                                                    }),
                                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                                        "data-testid": "avg-frame-time",
                                                        style: {
                                                            fontWeight: 600,
                                                            color: getFrameTimeStatus(renderMetrics.avgFrameTime).color
                                                        },
                                                        children: [
                                                            renderMetrics.avgFrameTime.toFixed(2),
                                                            "ms"
                                                        ]
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                                style: {
                                                    fontSize: '12px',
                                                    color: '#6b7280'
                                                },
                                                children: getFrameTimeStatus(renderMetrics.avgFrameTime).label
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        style: {
                                            display: 'flex',
                                            justifyContent: 'space-between'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                children: "P95 Frame Time"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                                "data-testid": "p95-frame-time",
                                                style: {
                                                    fontWeight: 600
                                                },
                                                children: [
                                                    renderMetrics.p95FrameTime.toFixed(2),
                                                    "ms"
                                                ]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        style: {
                                            display: 'flex',
                                            justifyContent: 'space-between'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                children: "P99 Frame Time"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                                "data-testid": "p99-frame-time",
                                                style: {
                                                    fontWeight: 600
                                                },
                                                children: [
                                                    renderMetrics.p99FrameTime.toFixed(2),
                                                    "ms"
                                                ]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        style: {
                                            display: 'flex',
                                            justifyContent: 'space-between'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                children: "Max Frame Time"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                                "data-testid": "max-frame-time",
                                                style: {
                                                    fontWeight: 600,
                                                    color: renderMetrics.maxFrameTime > 100 ? '#ef4444' : 'inherit'
                                                },
                                                children: [
                                                    renderMetrics.maxFrameTime.toFixed(2),
                                                    "ms"
                                                ]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("hr", {
                                        style: {
                                            border: 'none',
                                            borderTop: '1px solid #e5e7eb',
                                            margin: '4px 0'
                                        }
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        style: {
                                            display: 'flex',
                                            justifyContent: 'space-between'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                children: "Peak Memory"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                "data-testid": "peak-memory",
                                                style: {
                                                    fontWeight: 600
                                                },
                                                children: formatBytes(renderMetrics.peakMemory)
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        style: {
                                            display: 'flex',
                                            justifyContent: 'space-between'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                                children: "Memory Growth"
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                                "data-testid": "memory-growth",
                                                style: {
                                                    fontWeight: 600,
                                                    color: renderMetrics.memoryGrowthRate > 1024 * 1024 ? '#ef4444' : 'inherit'
                                                },
                                                children: [
                                                    formatBytes(Math.abs(renderMetrics.memoryGrowthRate)),
                                                    "/s"
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                style: {
                                    marginTop: '16px',
                                    padding: '12px',
                                    background: '#fff',
                                    borderRadius: '6px',
                                    border: '1px solid #e5e7eb',
                                    fontSize: '12px'
                                },
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                        style: {
                                            fontWeight: 600,
                                            marginBottom: '8px'
                                        },
                                        children: "Current Test Config"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                        style: {
                                            color: '#6b7280'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                                children: [
                                                    "Type: ",
                                                    (customConfig || TEST_PRESETS[selectedPreset]).type
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                                children: [
                                                    "Rate: ",
                                                    (customConfig || TEST_PRESETS[selectedPreset]).rate,
                                                    "/sec"
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                                children: [
                                                    "Duration: ",
                                                    (customConfig || TEST_PRESETS[selectedPreset]).duration,
                                                    "s"
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        ref: terminalRef,
                        "data-testid": "terminal-container",
                        style: {
                            border: '1px solid #374151',
                            borderRadius: '8px',
                            background: '#1e1e1e',
                            padding: '8px',
                            minHeight: '500px'
                        }
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                style: {
                    marginTop: '20px',
                    padding: '16px',
                    background: '#eff6ff',
                    borderRadius: '8px',
                    border: '1px solid #bfdbfe'
                },
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h4", {
                        style: {
                            margin: '0 0 8px 0',
                            color: '#1e40af'
                        },
                        children: "Expected Performance Targets"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("ul", {
                        style: {
                            margin: 0,
                            paddingLeft: '20px',
                            color: '#1e40af'
                        },
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                                children: "Frame time should be ≤16.67ms (60fps) for ASCII video tests"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                                children: "Frame time should be ≤33.33ms (30fps) for log flood tests"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                                children: "Memory growth should be <1MB/sec (no memory leaks)"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                                children: "No dropped frames during normal operation"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}


/***/ })

};
;