# Loaf Plugin Setup

After installing the plugin, you may need to install additional dependencies for full functionality.

## Language Servers (LSP)

The plugin configures LSP servers for code intelligence. Install the servers for languages you use:

### Go

```bash
# Install gopls (Go language server)
go install golang.org/x/tools/gopls@latest

# Ensure $GOPATH/bin is in your PATH
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Python

```bash
# Install pyright (recommended)
npm install -g pyright

# Or via pip
pip install pyright
```

### TypeScript / JavaScript

```bash
# Install typescript-language-server
npm install -g typescript typescript-language-server
```

### Ruby

```bash
# Install solargraph
gem install solargraph

# For Rails projects, also consider:
gem install solargraph-rails
```

## Recommended MCP Servers

MCPs are not bundled with Loaf — users configure them independently.
Run `loaf install` to see recommendations.

### Linear (Recommended)

Issue tracking integration. `loaf install` can register the standard remote; authentication (OAuth, API keys, env) is between you, Linear, and your tools — Loaf does not inject secrets.

```bash
claude mcp add linear -- npx -y mcp-remote https://mcp.linear.app/mcp
```

### Serena (Optional)

Semantic editing operations (`rename_symbol`, `replace_symbol_body`, `insert_after_symbol`) for large codebase refactoring. Most read-only code intelligence (symbol search, go-to-definition, find references) is now covered by Claude Code's native LSP.

Still valuable for non-Claude-Code targets (Cursor, Codex, etc.) that lack native LSP integration.

Requires Python 3.10+ and uv:

```bash
# Install uv if you don't have it:
curl -LsSf https://astral.sh/uv/install.sh | sh

# Add to Claude Code:
claude mcp add serena -- uvx -p 3.13 --from git+https://github.com/oraios/serena serena start-mcp-server --context claude-code --project-from-cwd
```

## Verification

After installation, verify your setup:

```bash
# Check Go LSP
gopls version

# Check Python LSP
pyright-langserver --version

# Check TypeScript LSP
typescript-language-server --version

# Check Ruby LSP
solargraph --version

# Check uv for Serena
uv --version
```

## Troubleshooting

### LSP not working

1. Ensure the binary is in your PATH
2. Restart Claude Code after installing
3. Check Claude Code logs for errors

### MCP server fails to start

1. Check Node.js version: `node --version` (need 22+)
2. Check Python/uv for Serena: `uv --version`
3. Try running the server manually to see errors

### Linear authentication

Configure auth in your environment or follow the browser OAuth flow when the MCP server starts (see Linear’s MCP docs). Loaf does not resolve `op://` references or manage API keys for you.

OAuth flow on first use:

1. A browser window will open
2. Authorize the connection
3. Return to Claude Code

To switch OAuth accounts, clear cached MCP OAuth state and reconnect:

```bash
rm -rf ~/.mcp-auth
```
