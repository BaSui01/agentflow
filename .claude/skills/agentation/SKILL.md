---
name: agentation
description: Add Agentation visual feedback toolbar to a Next.js project, and configure MCP server for Codex CLI (preferred) or Claude Code
---

# Agentation Setup

Set up the Agentation annotation toolbar in this project.

## Steps

1. **Check if already installed**
   - Look for `agentation` in package.json dependencies
   - If not found, run `npm install agentation` (or pnpm/yarn based on lockfile)

2. **Check if already configured**
   - Search for `<Agentation` or `import { Agentation }` in src/ or app/
   - If found, report that Agentation is already set up and exit

3. **Detect framework**
   - Next.js App Router: has `app/layout.tsx` or `app/layout.js`
   - Next.js Pages Router: has `pages/_app.tsx` or `pages/_app.js`

4. **Add the component**

   For Next.js App Router, add to the root layout:
   ```tsx
   import { Agentation } from "agentation";

   // Add inside the body, after children:
   {process.env.NODE_ENV === "development" && <Agentation />}
   ```

   For Next.js Pages Router, add to _app:
   ```tsx
   import { Agentation } from "agentation";

   // Add after Component:
   {process.env.NODE_ENV === "development" && <Agentation />}
   ```

5. **Confirm component setup**
   - Tell the user the Agentation toolbar component is configured

6. **Detect MCP config target (Codex first)**
   - If `~/.codex/config.toml` exists, configure Codex CLI MCP first.
   - Otherwise, if `~/.claude/claude_code_config.json` exists, configure Claude Code MCP.
   - If both exist, prefer configuring both to keep behavior consistent.

7. **Configure MCP server**
   - For Codex CLI (`~/.codex/config.toml`), add or merge:
     ```toml
     [mcp_servers.agentation]
     type = "stdio"
     command = "npx"
     args = ["-y", "agentation-mcp", "server"]
     startup_timeout_sec = 60
     ```
   - For Claude Code (`~/.claude/claude_code_config.json`), add or merge:
     ```json
     {
       "mcpServers": {
         "agentation": {
           "command": "npx",
           "args": ["agentation-mcp", "server"]
         }
       }
     }
     ```
   - Preserve existing MCP entries in either config file.

8. **Confirm full setup**
   - Tell the user both layers are set up:
     - React component for toolbar (`<Agentation />`)
     - MCP server auto-start entry for configured client(s)
   - Tell user to restart the corresponding client (`Codex` and/or `Claude Code`) to load the MCP server
   - Explain that annotations can now sync through MCP

## Notes

- The `NODE_ENV` check ensures Agentation only loads in development
- Agentation requires React 18
- The MCP server auto-starts when Codex/Claude launches (uses npx, no global install needed)
- Port 4747 is used by default for the HTTP server
- Run `npx agentation-mcp doctor` to verify setup
