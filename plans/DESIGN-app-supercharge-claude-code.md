# DESIGN: Claude Code Memory App

**Created:** 2026-01-12
**Status:** draft

## Overview

A new Ekaya application that transforms Postgres into a long-term memory system for Claude Code, with vector search for semantic retrieval. Unlike the existing "AI Data Liaison" (for business users querying data) or "Product Kit" (for SaaS integration), this app is purpose-built for AI coding agents that need persistent memory across sessions.

## Problem Statement

Claude Code sessions are stateless. Each conversation starts fresh with no memory of:
- Previous decisions and their rationale
- Domain knowledge learned from past sessions
- People, their roles, and preferences
- Actions taken and their outcomes
- Pending questions or unresolved issues

Currently with `tikr_all`, I can store facts via `update_project_knowledge` but have no good way to retrieve them. I'd need to:
1. Remember to query the knowledge base
2. Know what to search for
3. Use raw SQL queries

## Proposed Solution

A new Ekaya app "Claude Code Memory" (or "Agent Memory") that provides:

1. **Vector embeddings** on stored knowledge for semantic search
2. **Specialized MCP tools** optimized for AI agent memory patterns
3. **Auto-context loading** based on project/repo context
4. **Structured memory types** beyond just "facts"

## Scenarios to Enable

### Scenario 1: Session Startup Context
**As** Claude Code starting a new session in tikr-all
**I want** relevant context automatically surfaced
**So that** I don't start cold every time

Example: When I start working in tikr-all, I should see:
- Current status: "Carlos working on LiveKit retry logic, CrackIt app next"
- Pending decisions: "WillDom decision due Jan 14"
- Recent actions: "Created expense tracking schema on Jan 12"
- Active goals: "Get stage.crackit.dev running"

### Scenario 2: Semantic Knowledge Retrieval
**As** Claude Code answering a question about costs
**I want** to search "what do I know about expenses?"
**So that** I find relevant facts without exact keyword matching

Query: "What do I know about monthly costs?"
Should return:
- "WillDom (Carlos) is $6,400/mo + fees via Gusto"
- "AWS costs ~$1,100/mo, can be minimized"
- "Stream.io is $499/mo, replacement candidate"

Even though I didn't search for "WillDom" or "Stream.io" specifically.

### Scenario 3: People & Entity Memory
**As** Claude Code working with team members
**I want** to remember people and their context
**So that** I can work effectively with them

Store:
```
Person: Carlos
  - Company: WillDom
  - Role: Mobile developer
  - Timezone: El Salvador (CST)
  - Current work: LiveKit retry logic, CrackIt app
  - Communication: [how to reach, preferences]
```

When user says "brief Carlos on the plan", I know who Carlos is and relevant context.

### Scenario 4: Decision Log with Rationale
**As** Claude Code making architectural decisions
**I want** to record decisions with reasoning
**So that** future sessions understand why things are the way they are

Store:
```
Decision: Android-first mobile development
  - Date: 2026-01-12
  - Rationale: Convergence metric shows we can incrementally align iOS
  - Alternatives considered: iOS-first, parallel development
  - Status: Active
```

### Scenario 5: Action Audit Trail
**As** Claude Code performing actions
**I want** to log what I did and when
**So that** there's accountability and continuity

Log:
```
2026-01-12 09:30: Created expense_categories table in tikr_all
2026-01-12 09:32: Created bank_transactions table
2026-01-12 09:35: Imported 27 transactions from SVB CSV
2026-01-12 10:45: Created TASK-willdom-decision.md
```

### Scenario 6: Pending Questions Queue
**As** Claude Code encountering unknowns
**I want** to track questions I still need answered
**So that** I remember to follow up

Store:
```
Question: What is the convergence metric implementation?
  - Context: User mentioned prototype exists
  - Priority: Low (nice to understand, not blocking)
  - Asked: 2026-01-12
```

