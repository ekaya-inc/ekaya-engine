# PLAN: UI Frontend Test Coverage

**Status: PENDING**

## Objective

Increase test coverage in the `ui/` frontend from ~31% file coverage (31/98 files) to cover all priority files listed below. All tests must pass with `cd ui && npx vitest run`.

## Environment

- Working directory for all commands: `ui/` (i.e., `/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/ui`)
- Run tests: `cd ui && npx vitest run`
- Test framework: vitest with happy-dom, @testing-library/react
- Config: `ui/vitest.config.js`
- Setup file: `ui/src/test/setup.ts`

## Test File Naming Convention

- Service tests: `src/services/__tests__/<name>.test.ts`
- Component tests: `src/components/__tests__/<name>.test.tsx`
- Page tests: `src/pages/__tests__/<name>.test.tsx`
- Hook tests: `src/hooks/__tests__/<name>.test.ts`
- Lib tests: `src/lib/<name>.test.ts`

## Existing Patterns to Follow

Read these files before writing any tests to understand conventions:

- `src/lib/api.test.ts` — API mock setup with `vi.mock('../../lib/api')`
- `src/services/__tests__/engineApi.test.ts` — Service test structure and mock Response creation
- `src/components/__tests__/GlossaryTermEditor.test.tsx` — Component test with userEvent
- `src/hooks/__tests__/useSqlValidation.test.ts` — Hook test with renderHook/act
- `src/pages/__tests__/QueriesPage.test.tsx` — Page test with MemoryRouter/Routes

### Key mock patterns

For all API service tests:
```ts
vi.mock('../../lib/api', () => ({ fetchWithAuth: vi.fn() }))
import { fetchWithAuth } from '../../lib/api'
const mockFetchWithAuth = fetchWithAuth as vi.Mock
```

For engineApi (singleton default export):
```ts
import engineApi from '../engineApi'
```

Mock Response objects:
```ts
mockFetchWithAuth.mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
```

## Implementation Checklist

### Priority 1 — API Services

#### `src/services/engineApi.ts` (1,344 lines, ~77 methods, 3 tests exist)

Read `src/services/__tests__/engineApi.test.ts` first — existing tests cover `makeRequest` 204/200/error behavior. New tests cover individual public API methods only.

File to create: `src/services/__tests__/engineApi.methods.test.ts`

- [x] Add tests for datasource methods: `createDataSource`, `updateDataSource`, `deleteDataSource`, `listDataSources`, `getDataSource`, `renameDatasource` — verify correct URL, HTTP method, request body, and parsed response
- [x] Add tests for schema operation methods: `refreshSchema`, `getSchema`, `getOntology`, `getContext`, `searchSchema` — verify correct URL and response parsing
- [x] Add tests for query CRUD methods: `listQueries`, `getQuery`, `createQuery`, `updateQuery`, `deleteQuery` — verify correct URL, HTTP method, body
- [x] Add tests for query execution methods: `executeQuery`, `explainQuery`, `validateQuery` — verify correct URL, body, and response parsing
- [x] Add tests for approved query methods: `listApprovedQueries`, `executeApprovedQuery` — verify URL and body
- [x] Add tests for glossary CRUD methods: `listGlossaryTerms`, `getGlossaryTerm`, `createGlossaryTerm`, `updateGlossaryTerm`, `deleteGlossaryTerm`, `getGlossarySql` — verify correct URL, HTTP method, body
- [x] Add tests for AI config methods: `getAIConfig`, `updateAIConfig`, `deleteAIConfig`, `testAIConfig` — verify 204 handling for delete, request/response shapes
- [x] Add tests for ontology change methods: `listPendingChanges`, `approveChange`, `rejectChange`, `approveAllChanges` — verify URL and method
- [x] Add tests for project knowledge methods: `listProjectKnowledge`, `createProjectKnowledge`, `updateProjectKnowledge`, `deleteProjectKnowledge` — verify URL, method, body
- [x] Add tests for MCP config methods: `getMCPConfig`, `updateMCPConfig` — verify URL, method, response (no `deleteMcpConfig` exists in engineApi)
- [x] Add tests for alerts methods: `listAlerts`, `getAlert`, `dismissAlert` — verify URL, method, response
- [ ] Run `cd ui && npx vitest run` and confirm all tests pass

#### `src/services/ontologyApi.ts` (482 lines, ~22 methods, 0 tests)

File to create: `src/services/__tests__/ontologyApi.test.ts`

