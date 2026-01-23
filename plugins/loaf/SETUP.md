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

## MCP Servers

MCP servers are automatically started when needed. Prerequisites:

### Sequential Thinking

```bash
# Requires Node.js 18+
# No additional installation - runs via npx
```

### Linear

```bash
# Requires Node.js 18+
# No additional installation - runs via npx
# You'll be prompted to authenticate with Linear on first use
```

### Serena

```bash
# Requires Python 3.10+ and uv
# Install uv if you don't have it:
curl -LsSf https://astral.sh/uv/install.sh | sh

# No additional installation - runs via uvx
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

1. Check Node.js version: `node --version` (need 18+)
2. Check Python/uv for Serena: `uv --version`
3. Try running the server manually to see errors

### Linear authentication

Linear uses OAuth. On first use:
1. A browser window will open
2. Authorize the connection
3. Return to Claude Code
