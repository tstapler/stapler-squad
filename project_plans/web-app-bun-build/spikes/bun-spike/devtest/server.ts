import index from "./index.html";
Bun.serve({ port: 62876, development: { hmr: true, console: true }, routes: { "/*": index } });
console.log("up");