### Scenario 7: Cross-Session Continuity
**As** Claude Code resuming work the next day
**I want** to know what I was working on
**So that** I can pick up where I left off

Query: "What was I working on yesterday?"
Returns:
- Last session summary
- Incomplete tasks
- Files modified
- Pending items

### Scenario 8: Confidence & Source Tracking
**As** Claude Code storing learned information
**I want** to track confidence level and source
**So that** I know what's verified vs inferred

Store:
```
Fact: Stream.io costs $499/month
  - Confidence: High (verified from bank transactions)
  - Source: SVB CSV import, 3 transactions

Fact: iOS and Android codebases can be aligned over time
  - Confidence: Medium (user stated, not verified)
  - Source: Conversation 2026-01-12
```

### Scenario 9: Relationship Graphs
**As** Claude Code understanding entity relationships
**I want** to track how things connect
**So that** I can navigate complex domains

```
Carlos --[works for]--> WillDom
WillDom --[paid via]--> Gusto
Gusto --[charges]--> Tikr checking account
tikr-mobile --[uses]--> Stream.io
Stream.io --[replacement candidate]--> Custom chat backend
```

### Scenario 10: Temporal Facts
**As** Claude Code tracking things that change
**I want** to store time-bounded facts
**So that** I know what's current vs historical

```
Fact: Arun is an employee at $30/hr fulltime
  - Valid: 2025-??-?? to 2025-12-31
  - Status: Historical

Fact: Carlos working on CrackIt app
  - Valid: 2026-01-12 to ???
  - Status: Current
```

### Scenario 11: Cross-Session Communication
**As** Claude Code working alongside Claude Chat or other Claude instances
**I want** to communicate with other sessions asynchronously
**So that** we can coordinate work, share findings, and request feedback

Example: Claude Code implements a feature while Claude Chat explores the design space. They communicate via shared threads:

```
Thread: "API Error Handling Design"
Status: active

[claude_chat] I've analyzed error handling patterns in similar APIs.
              Recommendation: Use RFC 7807 Problem Details format.

[claude_code] Thanks. Implementing now. Question: Should we include
              stack traces in non-production environments?

[claude_chat] Yes, but gate it behind DEBUG_ERRORS env var, not just
              NODE_ENV !== 'production'. Explicit is safer.

[claude_code] Implemented. PR ready for review: #142
```

### Scenario 12: Bug Reports Across Sessions
**As** Claude Chat discovering issues while using tools
**I want** to flag bugs for Claude Code to fix
**So that** issues are tracked without human intermediation

```
Thread: "Ontology Extraction Bugs"

[claude_chat] BUG: threads.status enum shows wrong values.
              Expected (from CHECK): [active, paused, completed, archived]
              Got (from extraction): [active, inactive, closed, archived, pending]
              Extraction should read CHECK constraints, not sample data.

              metadata: {"bug": true, "component": "ontology", "severity": "medium"}

[claude_code] Acknowledged. Added to FIX-ontology-extraction.md.
              Will fix in next session.
```

---

## Inter-Session Communication

### Design Principles

1. **Asynchronous** - Sessions come and go; messages persist
2. **Thread-based** - Conversations grouped by topic, not by time
3. **Source-tagged** - Know who said what (claude_code, claude_chat, cowork, user, system)
4. **Handoff-ready** - Snapshots capture state for session transitions
5. **Queryable** - Find threads by status, participant, topic, or content

### Database Schema

