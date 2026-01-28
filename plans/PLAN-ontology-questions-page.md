# Plan: Ontology Questions Tile and Page

## Context

The ontology extraction process generates questions that go beyond what can be inferred from database schema alone. Answering these questions is key to improving ontology quality from ~2/10 (schema-only) to 8+/10 (full unambiguous ontology).

Currently, questions are displayed within the Ontology Extraction page, but they deserve their own dedicated tile and page in the Intelligence area. This makes the questions more discoverable and emphasizes their importance in the ontology refinement workflow.

**Key insight:** MCP Clients (like Claude Code) can automate answering ~190+ questions by reviewing source code. However, not every project has a codebase to query—some are business processes or 3rd party applications. The Admin UI should guide users toward using an MCP Client for efficiency while supporting manual review.

## Requirements

1. **New Tile:** "Ontology Questions" in the Intelligence section
   - Position: After "Ontology" (extraction) and before "Project Knowledge"
   - Route: `/projects/:pid/questions`
   - Badge: Shows count when pending questions exist

2. **New Page:** Ontology Questions screen
   - Introductory text explaining the purpose of these questions
   - Section explaining how to use AI (MCP Client) to answer questions
   - List of pending questions (simple display, no CRUD operations initially)

## Implementation Tasks

### Task 1: Add New Tile to Project Dashboard

**File:** `ui/src/pages/ProjectDashboard.tsx`

1. Add new tile definition in `intelligenceTiles` array between "Ontology" and "Project Knowledge":
   ```typescript
   {
     title: 'Ontology Questions',
     icon: MessageCircleQuestion,
     path: `/projects/${pid}/questions`,
     disabled: !isConnected || !hasSelectedTables,
     color: 'amber',
   },
   ```

2. Update badge rendering logic to also show badge on "Ontology Questions" tile:
   ```typescript
   const isQuestionsTile = tile.title === 'Ontology Questions';
   // Show badge on both Ontology tile and Questions tile
   {(isOntologyTile || isQuestionsTile) && pendingQuestions > 0 && !tile.disabled && (
     ...badge JSX...
   )}
   ```

3. Remove badge from Ontology tile (optional, if we want badge only on Questions tile)

### Task 2: Add Route in App.tsx

**File:** `ui/src/App.tsx`

1. Import the new page component:
   ```typescript
   import OntologyQuestionsPage from './pages/OntologyQuestionsPage';
   ```

2. Add route after `ontology` route:
   ```typescript
   <Route path="questions" element={<OntologyQuestionsPage />} />
   ```

### Task 3: Create OntologyQuestionsPage Component

**File:** `ui/src/pages/OntologyQuestionsPage.tsx` (new file)

**Structure:**

```typescript
const OntologyQuestionsPage = () => {
  const { pid } = useParams<{ pid: string }>();
  const navigate = useNavigate();

  // State
  const [questions, setQuestions] = useState<QuestionDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Fetch questions on mount
  useEffect(() => {
    fetchQuestions();
  }, [pid]);

  return (
    <div className="space-y-6 p-6">
      {/* Header with back button */}
      <div className="flex items-center gap-4">
        <Button variant="ghost" onClick={() => navigate(`/projects/${pid}`)}>
          <ArrowLeft className="h-4 w-4 mr-2" />
          Back
        </Button>
        <h1 className="text-2xl font-semibold">Ontology Questions</h1>
      </div>

      {/* Intro text */}
      <Card>
        <CardContent className="pt-6">
          <p className="text-muted-foreground">
            Ontology Extraction is limited by what is in the data. The answers to these
            questions go beyond the contents of the data and are important to enabling
            your users to ask ad-hoc questions.
          </p>
        </CardContent>
      </Card>

      {/* AI Answering Section */}
      <AIAnsweringGuide projectId={pid} questionCount={questions.length} />

      {/* Questions List */}
      <QuestionsList questions={questions} loading={loading} error={error} />
    </div>
  );
};
```

### Task 4: Create AIAnsweringGuide Component

**File:** `ui/src/components/ontology/AIAnsweringGuide.tsx` (new file)

This component explains how to use an MCP Client to answer questions.

**Content sections:**

