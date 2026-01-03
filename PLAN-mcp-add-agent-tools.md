# PLAN: Add Agent Tools to MCP Server Configuration

## Overview

Add a new "Agent Tools" section to the MCP Server configuration page, positioned between "Business User Tools" and "Developer Tools". This enables AI Agents to access the database safely and securely through Pre-Approved Queries only.

## UI Changes

### New Section: Agent Tools

**Location:** Between "Business User Tools" and "Developer Tools" on `/projects/{pid}/mcp-server`

**Toggle:** Independent enable/disable toggle (like other tool sections)

**Header Text:**
> Enable AI Agents to access the database safely and securely with logging and auditing capabilities. AI Agents can only use the enabled Pre-Approved Queries so that you have full control over access.

### Agent API Key Display

- 32-byte MD5 randomly generated API key
- Displayed as `****` by default (masked)
- Text box behavior:
  - Clicking in the text box reveals the full key
  - Key is auto-selected for easy manual copying
  - Supports both click-to-reveal and copy icon
- Copy icon next to the text box for clipboard copy

### Key Regeneration

- Recycle icon button next to the API key
- On click, show confirmation dialog:
  > "This will reset the API key. All previously configured Agents will fail to authenticate."
- Regenerates a new 32-byte MD5 key on confirmation

## Backend Changes

### Database

- Store Agent API Key in database encrypted with `PROJECT_CREDENTIALS_KEY`
- One Agent API Key per project at a time
- Key created automatically during project provisioning

### API Endpoints

- GET endpoint to retrieve masked/unmasked key
- POST endpoint to regenerate key

### Authentication

- Agents authenticate using the API key
- Key validation on MCP requests when Agent Tools are enabled

## Security Considerations

- Keys encrypted at rest using PROJECT_CREDENTIALS_KEY
- Audit logging for all agent API access
- Key regeneration invalidates all previous keys immediately