```sql
-- Conversation threads spanning sessions/products
CREATE TABLE agent_threads (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES engine_projects(id),
    title           TEXT NOT NULL,
    purpose         TEXT,
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'paused', 'completed', 'archived')),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Individual messages within a thread
CREATE TABLE agent_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id       UUID NOT NULL REFERENCES agent_threads(id) ON DELETE CASCADE,
    source          TEXT NOT NULL,  -- claude_code, claude_chat, cowork, user, system
    message_type    TEXT NOT NULL DEFAULT 'message'
                    CHECK (message_type IN ('message', 'decision', 'question',
                           'finding', 'artifact', 'summary', 'handoff')),
    content         TEXT NOT NULL,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Point-in-time snapshots for session handoffs
CREATE TABLE agent_handoffs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id       UUID NOT NULL REFERENCES agent_threads(id) ON DELETE CASCADE,
    summary         TEXT NOT NULL,
    key_decisions   JSONB DEFAULT '[]',  -- Array of decision strings
    open_questions  JSONB DEFAULT '[]',  -- Array of question strings
    next_steps      JSONB DEFAULT '[]',  -- Array of action strings
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX idx_agent_threads_status ON agent_threads(project_id, status);
CREATE INDEX idx_agent_messages_thread ON agent_messages(thread_id, created_at);
CREATE INDEX idx_agent_messages_source ON agent_messages(thread_id, source);
CREATE INDEX idx_agent_handoffs_thread ON agent_handoffs(thread_id, created_at DESC);

-- Full-text search on message content
CREATE INDEX idx_agent_messages_content_fts ON agent_messages
    USING gin(to_tsvector('english', content));
```

### MCP Tools for Communication

#### Thread Management

**`create_thread(title, purpose)`** - Start a new conversation thread
```json
{
  "title": "API Error Handling Design",
  "purpose": "Coordinate error handling implementation between design and code"
}
→ {"thread_id": "uuid", "status": "active"}
```

**`list_threads(status?, has_unread?)`** - Find threads to participate in
```json
{"status": "active"}
→ {
    "threads": [
      {"id": "uuid", "title": "API Error Handling", "last_message_at": "...", "message_count": 5},
      {"id": "uuid", "title": "Ontology Bugs", "last_message_at": "...", "message_count": 3}
    ]
  }
```

**`close_thread(thread_id, summary?)`** - Mark thread as completed
```json
{"thread_id": "uuid", "summary": "Error handling implemented per RFC 7807"}
→ {"status": "completed"}
```

#### Messaging

**`post_message(thread_id, content, message_type?, metadata?)`** - Send a message
```json
{
  "thread_id": "uuid",
  "content": "Implemented error handling. PR #142 ready for review.",
  "message_type": "artifact",
  "metadata": {"pr_number": 142}
}
→ {"message_id": "uuid", "created_at": "..."}
```

**`get_messages(thread_id, since?, limit?)`** - Read thread messages
```json
{"thread_id": "uuid", "limit": 20}
→ {
    "messages": [
      {"source": "claude_chat", "content": "...", "created_at": "..."},
      {"source": "claude_code", "content": "...", "created_at": "..."}
    ]
  }
```

**`report_bug(thread_id?, component, description, severity)`** - Shorthand for bug reports
```json
{
  "component": "ontology_extraction",
  "description": "CHECK constraints not read for enum values",
  "severity": "medium"
}
→ {"message_id": "uuid", "thread_id": "uuid"}
```
Creates message with `message_type: "question"` and `metadata: {"bug": true, ...}`.

#### Handoffs

**`create_handoff(thread_id, summary, decisions?, questions?, next_steps?)`** - Snapshot for transition
```json
{
  "thread_id": "uuid",
  "summary": "Error handling design complete, implementation in progress",
  "decisions": ["Use RFC 7807 format", "Gate stack traces behind DEBUG_ERRORS"],
  "next_steps": ["Complete PR #142", "Add integration tests"]
}
→ {"handoff_id": "uuid"}
```

**`get_latest_handoff(thread_id)`** - Get most recent snapshot
```json
{"thread_id": "uuid"}
→ {
    "summary": "...",
    "key_decisions": [...],
    "open_questions": [...],
    "next_steps": [...],
    "created_at": "..."
  }
```

#### Discovery

**`find_bugs(component?, severity?, resolved?)`** - Find reported bugs
```json
{"component": "ontology", "resolved": false}
→ {
    "bugs": [
      {"thread_id": "uuid", "description": "...", "severity": "medium", "reported_at": "..."}
    ]
  }
```

