# CockroachDB Managed MCP — add to Claude Code / Cursor / VS Code

This is the official CockroachDB Cloud MCP endpoint. StillMind exposes its memory tables through it for live inspection during the demo.

```json
{
  "mcpServers": {
    "cockroachdb": {
      "type": "http",
      "url": "https://cockroachlabs.cloud/mcp"
    }
  }
}
```

## What you can ask the demo MCP-connected agent

- "List tables in the stillmind database."
- "Show the schema of `notes`."
- "Run `EXPLAIN` on the last vector query."
- "How many rows are in `compactions` today?"
- "Run `SHOW RANGES FROM TABLE notes` and summarize the replication footprint."

The MCP server is read-only by default. No custom proxy required.
