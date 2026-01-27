# DESIGN: AI Data Liaison

## Vision

AI Data Liaison is Ekaya's enterprise self-service analytics feature that enables business users to query their organization's data through natural language while maintaining data governance controls. It bridges the gap between business teams who need insights and data teams who manage data access.

**Core Value Proposition:**
- Business users get 10x faster response times for ad-hoc queries
- Data teams reduce ad-hoc request burden by 50%+
- System becomes smarter through usage (ontology refinement)
- Full audit trail for compliance

## Target Users

### Business Users (Consumers)
- Marketing, Sales, Product, Operations, Support, Analytics teams
- Use Claude Desktop, Cowork, or Claude Code as their chat interface
- Express data needs in natural language
- Consume pre-approved queries and request new ones

### Data Engineers (Administrators)
- Configure datasources and ontology
- Review and approve/reject query suggestions
- Refine ontology based on usage patterns
- Monitor audit trails

### AI Agents (Automated Processes)
- Use MCP tools programmatically
- Execute pre-approved queries within guardrails
- Part of automated workflows and pipelines

## User Journey

### Setup Flow (Data Engineer)
1. Create project in Ekaya
2. Connect datasource (PostgreSQL, SQL Server, ClickHouse)
3. Run ontology extraction (automatic semantic understanding)
4. Review and refine extracted ontology (entities, relationships, glossary)
5. Install "AI Data Liaison" application
6. Generate MCP configuration for distribution to users
7. Share MCP config with business users

### Daily Flow (Business User)
1. Open Claude Desktop/Cowork/Code with MCP configured
2. Ask natural language question about data
3. AI translates to SQL using ontology context
4. One of two paths:
   - **Pre-approved query exists**: Execute immediately, return results
   - **New query needed**: Suggest query for approval, explain wait
5. Results returned with source attribution
6. Usage contributes to ontology refinement

### Governance Flow (Data Engineer)
1. Review pending query suggestions
2. Approve or reject with feedback
3. Monitor query execution audit logs
4. Refine ontology based on patterns observed
5. Add glossary terms for common business concepts

## Architecture

### AI Data Liaison as Tool Gating Mechanism

AI Data Liaison is not a separate MCP endpoint—it's an **add-on application** that unlocks additional MCP tools when installed on a project.

**Without AI Data Liaison:**
- Developer Tools: Full ontology management
- Agent Tools: Limited read-only tools
- User Tools: **Not available**

**With AI Data Liaison Installed:**
- Developer Tools: Base tools **+ query suggestion management** (~8 tools)
- Agent Tools: Same as before
- User Tools: **Now available** for business users

### MCP Tool Exposure

**User Tools** (requires AI Data Liaison):
- `get_context` - Database context with semantic information
- `get_schema` - Schema with entity/role annotations
- `list_glossary` - Business term definitions
- `get_glossary_sql` - SQL for specific terms
- `list_approved_queries` - Available pre-approved queries
- `execute_approved_query` - Run approved query with parameters
- `get_query_history` - Recent query executions
- `suggest_approved_query` - Propose new query for approval
- `suggest_query_update` - Propose updates to existing queries

**Developer Tools - Query Suggestion Management** (requires AI Data Liaison):
- `list_query_suggestions` - View pending suggestions
- `approve_query_suggestion` - Approve and activate a suggestion
- `reject_query_suggestion` - Reject with feedback
- `create_approved_query` - Direct creation (bypass suggestion)
- `update_approved_query` - Direct update
- `delete_approved_query` - Remove query

**Developer Tools - Base** (MCP Server only):
- Full ontology management (entities, relationships, columns)
- Pending change review
- Ontology question management
- Schema and glossary management

### Security Model

1. **Authentication**: OAuth SSO or Agent API Keys
2. **Authorization**: Project-scoped access via RLS policies
3. **Query Execution**: Only pre-approved queries or suggestions go through
4. **Audit Trail**: All query executions logged with user attribution
5. **SQL Injection Protection**: libinjection scanning on all queries

### Ontology Feedback Loop

The system becomes "indispensable" through continuous learning:

1. **Query Suggestions**: Business users suggest queries data team hadn't anticipated
2. **Execution Patterns**: Popular queries inform glossary terms
3. **Failed Queries**: Reveal ontology gaps (missing relationships, unclear terminology)
4. **Refinement Chat**: Data engineers can chat to refine ontology
5. **Project Knowledge**: Domain facts captured during refinement persist

## Feature Status

### DONE, DONE (Production Ready)
- MCP Server with full tool set
- Ontology extraction DAG (10-stage pipeline)
- Pre-approved query workflow (suggest → approve → execute)
- Query execution audit logging
- OAuth/JWT/API Key authentication
- Datasource management (PostgreSQL, SQL Server)
- Business glossary with SQL definitions
- Project knowledge storage
- Application installation framework
- Tool visibility gating

### Thursday Priority (Must Ship)
1. **AI Data Liaison Configuration UI** - Replace "Coming soon" placeholder
2. **MCP Config Generator** - One-click config generation for users
3. **Agent API Key UI** - Easy key generation for business users
4. **Documentation** - Setup guide for business users

### Future Backlog (Talk to Sales/Support)
1. **On-Prem Models** - Fine-tuned Qwen3 models (community/security tiers)
2. **SIEM Integration** - Export audit logs to external systems
3. **Team Workspaces** - Department-specific query libraries
4. **Usage Analytics Dashboard** - Query patterns, popular queries, gaps
5. **Automated Query Approval** - Rules-based auto-approval for low-risk queries
6. **ClickHouse Adapter** - OLAP database support
7. **Advanced RLS** - Dynamic row filtering based on user attributes
8. **Query Scheduling** - Recurring query execution

## Configuration Options

The AI Data Liaison configuration page should allow:

### Query Behavior
- **Auto-suggest mode**: Automatically suggest queries or wait for explicit request
- **Result row limit**: Maximum rows returned (default: 100)
- **Query timeout**: Maximum execution time (default: 30s)

### Audit Settings
- **Log read queries**: Whether to audit SELECT queries (can be high volume)
- **Log parameters**: Include query parameters in audit logs

### Access Control
- **Require approval for new queries**: Toggle approval workflow
- **Allow direct execution of SELECT**: Trust business users for read-only

### MCP Client Configuration
- **Generated config snippet**: Copy-paste for Claude Desktop
- **API Key management**: Generate/revoke agent keys
- **Webhook URL**: For query approval notifications

## Success Metrics

1. **Time to Insight**: Business users get answers in seconds vs days
2. **Request Reduction**: Fewer ad-hoc tickets to data team
3. **Query Reuse**: High ratio of approved query executions to suggestions
4. **Ontology Completeness**: Decreasing failed query rate over time
5. **User Adoption**: Number of unique users executing queries weekly

## Out of Scope (V1)

- Real-time streaming queries
- Write operations beyond pre-approved procedures
- Cross-datasource joins
- Custom visualization/charting
- Natural language query building UI (relies on MCP client)
- Embedded analytics (iframe/widget)

## Dependencies

- Claude Desktop, Cowork, or Claude Code as MCP client
- Valid datasource connection with schema access
- Ontology extraction completed (at least entity discovery)
- OAuth configured for user authentication