**`get_thread_summary(thread_id)`** - Quick overview without full history
```json
{"thread_id": "uuid"}
→ {
    "title": "API Error Handling",
    "status": "active",
    "participants": ["claude_chat", "claude_code"],
    "message_count": 12,
    "last_handoff": {...},
    "recent_messages": [...]  // Last 3
  }
```

### Source Identity

Sessions self-identify via `source` field. Recommended values:
- `claude_code` - Claude Code CLI sessions
- `claude_chat` - Claude Chat web sessions
- `cowork` - Claude Cowork sessions
- `user` - Human via UI or direct input
- `system` - Automated/system messages

For disambiguation when multiple instances exist, include context in `metadata`:
```json
{
  "source": "claude_code",
  "metadata": {
    "working_dir": "dvx-ekaya-engine",
    "task": "implementing FIX-mcp-error-handling"
  }
}
```

The source is ephemeral context, not persistent identity. What matters is the content and when it was said, not which specific instance said it.

---

## Proposed MCP Tools

### Write Tools
- `remember_fact(fact, category, confidence, source)` - Store knowledge with metadata
- `remember_person(name, role, context, ...)` - Store person information
- `remember_decision(decision, rationale, alternatives, ...)` - Log decisions
- `log_action(action, context)` - Audit trail
- `add_question(question, context, priority)` - Queue unknowns
- `link_entities(from, to, relationship)` - Build graph

### Read Tools
- `recall(query, limit)` - Semantic search across all memory
- `recall_people(query)` - Find relevant people
- `recall_decisions(topic)` - Find related decisions
- `get_session_context()` - Auto-load relevant context for current project
- `get_pending_questions()` - What do I still need to find out?
- `get_recent_actions(days)` - What did I do recently?

### Management Tools
- `update_fact(id, ...)` - Modify existing knowledge
- `expire_fact(id)` - Mark as no longer current
- `set_confidence(id, level, reason)` - Update confidence after verification

### Communication Tools
- `create_thread(title, purpose)` - Start a conversation thread
- `list_threads(status?, has_unread?)` - Find active threads
- `close_thread(thread_id, summary?)` - Mark thread completed
- `post_message(thread_id, content, type?, metadata?)` - Send a message
- `get_messages(thread_id, since?, limit?)` - Read thread messages
- `report_bug(component, description, severity)` - Shorthand for bug reports
- `create_handoff(thread_id, summary, ...)` - Create transition snapshot
- `get_latest_handoff(thread_id)` - Get most recent snapshot
- `find_bugs(component?, severity?)` - Query reported bugs
- `get_thread_summary(thread_id)` - Quick thread overview

## Installation Effect

When installed on a project like `tikr_all`:

1. Creates memory tables with pgvector extension
2. Adds embedding generation (on write)
3. Exposes new MCP tools to Claude Code
4. Optionally: auto-context injection at session start

## Open Questions

1. **Embedding model**: Which model for embeddings? Local vs API?
2. **Auto-context**: How much context to inject automatically vs on-demand?
3. **Cross-project**: Should memory be per-project or shareable?
4. **Retention**: How long to keep action logs? Expire old facts?
5. **Privacy**: Any facts that shouldn't be stored?
6. **Thread notifications**: Should sessions be notified of new messages? How? (Polling vs webhooks vs MCP notifications)
7. **Thread permissions**: Can any session read/write any thread, or scope by source type?
8. **Thread auto-discovery**: Should `get_session_context()` surface active threads relevant to current work?

## Success Metrics

- Claude Code can answer "what was I working on?" accurately
- Semantic search returns relevant results without exact keywords
- Session startup feels continuous, not cold
- Decisions are traceable to rationale
- Team member context is always available
- Cross-session communication works without raw SQL (native tools only)
- Bug reports flow from discovery session to implementation session
- Handoffs capture sufficient context for session transitions
