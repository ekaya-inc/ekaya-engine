# PLAN: Setup Wizard Shared Surfaces

**Status:** Open  
**Branch:** follow-up to setup-state + shared wizard rollout  
**Parent Plan:** `plans/PLAN-setup-wizard-consolidation.md`

## Context

The setup-state and wizard consolidation work is now in place:

- `/:pid/setup` is the canonical setup entry point
- setup state is persisted in `engine_projects.parameters.setup_steps`
- the project dashboard can show a `[Setup]` button with a badge from one request
- setup ownership has been moved out of page-level checklist cards
- `/setup-status` is live and wired into the dashboard and wizard
- the wizard can embed the datasource flow directly and can trigger inline app activation
- app pages and the wizard now share setup state semantics, but they still largely render separate UI implementations

That still leaves an important gap: the consolidated wizard centralizes ordering and status, but most non-datasource work still sends users to page-specific management surfaces.

The next step is to refactor the setup-capable parts of the application pages into shared surfaces so:

- the Setup Wizard can render them inline
- the app pages can render the same logic in maintenance mode
- setup behavior and maintenance behavior stay aligned

## Goals

- Keep users inside the Setup Wizard for most or all setup work.
- Reuse the same components and logic for setup and maintenance.
- Eliminate duplicate fetch/mutation logic between wizard steps and app pages.
- Make each step behave correctly whether the project is incomplete, partially configured, or already fully set up.

## Non-Goals

- No changes to the backend setup-state model introduced by consolidation.
- No new step semantics or badge behavior.
- No attempt to make setup definitions fully data-driven across backend and frontend in this plan.
- No broad visual redesign of the app pages beyond what is needed to host the shared surfaces.

## Design Principles

- One implementation per setup capability.
- Shared surfaces should support at least two modes:
  - `setup`
  - `manage`
- Shared surfaces should own their own fetch/mutation logic unless there is already a stable shared hook worth reusing.
- Wizard orchestration should stay separate from step internals.
- App pages should become thin shells that compose shared surfaces rather than reimplement setup flows.

## Current State After Consolidation

Current starting point after the setup-state rollout:

- `ui/src/components/ProjectSetupWizardGate.tsx` owns step ordering and routing inside the wizard.
- App pages no longer own checklist status as the primary setup system.
- `DatasourceSetupFlow` already supports wizard embedding, including embedded back navigation.
- The actual UI for schema selection, AI config, ontology extraction, questions, glossary management, queries, and tunnel management still mostly lives in page-specific code paths.
- The wizard still depends on page links for most setup work outside datasource configuration and inline activation.

## Target State

Each setup capability should be represented by a shared surface component that can be rendered:

- inside the Setup Wizard as an embedded step
- inside the relevant app page as the maintenance/configuration surface

Suggested surface contract:

```typescript
type SetupSurfaceMode = 'setup' | 'manage';

interface SetupSurfaceProps {
  projectId: string;
  mode: SetupSurfaceMode;
  onComplete?: () => void;
  onStateChange?: () => void;
}
```

Behavior expectations:

- In `setup` mode:
  - emphasize the next required action
  - surface compact instructional copy
  - call `onComplete` when the step reaches completion
- In `manage` mode:
  - show the same controls, current status, and edit paths
  - avoid wizard-specific framing and navigation
  - preserve the ability to revisit and change configuration after setup

## Shared Surface Inventory