- [x] Read `src/services/ontologyApi.ts` to identify all exported async methods and response shapes
- [ ] Add tests for each async method verifying: correct URL passed to `fetchWithAuth`, correct HTTP method, correct request body, correct response parsing
- [x] Add tests for `pollStatus` polling logic: verify polling starts, calls the correct endpoint at each interval, stops on terminal status, handles errors
- [ ] Add tests for wrapped vs. unwrapped response handling (based on actual behavior found in the file)
- [x] Run `cd ui && npx vitest run` and confirm all tests pass

#### `src/services/config.ts` (147 lines, 5 exports, 0 tests)

File to create: `src/lib/config.test.ts` (or `src/services/__tests__/config.test.ts` — match actual location of the file)

- [x] Read `src/services/config.ts` to identify actual export names and signatures
- [x] Add tests for `fetchConfig()`: mock `global.fetch` for both `/api/config` and `/.well-known/oauth-authorization-server`, verify parallel fetch and merged result
- [x] Add tests for `getCachedConfig()`: verify returns null before fetch, returns cached value after fetch
- [x] Add tests for `getAuthUrlFromQuery()`: verify URL parameter extraction for valid and missing query params
- [x] Add tests for `getProjectIdFromPath()`: verify path segment extraction for valid and missing paths
- [x] Run `cd ui && npx vitest run` and confirm all tests pass

### Priority 2 — Business Logic

#### `src/services/ontologyService.ts` (512 lines, ~10 functions, 0 tests)

File to create: `src/services/__tests__/ontologyService.test.ts`

- [x] Read `src/services/ontologyService.ts` to identify all exported functions
- [x] Add tests for `transformEntityQueue`: provide representative input, assert correct output shape
- [ ] Add tests for `transformTaskQueue`: provide representative input, assert correct output shape
- [x] Add tests for `transformQuestions`: provide representative input, assert correct output shape
- [x] Add tests for `startPolling`: mock ontologyApi and timer functions (`vi.useFakeTimers()`), verify polling interval and stop behavior
- [x] Add tests for `stopPolling`: verify timer is cleared and polling halts
- [x] Run `cd ui && npx vitest run` and confirm all tests pass

### Priority 3 — Critical Components

#### `src/components/AIConfigWidget.tsx` (563 lines, 0 tests)

File to create: `src/components/__tests__/AIConfigWidget.test.tsx`

- [x] Read `src/components/AIConfigWidget.tsx` to identify props interface, mock dependencies
- [ ] Add test: loads AI config on mount and displays provider/model values
- [x] Add test: provider selection updates the form state
- [ ] Add test: form validation prevents save when required fields are empty
- [x] Add test: save operation calls `engineApi.updateAIConfig` with correct payload and shows success state
- [x] Add test: delete operation calls `engineApi.deleteAIConfig` and updates UI state
- [ ] Add test: test-connection operation calls `engineApi.testAIConfig` and displays result
- [x] Add test: error from any API call is displayed to the user
- [ ] Mock: `vi.mock('../../services/engineApi', () => ({ default: { getAIConfig: vi.fn(), updateAIConfig: vi.fn(), deleteAIConfig: vi.fn(), testAIConfig: vi.fn() } }))`
- [ ] Run `cd ui && npx vitest run` and confirm all tests pass

#### `src/contexts/ProjectContext.tsx` (76 lines, 0 tests)

File to create: `src/contexts/__tests__/ProjectContext.test.tsx`

- [ ] Read `src/contexts/ProjectContext.tsx` to confirm exported hook name and context shape
- [ ] Add test: `ProjectContext.Provider` renders children without error
- [ ] Add test: `useProjectContext` hook returns initial state when no value set
- [ ] Add test: `setProjectInfo` updates context value accessible to consumers
- [ ] Add test: `clearProjectInfo` resets context to initial state
- [ ] Run `cd ui && npx vitest run` and confirm all tests pass

### Priority 4 — Auth Flows

#### `src/lib/oauth.ts` (101 lines, 2 exports, 0 direct tests for PKCE)

Note: `src/lib/oauth.test.ts` may already exist — read it to check what is already covered before writing new tests.

- [ ] Read `src/lib/oauth.ts` to identify actual exports and PKCE implementation
- [ ] Read existing `oauth.test.ts` to identify what is already tested
- [ ] Add tests for `generateRandomString`: verify output length, character set (URL-safe base64), uniqueness across calls
- [ ] Add tests for full PKCE flow: code verifier generation, code challenge derivation (SHA-256 + base64url), verify challenge matches verifier per RFC 7636
- [ ] Run `cd ui && npx vitest run` and confirm all tests pass

## Completion Criteria

- All new test files are created and passing
- `cd ui && npx vitest run` exits with 0
- No existing tests are broken
- Do not create documentation, README, or architecture files
