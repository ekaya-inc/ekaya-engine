# PLAN: Thursday Release Assessment

## Executive Summary

Ekaya Engine has substantial production-ready infrastructure. The core MCP server, ontology extraction, pre-approved queries, and audit logging are complete. ~~The primary gap is the **AI Data Liaison configuration UI** which currently shows "Coming soon".~~ **COMPLETED 2025-01-27**: AI Data Liaison now has a functional admin checklist UI.

---

## NEW: AI Data Liaison as MCP Tool Gating Mechanism

**Key Insight (2025-01-27):** AI Data Liaison is not a separate endpoint—it's an add-on that unlocks additional MCP tools when installed.

### MCP Server WITHOUT AI Data Liaison Installed

| Tool Group | Available | Description |
|------------|-----------|-------------|
| **Developer Tools** | ✅ Yes | Full ontology management, schema, entities, relationships, columns, glossary, knowledge |
| **Agent Tools** | ✅ Yes | Limited read-only tools for automated processes |
| **User Tools** | ❌ No | Not available |

### MCP Server WITH AI Data Liaison Installed

| Tool Group | Available | Description |
|------------|-----------|-------------|
| **Developer Tools** | ✅ Yes + Query Suggestion Tools | Base tools PLUS ~8 tools for reviewing/approving query suggestions |
| **Agent Tools** | ✅ Yes | Same as without AI Data Liaison |
| **User Tools** | ✅ Yes | Business user tools: `list_approved_queries`, `execute_approved_query`, `suggest_approved_query`, `suggest_query_update`, `get_query_history`, glossary access |

### Tool Visibility Logic

```
IF datasource configured:
  Developer Tools = always visible
  Agent Tools = always visible

  IF AI Data Liaison installed:
    User Tools = visible
    Developer Tools += query suggestion management tools
```

### Why This Matters

1. **Data Engineers** install MCP Server first → get Developer + Agent tools
2. **Data Engineers** install AI Data Liaison → unlocks User Tools for business users
3. **Business Users** connect with same MCP URL → only see User Tools (based on their auth)
4. **Developer Tools** get enhanced with suggestion review workflow when AI Data Liaison is active

This means the AI Data Liaison configuration page should:
- Show the User Tools that become available
- Show the additional Developer Tools for query suggestion management
- Provide business-user-focused MCP setup instructions

---

## DONE, DONE (Production Ready)

### Core Infrastructure
| Feature | Status | Notes |
|---------|--------|-------|
| MCP Server | ✅ Complete | 25+ tools, stateless, tool filtering |
| Authentication | ✅ Complete | OAuth, JWT, API Keys, SSO |
| Datasource Management | ✅ Complete | PostgreSQL, SQL Server, encrypted credentials |
| Multi-tenancy | ✅ Complete | RLS policies, project isolation |

### Ontology System
| Feature | Status | Notes |
|---------|--------|-------|
| Ontology DAG Extraction | ✅ Complete | 10-stage pipeline, LLM enrichment |
| Entity Discovery | ✅ Complete | PK/unique constraint detection |
| Relationship Detection | ✅ Complete | FK and PK-match inference |
| Column Enrichment | ✅ Complete | Semantic roles, enum detection |
| Business Glossary | ✅ Complete | Terms, aliases, SQL definitions |
| Project Knowledge | ✅ Complete | Domain facts, business rules |
| Ontology Questions | ✅ Complete | User clarification workflow |
| Pending Changes | ✅ Complete | Review workflow for schema changes |

### Query System
| Feature | Status | Notes |
|---------|--------|-------|
| Pre-approved Queries | ✅ Complete | CRUD, parameters, execution |
| Query Suggestions | ✅ Complete | Business user → data engineer workflow |
| Query Execution | ✅ Complete | Parameter validation, injection detection |
| Query History | ✅ Complete | Execution audit trail |

### Security & Audit
| Feature | Status | Notes |
|---------|--------|-------|
| Audit Logging | ✅ Complete | Security events, query execution |
| SQL Injection Detection | ✅ Complete | libinjection fingerprinting |
| Rate Limiting | ✅ Complete | Per-user limits |

### Applications
| Feature | Status | Notes |
|---------|--------|-------|
| App Installation Framework | ✅ Complete | Install/uninstall, tool gating |
| MCP Server App | ✅ Complete | Developer tools exposure |
| AI Data Liaison Installation | ✅ Complete | Business user tools exposure |
| AI Data Liaison Config UI | ✅ Complete | Admin checklist, tool display, MCP URL sharing |