1. **Header:** "Answering Questions with AI"

2. **Overview text:**
   "The recommended way to answer these questions is using an AI assistant connected via MCP (Model Context Protocol). When connected to your codebase, AI can research each question and update the ontology automatically."

3. **MCP Connection Instructions:**
   - Brief explanation that MCP tools like `list_ontology_questions`, `resolve_ontology_question`, `update_entity`, etc. are available
   - Link/reference to MCP Server configuration page
   - Example workflow: AI reads question → researches code → updates ontology → resolves question

4. **When AI Can't Help:**
   "For projects without accessible source code (business processes, 3rd party applications), you can review and answer questions manually below."

### Task 5: Create QuestionsList Component

**File:** `ui/src/components/ontology/QuestionsList.tsx` (new file)

Simple list display of pending questions.

**Features:**
- Group by category (or priority)
- Show question text, category, priority badge, affected tables/columns
- No CRUD operations in first implementation
- Empty state when no questions
- Loading and error states

**Structure:**
```typescript
interface QuestionsListProps {
  questions: QuestionDTO[];
  loading: boolean;
  error: string | null;
}

const QuestionsList = ({ questions, loading, error }: QuestionsListProps) => {
  if (loading) return <LoadingSkeleton />;
  if (error) return <ErrorState message={error} />;
  if (questions.length === 0) return <EmptyState />;

  // Group questions by category
  const grouped = groupByCategory(questions);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Pending Questions ({questions.length})</CardTitle>
      </CardHeader>
      <CardContent>
        {Object.entries(grouped).map(([category, categoryQuestions]) => (
          <CategorySection
            key={category}
            category={category}
            questions={categoryQuestions}
          />
        ))}
      </CardContent>
    </Card>
  );
};
```

**Category display order:**
1. `business_rules` - Business Rules
2. `relationship` - Relationships
3. `terminology` - Terminology
4. `enumeration` - Enumerations
5. `temporal` - Temporal Patterns
6. `data_quality` - Data Quality

**Question item display:**
- Question text
- Priority badge (1=Critical, 2=High, 3=Medium, 4-5=Low)
- Affected tables/columns as small chips
- Reasoning (if present) in expandable section

### Task 6: Add API Method to engineApi (if not already present)

**File:** `ui/src/services/engineApi.ts`

Verify the questions list endpoint is exposed. It should return `QuestionDTO[]` matching the handler response.

```typescript
// May already exist via ontologyApi.ts
// If using engineApi pattern, add:
listOntologyQuestions: async (projectId: string) => {
  const response = await api.get<ApiResponse<{ questions: QuestionDTO[]; total: number }>>(
    `/api/projects/${projectId}/ontology/questions`
  );
  return response.data;
},
```

## File Changes Summary

| File | Change |
|------|--------|
| `ui/src/pages/ProjectDashboard.tsx` | Add tile, update badge logic |
| `ui/src/App.tsx` | Add route, import page |
| `ui/src/pages/OntologyQuestionsPage.tsx` | **New file** - main page |
| `ui/src/components/ontology/AIAnsweringGuide.tsx` | **New file** - MCP guidance |
| `ui/src/components/ontology/QuestionsList.tsx` | **New file** - questions display |

## Design Decisions

1. **Separate tile vs embedding in Ontology page:** A separate tile emphasizes the importance of questions and makes them more discoverable. The badge draws attention when action is needed.

2. **Badge on Questions tile only:** Moving the badge from "Ontology" to "Ontology Questions" provides clearer UX—the badge appears on the tile that leads directly to the questions.

3. **Amber color for tile:** Matches the existing amber badge color, creating visual consistency. Amber suggests "attention needed" which fits the purpose.

4. **No CRUD initially:** Keep the first implementation simple—just display questions. CRUD operations can be added later if the Admin needs to manually answer questions through the UI (vs MCP).

5. **AI-first guidance:** Prominently feature MCP/AI instructions since that's the preferred workflow. Manual review is secondary.

## Future Enhancements (Out of Scope)

- Manual answer submission form
- Skip/dismiss actions
- Filter by status (pending, answered, skipped)
- Batch answer operations
- Progress tracking for AI answering sessions
- Integration with specific MCP clients
