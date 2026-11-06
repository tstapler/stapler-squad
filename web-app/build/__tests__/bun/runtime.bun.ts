// Proves runInBun really executes under Bun, not Node.
console.log(JSON.stringify({ runtime: typeof Bun !== "undefined" }));
