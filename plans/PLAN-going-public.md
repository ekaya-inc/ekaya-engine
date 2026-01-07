# PLAN: Going Public Checklist

Pre-release checklist for transitioning ekaya-engine from private to public repository.

---

## Critical: Must Fix Before Going Public

### 1. Rotate Compromised Secrets

**Status:** ACTION REQUIRED

The `.env` file in the working directory contains real secrets. While `.env` is properly gitignored and NOT tracked in git history, these secrets should be considered potentially compromised once the repo goes public (someone may have cloned it):

| Secret | Action |
|--------|--------|
| `ANTHROPIC_API_KEY` | Regenerate in Anthropic console immediately after going public |
| `PROJECT_CREDENTIALS_KEY` | Regenerate - this encrypts datasource credentials in the database |
| `PGPASSWORD` | If this is a production password, rotate it |

**Steps:**
1. Go to https://console.anthropic.com and regenerate the API key
2. Generate new `PROJECT_CREDENTIALS_KEY`: `openssl rand -base64 32`
3. Re-encrypt any existing datasource credentials in production (or accept they'll need to be re-entered)

### 2. Add Copyright Notice to LICENSE

**Status:** ACTION REQUIRED

The Apache 2.0 LICENSE file exists but uses placeholder text:
```
Copyright [yyyy] [name of copyright owner]
```

**Fix:** Replace with actual copyright:
```
Copyright 2025 Ekaya, Inc.
```

---

## Recommended: Clean Up Before Going Public

### 3. Abstract Internal Infrastructure References

**Status:** RECOMMENDED

Several files reference internal Ekaya infrastructure hostnames. These expose internal architecture but aren't security risks:

| File | References | Recommendation |
|------|------------|----------------|
| `config/config.*.yaml` | `sparkone:30000`, `sparktwo:30001` | Replace with placeholder comments or remove |
| `PLAN-*.md` files | Internal model hostnames | Remove or generalize |
| `Makefile:6-7` | `us-central1-docker.pkg.dev/ekaya-dev-shared/...` | Add comment that these are Ekaya's internal registries |
| `pkg/testhelpers/containers.go:23` | Same registry URL | Document or make configurable |
| `test/docker/engine-test-db/README.md` | Same registry URL | Document as internal |

**Option A (Minimal):** Add comments explaining these are internal Ekaya infrastructure
**Option B (Clean):** Extract to environment variables or remove from committed configs

### 4. Review PLAN/FIX/DESIGN Files

**Status:** DECISION NEEDED

22 PLAN files, 2 FIX files, and 1 DESIGN file contain internal product roadmap and architecture details:

| Keep | Remove/Redact |
|------|---------------|
| Useful for contributors understanding direction | May reveal competitive information |
| Shows mature project planning | Internal hostname references |
| Demonstrates thoughtful architecture | Some contain specific model names/configs |

**Files with most internal detail:**
- `PLAN-text2sql-implementation.md` - References `sparktwo:30001`, internal embedding API
- `PLAN-claudes-wish-list.md` - Internal feature wishlist with token costs
- `PLAN-dvx-add-ontology-errors.md` - References internal model at `sparkone:30000`

**Recommendation:** Quick search-replace of `sparkone`/`sparktwo` with `<embedding-host>` placeholders is sufficient.

### 5. Add Community Files

**Status:** RECOMMENDED

Missing standard open-source community files:

| File | Purpose | Priority |
|------|---------|----------|
| `CONTRIBUTING.md` | How to contribute | High |
| `SECURITY.md` | Security vulnerability reporting | High |
| `CODE_OF_CONDUCT.md` | Community standards | Medium |

### 6. README Enhancements

**Status:** OPTIONAL

Current README is functional but minimal. Consider adding:
- Brief description of what Ekaya Engine does (current: just "Regional controller")
- Architecture overview or link to docs
- Screenshots/demo
- Badge for build status, license
- Link to hosted demo or docs site

---

## Already Safe: No Action Required

### Git History
- **Clean** - No API keys or passwords found in git history
- **`.env` never committed** - Verified with `git ls-files .env` (empty)
- Git history only contains development passwords in docker-compose (expected)

### Dependencies
- **All public packages** - No private GitHub dependencies in `go.mod`
- **npm packages all public** - Standard React/Vite/Tailwind stack

### Source Code
- **No hardcoded secrets** - All credentials loaded from environment variables
- **Proper secret handling** - `yaml:"-"` tags prevent secrets from being in config files

### Configuration Templates
- **Safe** - `config/*.yaml` files contain only URLs, not secrets
- **`.env.example`** - Properly empty/placeholder values

### CI/CD Workflows
- **Safe** - No secrets in `.github/workflows/*.yml`
- **Uses GitHub secrets** - Deployment credentials not in code

### Test Data
- **`test_data.dump`** - Contains trimmed test database (259KB)
- **No real customer data** - Synthetic test data only

### Docker Configuration
- **Safe** - `Dockerfile` uses multi-stage build, non-root user
- **`docker-compose.dev.yml`** - Uses `localdev` password (expected for local dev)

---

## Quick Command Checklist

```bash
# 1. Search and verify no secrets in tracked files
git grep -i "sk-ant-api" --cached
git grep -i "POSTGRES_PASSWORD=" --cached  # Should only find docker-compose, .env.example

# 2. Verify .env not tracked
git ls-files .env  # Should be empty

# 3. After going public, regenerate:
#    - Anthropic API key at console.anthropic.com
#    - PROJECT_CREDENTIALS_KEY with: openssl rand -base64 32
```

---

## Decision: What to Do with CLAUDE.md

The `CLAUDE.md` file contains detailed development instructions including:
- Database schema details
- Manual testing procedures
- Internal debugging commands
- DAG workflow documentation

**Options:**
1. **Keep as-is** - Useful for contributors, nothing secret
2. **Trim** - Remove internal debugging sections, keep architecture guidance
3. **Split** - Keep basic CLAUDE.md, move detailed procedures to docs/

**Recommendation:** Keep as-is. It's valuable documentation for contributors and contains no secrets.

---

## Summary

| Category | Items | Action |
|----------|-------|--------|
| Critical | 2 | Rotate secrets, fix LICENSE copyright |
| Recommended | 4 | Internal refs, PLAN files, community files, README |
| Safe | 7 | No action needed |

**Estimated effort:** 1-2 hours for critical items, 2-4 hours for recommended cleanup.