| Surface | Current source | Used by wizard | Used by page |
|---------|----------------|----------------|--------------|
| `DatasourceSetupSurface` | `DatasourceSetupFlow` / datasource page flow | MCP Server datasource step | MCP Server page |
| `SchemaSelectionSurface` | Schema page flow | Ontology Forge schema step | Ontology Forge page or schema page shell |
| `AIConfigSurface` | AI config page flow | Ontology Forge AI step | Ontology Forge page or AI config page shell |
| `OntologyExtractionSurface` | Ontology page flow | Ontology Forge extraction step | Ontology Forge page or ontology page shell |
| `OntologyQuestionsSurface` | ontology questions page flow | Ontology Forge questions step | Ontology Forge page or questions page shell |
| `QueriesSetupSurface` | queries page flow | Ontology Forge optional queries step, AI Agents queries step | AI Agents page, Ontology Forge page, queries page shell |
| `GlossarySetupSurface` | glossary page flow / AI Data Liaison page | AI Data Liaison glossary step | AI Data Liaison page, glossary page shell |
| `AppActivationSurface` | installed-app activation behavior | AI Data Liaison activation, MCP Tunnel activation | AI Data Liaison page, MCP Tunnel page |
| `TunnelConnectionSurface` | MCP Tunnel page | MCP Tunnel connection step | MCP Tunnel page |

Notes:

- It is acceptable for some existing route pages such as `/schema`, `/ai-config`, `/ontology`, `/ontology-questions`, `/queries`, and `/glossary` to become thin wrappers around the extracted shared surfaces.
- The wizard should not own business logic for these steps. It should only host and sequence the shared surfaces.

---

## Implementation Tasks

### Task 1: Frontend - Shared Surface Foundation

- [ ] Create a shared setup-surface pattern and folder structure

Suggested structure:

- `ui/src/components/setup/`
- `ui/src/components/setup/types.ts`
- `ui/src/components/setup/layout/`
- `ui/src/components/setup/surfaces/`

Create:

- a common `SetupSurfaceProps` type
- lightweight shared layout primitives for step headers, status banners, and completion messaging
- a small helper pattern for step-local loading/error/success states

### Task 2: Frontend - Convert Existing Datasource Flow

- [ ] Adapt `DatasourceSetupFlow` into the shared surface pattern

This is the baseline example and should establish:

- how surfaces signal completion
- how setup and manage modes differ
- how wizard embedding and page embedding coexist cleanly

Current branch note:

- the datasource flow already supports embedded wizard mode, but it has not yet been extracted into a shared setup-surface contract that app pages can compose directly

### Task 3: Frontend - Extract Ontology Forge Surfaces

- [ ] Extract the Ontology Forge setup work into shared surfaces

Scope:

- schema selection
- AI configuration
- ontology extraction
- ontology questions
- optional pre-approved queries

Expected result:

- `OntologyForgePage.tsx` becomes a thin composition over the shared surfaces
- wizard steps for Ontology Forge render these surfaces inline
- route pages like `/schema`, `/ai-config`, `/ontology`, and `/ontology-questions` can reuse the same internals instead of maintaining separate implementations

### Task 4: Frontend - Extract AI Data Liaison and AI Agents Surfaces

- [ ] Extract shared surfaces for glossary setup, app activation, and agent query prerequisites

Scope:

- glossary term creation / glossary readiness
- AI Data Liaison activation
- AI Agents query prerequisite management

Expected result:

- AI Data Liaison and AI Agents no longer have wizard-specific or page-specific setup logic forks
- the same query-setup capability can be reused by both Ontology Forge and AI Agents

### Task 5: Frontend - Extract MCP Tunnel Surfaces

- [ ] Extract shared surfaces for tunnel activation and tunnel connection monitoring

Scope:

- activation CTA and activation status
- connection state, refresh/poll behavior, and connected messaging

Expected result:

- wizard and MCP Tunnel page render the same activation and connection surfaces
- tunnel polling logic exists in one place

### Task 6: Frontend - Refactor Wizard to Render Shared Surfaces

- [ ] Update `ui/src/components/ProjectSetupWizardGate.tsx` to render the extracted shared surfaces directly

Requirements:

- wizard step registry maps step IDs to shared surfaces
- wizard no longer routes users away for the normal setup path except where intentionally required
- completion and refresh behavior is driven by the shared surface callbacks plus `useSetupStatus`

Current branch note:

- the wizard already uses `useSetupStatus`, embeds datasource setup inline, and supports inline app activation
- the remaining work is to replace page links and ad hoc embedded logic with extracted shared surfaces for the rest of the setup capabilities

### Task 7: Frontend - Refactor App Pages to Compose Shared Surfaces

