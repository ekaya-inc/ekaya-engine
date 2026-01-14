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

## Success Metrics

- Claude Code can answer "what was I working on?" accurately
- Semantic search returns relevant results without exact keywords
- Session startup feels continuous, not cold
- Decisions are traceable to rationale
- Team member context is always available
