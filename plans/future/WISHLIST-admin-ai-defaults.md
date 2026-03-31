# WISHLIST: Admin-Configured AI Defaults for New Projects

**Goal:** Allow an admin to configure a default AI provider and model so that every new project is created with AI Configuration pre-filled. Users can override or remove it per-project, but the zero-config path just works.

---

## Problem

Today, every new project requires the user to manually configure an AI provider (API key, model name) before AI features (ontology extraction, glossary generation, etc.) work. For organizations deploying Ekaya Engine for business users, this is an unnecessary friction point — the admin already knows which provider and model to use.

## Proposed Solution

Add server-level AI configuration via `config.yaml` (or environment variables) that sets defaults for all new projects.

### Configuration

```yaml
ai:
  api_key: "sk-ant-..."          # or set via AI_API_KEY env var
  provider: "anthropic"          # anthropic | openai | custom
  model: "claude-sonnet-4-5-20250929"  # or set via AI_MODEL_NAME env var

  # For custom providers only:
  # api_url: "https://your-llm-gateway/v1"  # or set via AI_CUSTOM_API_URL env var
```

Environment variable mapping:

| Env Var | Purpose |
|---------|---------|
| `AI_API_KEY` | API key for the configured provider |
| `AI_PROVIDER` | `anthropic`, `openai`, or `custom` |
| `AI_MODEL_NAME` | Model identifier (e.g., `claude-sonnet-4-5-20250929`, `gpt-4o`) |
| `AI_CUSTOM_API_URL` | Base URL for custom/self-hosted providers |

### Behavior

- When a new project is created and the server has AI defaults configured, the project's AI Configuration is pre-filled with those values.
- The API key is encrypted using the existing `project_credentials_key` mechanism before storage — it is never stored in plaintext.
- Users can change or remove the AI configuration per-project at any time.
- If no server-level AI config is set, behavior is unchanged (user must configure manually).

### Quickstart and Docker Deployment

- The quickstart image could accept `AI_API_KEY` and `AI_MODEL_NAME` as environment variables for a one-line start with AI enabled.
- The deploy/docker workflow would set these in config.yaml alongside secrets and TLS.

## Open Questions

- Should the admin be able to restrict which providers/models users can configure? (Scope creep — probably not for v1.)
- Should existing projects be backfilled when an admin adds AI defaults, or only new projects?