- [ ] Replace page-local setup UI on app pages with shared surfaces in `manage` mode

Pages in scope:

- `ui/src/pages/MCPServerPage.tsx`
- `ui/src/pages/OntologyForgePage.tsx`
- `ui/src/pages/AIDataLiaisonPage.tsx`
- `ui/src/pages/MCPTunnelPage.tsx`
- `ui/src/pages/AIAgentsPage.tsx`

Expected result:

- app pages retain their identity as management/configuration screens
- setup-critical controls come from shared surfaces rather than duplicated page logic

### Task 8: Frontend - Route Pages Become Thin Shells Where Appropriate

- [ ] Convert specialized route pages to thin wrappers around shared surfaces where doing so reduces duplication

Candidate routes:

- `/schema`
- `/ai-config`
- `/ontology`
- `/ontology-questions`
- `/queries`
- `/glossary`

This task should be applied pragmatically. Only convert a route page when it reduces duplicate logic without harming the page’s non-setup responsibilities.

### Task 9: Frontend - Remove Duplicated Setup Hooks and Drift

- [ ] Remove obsolete page-specific setup fetch/state code after shared surfaces are in place

Examples of duplication to remove:

- page-local readiness fetch bundles that only existed for setup cards
- page-local activation orchestration that now belongs to shared surfaces
- duplicated polling logic for tunnel setup
- repeated query/glossary readiness checks that shared surfaces now own

### Task 10: Test Coverage

- [ ] Add or update automated tests around shared surfaces and wizard embedding

Coverage areas:

- surface rendering in `setup` mode and `manage` mode
- wizard advancement when surfaces complete
- app pages rendering the same shared surfaces
- tunnel polling behavior staying scoped to the shared tunnel surface
- regression coverage for Ontology Forge, AI Data Liaison, AI Agents, and MCP Tunnel setup flows

---

## Key Files

**Primary existing files**

- `ui/src/components/ProjectSetupWizardGate.tsx`
- `ui/src/components/DatasourceSetupFlow.tsx`
- `ui/src/pages/MCPServerPage.tsx`
- `ui/src/pages/OntologyForgePage.tsx`
- `ui/src/pages/AIDataLiaisonPage.tsx`
- `ui/src/pages/MCPTunnelPage.tsx`
- `ui/src/pages/AIAgentsPage.tsx`
- `ui/src/pages/SchemaPage.tsx`
- `ui/src/pages/AIConfigPage.tsx`
- `ui/src/pages/OntologyPage.tsx`
- `ui/src/pages/OntologyQuestionsPage.tsx`
- `ui/src/pages/QueriesPage.tsx`
- `ui/src/pages/GlossaryPage.tsx`

**Suggested new files**

- `ui/src/components/setup/types.ts`
- `ui/src/components/setup/layout/SetupSurfaceFrame.tsx`
- `ui/src/components/setup/surfaces/DatasourceSetupSurface.tsx`
- `ui/src/components/setup/surfaces/SchemaSelectionSurface.tsx`
- `ui/src/components/setup/surfaces/AIConfigSurface.tsx`
- `ui/src/components/setup/surfaces/OntologyExtractionSurface.tsx`
- `ui/src/components/setup/surfaces/OntologyQuestionsSurface.tsx`
- `ui/src/components/setup/surfaces/QueriesSetupSurface.tsx`
- `ui/src/components/setup/surfaces/GlossarySetupSurface.tsx`
- `ui/src/components/setup/surfaces/AppActivationSurface.tsx`
- `ui/src/components/setup/surfaces/TunnelConnectionSurface.tsx`

## Verification

1. `make check` passes.
2. The Setup Wizard can complete datasource, schema, AI config, ontology extraction, questions, glossary, queries, and tunnel steps without routing the user out for the common path.
3. The same shared surface code is used by the wizard and the relevant app page for each setup capability.
4. App pages continue to support revisiting and changing configuration after setup is complete.
5. Removing duplicated page-local setup logic does not change setup-state behavior or step completion semantics.