---

## Thursday Priority (Must Complete)

### P0 - Critical Path

#### 1. AI Data Liaison Configuration Page ✅ COMPLETED
**Status:** Implemented 2025-01-27
**Delivered:**
- [x] Setup checklist with status indicators (datasource, ontology, MCP URL, installation)
- [x] MCP Server URL with copy button for sharing with business users
- [x] Link to MCP Setup Instructions
- [x] Display of enabled User Tools (2) and Developer Tools (6)
- [x] Uninstall functionality with confirmation dialog

#### 2. Getting Started Checklist ✅ COMPLETED
**Status:** Implemented 2025-01-27
**Delivered:**
- [x] Step 1: Datasource configured (with status + link)
- [x] Step 2: Ontology extracted (with status + link)
- [x] Step 3: MCP Server URL ready (with status + link)
- [x] Step 4: AI Data Liaison installed (always complete on this page)
- [x] "Share with Business Users" card with MCP URL and setup instructions link

### P1 - Important for Demo

#### 3. Clarify AI Data Liaison vs MCP Server ✅ RESOLVED
**Answer documented in plan:**
- **MCP Server**: Exposes Developer Tools (ontology management) + Agent Tools
- **AI Data Liaison**: When installed, adds User Tools + query suggestion tools to Developer Tools

The AI Data Liaison page now shows:
- [x] Which tools business users get access to (User Tools section)
- [x] Which tools data engineers get for managing suggestions (Developer Tools section)
- [x] MCP URL for sharing with business users

#### 4. Query Suggestion Notification
**Status:** Deferred to post-Thursday
- [ ] Visual indicator when suggestions pending (on dashboard)
- [ ] Quick link to review suggestions

---

## Future Backlog (Talk to Sales/Support)

### Tier 1 - Near-term (Post-Thursday)
| Feature | Value | Complexity |
|---------|-------|------------|
| On-prem models (Community) | High - cost control | Medium |
| Usage analytics dashboard | Medium - insights | Medium |
| ClickHouse adapter | High - OLAP use cases | Medium |
| Webhook notifications | Medium - integration | Low |

### Tier 2 - Medium-term
| Feature | Value | Complexity |
|---------|-------|------------|
| On-prem models (Security) | High - compliance | High |
| SIEM log export | High - enterprise | Medium |
| Team workspaces | Medium - organization | Medium |
| Auto-approval rules | Medium - efficiency | Medium |

### Tier 3 - Long-term
| Feature | Value | Complexity |
|---------|-------|------------|
| Cross-datasource joins | High - flexibility | High |
| Query scheduling | Medium - automation | Medium |
| Dynamic RLS | High - security | High |
| Embedded analytics | Medium - distribution | High |

---

## Risk Assessment

### Thursday Delivery Risks

1. **UI Configuration State Management**
   - Risk: Configuration not persisting correctly
   - Mitigation: Leverage existing `engine_installed_apps.settings` JSONB column

2. **MCP Config Generation**
   - Risk: Config format incompatible with Claude Desktop
   - Mitigation: Test with actual Claude Desktop before release

3. **API Key Flow**
   - Risk: Key generation UX unclear
   - Mitigation: Follow standard API key patterns (show once, copy button)

### Go-Live Readiness

| Requirement | Status |
|-------------|--------|
| Datasource connection works | ✅ |
| Ontology extraction runs | ✅ |
| MCP tools respond | ✅ |
| Queries execute | ✅ |
| Audit logs capture | ✅ |
| User can authenticate | ✅ |
| Business user can suggest queries | ✅ |
| Data engineer can approve queries | ✅ |
| Configuration UI works | ⏳ Thursday |

---

## Definition of Done for Thursday

1. Customer can install AI Data Liaison from Applications page
2. Customer sees configuration options (not "Coming soon")
3. Customer can save configuration
4. Customer can generate MCP config for Claude Desktop
5. Customer can generate Agent API Key for MCP access
6. Business user can connect Claude Desktop to Ekaya
7. Business user can execute pre-approved queries
8. Business user can suggest new queries
9. Data engineer can approve/reject suggestions via MCP tools

---

## What We're NOT Shipping Thursday

- On-prem model deployment
- SIEM integration
- Usage analytics dashboard
- Team workspaces
- Auto-approval rules
- ClickHouse support
- Advanced RLS

These become "Contact sales" or "Contact support" conversations.
