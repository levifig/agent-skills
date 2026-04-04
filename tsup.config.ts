import { defineConfig } from "tsup";

export default defineConfig({
  entry: ["cli/index.ts"],
  format: ["esm"],
  target: "node22",
  outDir: "dist-cli",
  clean: true,
  sourcemap: true,
  dts: false,
  shims: false,
  splitting: false,
  noExternal: ["commander", "gray-matter", "yaml", "picomatch"],
  esbuildOptions(options) {
    // When bundling CJS packages (commander) into ESM, esbuild's __require shim
    // can't resolve Node builtins. Mark them as external so Node resolves them at runtime.
    options.external = [
      ...(options.external || []),
      "events", "fs", "path", "crypto", "child_process", "os", "url",
      "readline", "stream", "util", "buffer", "string_decoder", "process",
      "tty", "module", "assert", "net", "http", "https", "zlib",
    ];
  },
  banner: {
    js: `#!/usr/bin/env node
import { createRequire as __createRequire } from 'module';
const require = __createRequire(import.meta.url);`,
  },
});
