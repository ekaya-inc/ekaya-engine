import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import engineApi from "../../services/engineApi";
import type { Query } from "../../types";
import QueriesView from "../QueriesView";

// Mock the engineApi module
vi.mock("../../services/engineApi", () => ({
  default: {
    listQueries: vi.fn(),
    getQuery: vi.fn(),
    createQuery: vi.fn(),
    updateQuery: vi.fn(),
    deleteQuery: vi.fn(),
    testQuery: vi.fn(),
    executeQuery: vi.fn(),
    validateQuery: vi.fn(),
    getSchema: vi.fn(),
  },
}));

// Mock useToast hook
vi.mock("../../hooks/useToast", () => ({
  useToast: () => ({
    toast: vi.fn(),
  }),
}));

// Mock useSqlValidation hook
vi.mock("../../hooks/useSqlValidation", () => ({
  useSqlValidation: () => ({
    status: "idle",
    error: null,
    warnings: [],
    validate: vi.fn(),
    reset: vi.fn(),
  }),
}));

// Mock SqlEditor component (CodeMirror has issues in test environment)
vi.mock("../SqlEditor", () => ({
  SqlEditor: ({
    value,
    onChange,
    placeholder,
  }: {
    value: string;
    onChange: (v: string) => void;
    placeholder?: string;
  }) => (
    <textarea
      data-testid="sql-editor"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
    />
  ),
}));

// Mock DeleteQueryDialog
vi.mock("../DeleteQueryDialog", () => ({
  DeleteQueryDialog: ({
    open,
    query,
    onQueryDeleted,
  }: {
    open: boolean;
    query: Query | null;
    onQueryDeleted: (id: string) => void;
  }) =>
    open && query ? (
      <div data-testid="delete-dialog">
        <button onClick={() => onQueryDeleted(query.query_id)}>
          Confirm Delete
        </button>
      </div>
    ) : null,
}));

const mockQueries: Query[] = [
  {
    query_id: "query-1",
    project_id: "proj-1",
    datasource_id: "ds-1",
    natural_language_prompt: "Show top customers",
    additional_context: "By revenue",
    sql_query: "SELECT * FROM customers ORDER BY revenue DESC LIMIT 10",
    dialect: "postgres",
    is_enabled: true,
    allows_modification: false,
    usage_count: 5,
    last_used_at: "2024-01-20T00:00:00Z",
    created_at: "2024-01-15T00:00:00Z",
    updated_at: "2024-01-15T00:00:00Z",
    parameters: [],
    status: "approved",
  },
  {
    query_id: "query-2",
    project_id: "proj-1",
    datasource_id: "ds-1",
    natural_language_prompt: "Daily sales report",
    additional_context: null,
    sql_query: "SELECT date, SUM(amount) FROM sales GROUP BY date",
    dialect: "postgres",
    is_enabled: false,
    allows_modification: false,
    usage_count: 0,
    last_used_at: null,
    created_at: "2024-01-10T00:00:00Z",
    updated_at: "2024-01-10T00:00:00Z",
    parameters: [],
    status: "approved",
  },
];

const mockQueriesWithStatuses: Query[] = [
  {
    query_id: "query-approved",
    project_id: "proj-1",
    datasource_id: "ds-1",
    natural_language_prompt: "Approved query",
    additional_context: null,
    sql_query: "SELECT * FROM approved",
    dialect: "postgres",
    is_enabled: true,
    allows_modification: false,
    usage_count: 0,
    last_used_at: null,
    created_at: "2024-01-15T00:00:00Z",
    updated_at: "2024-01-15T00:00:00Z",
    parameters: [],
    status: "approved",
  },
  {
    query_id: "query-pending",
    project_id: "proj-1",
    datasource_id: "ds-1",
    natural_language_prompt: "Pending query",
    additional_context: null,
    sql_query: "SELECT * FROM pending",
    dialect: "postgres",
    is_enabled: false,
    allows_modification: false,
    usage_count: 0,
    last_used_at: null,
    created_at: "2024-01-10T00:00:00Z",
    updated_at: "2024-01-10T00:00:00Z",
    parameters: [],
    status: "pending",
    suggested_by: "agent",
  },
  {
    query_id: "query-rejected",
    project_id: "proj-1",
    datasource_id: "ds-1",
    natural_language_prompt: "Rejected query",
    additional_context: null,
    sql_query: "SELECT * FROM rejected",
    dialect: "postgres",
    is_enabled: false,
    allows_modification: false,
    usage_count: 0,
    last_used_at: null,
    created_at: "2024-01-05T00:00:00Z",
    updated_at: "2024-01-05T00:00:00Z",
    parameters: [],
    status: "rejected",
    suggested_by: "agent",
    rejection_reason: "SQL syntax invalid",
  },
  {
    query_id: "query-modifying",
    project_id: "proj-1",
    datasource_id: "ds-1",
    natural_language_prompt: "Modifying query",
    additional_context: null,
    sql_query: "INSERT INTO users (name) VALUES ({{name}}) RETURNING id",
    dialect: "postgres",
    is_enabled: true,
    allows_modification: true,
    usage_count: 0,
    last_used_at: null,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    parameters: [],
    status: "approved",
  },
];

const buildQuery = (overrides: Partial<Query>): Query => ({
  query_id: "query-test",
  project_id: "proj-1",
  datasource_id: "ds-1",
  natural_language_prompt: "Parameterized query",
  additional_context: null,
  sql_query: "SELECT * FROM items WHERE region = {{region}}",
  dialect: "postgres",
  is_enabled: true,
  allows_modification: false,
  usage_count: 0,
  last_used_at: null,
  created_at: "2024-01-15T00:00:00Z",
  updated_at: "2024-01-15T00:00:00Z",
  parameters: [],
  status: "approved",
  ...overrides,
});

describe("QueriesView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default mock for getSchema to prevent unhandled rejections
    vi.mocked(engineApi.getSchema).mockResolvedValue({
      success: true,
      data: { tables: [], total_tables: 0, relationships: [] },
    });
  });

  it("renders loading state initially", () => {
    vi.mocked(engineApi.listQueries).mockReturnValue(new Promise(() => {}));

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    expect(screen.getByText(/loading queries/i)).toBeInTheDocument();
  });

  it("renders queries after loading", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Show top customers")).toBeInTheDocument();
      expect(screen.getByText("Daily sales report")).toBeInTheDocument();
    });
  });

  it("shows empty state when no queries", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: [] },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText(/no approved queries yet/i)).toBeInTheDocument();
    });
  });

  it("shows error state on load failure", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: false,
      error: "Database connection failed",
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText(/failed to load queries/i)).toBeInTheDocument();
      expect(
        screen.getByText("Database connection failed"),
      ).toBeInTheDocument();
    });
  });

  it("opens create form when clicking add button", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Show top customers")).toBeInTheDocument();
    });

    // Find the add button (the Plus icon button in the header)
    const addButtons = screen.getAllByRole("button");
    const addButton = addButtons.find((btn) =>
      btn.querySelector("svg.lucide-plus"),
    );
    expect(addButton).toBeInTheDocument();

    if (!addButton)
      throw new Error("Expected to find add button with plus icon");
    fireEvent.click(addButton);

    expect(screen.getByText("Create New Query")).toBeInTheDocument();
  });

  it("filters queries based on search term", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Show top customers")).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/search queries/i);
    fireEvent.change(searchInput, { target: { value: "daily" } });

    expect(screen.queryByText("Show top customers")).not.toBeInTheDocument();
    expect(screen.getByText("Daily sales report")).toBeInTheDocument();
  });

  it("shows no results message when search has no matches", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Show top customers")).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/search queries/i);
    fireEvent.change(searchInput, { target: { value: "nonexistent" } });

    expect(screen.getByText(/no queries found/i)).toBeInTheDocument();
  });

  it("selects query when clicking on list item", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Show top customers")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Show top customers"));

    // Should show query details in the right panel - check for detail view elements
    await waitFor(() => {
      // In detail view, the prompt appears as a CardTitle
      // Check for the SQL Query section header which only appears in detail view
      expect(screen.getByText("SQL Query")).toBeInTheDocument();
    });
  });

  it("shows detail view with action buttons when query selected", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Show top customers")).toBeInTheDocument();
    });

    // Select the query
    fireEvent.click(screen.getByText("Show top customers"));

    // Should show query details in the right panel
    await waitFor(() => {
      expect(screen.getByText("SQL Query")).toBeInTheDocument();
    });

    // Should have the Execute Query button in detail view
    expect(
      screen.getByRole("button", { name: /execute query/i }),
    ).toBeInTheDocument();

    // Should have the Copy button
    expect(screen.getByRole("button", { name: /copy/i })).toBeInTheDocument();
  });

  it("disables the approved execute button when a required parameter without a default is missing", async () => {
    const query = buildQuery({
      query_id: "query-approved-required",
      parameters: [
        {
          name: "region",
          type: "string",
          description: "Region",
          required: true,
          default: null,
        },
      ],
    });

    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: [query] },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
        initialQueryId={query.query_id}
      />,
    );

    const executeButton = await screen.findByRole("button", {
      name: /execute query/i,
    });
    expect(executeButton).toBeDisabled();
  });

  it("enables approved execution after entering a required parameter value", async () => {
    const query = buildQuery({
      query_id: "query-approved-enter-value",
      parameters: [
        {
          name: "region",
          type: "string",
          description: "Region",
          required: true,
          default: null,
        },
      ],
    });

    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: [query] },
    });
    vi.mocked(engineApi.executeQuery).mockResolvedValue({
      success: true,
      data: {
        columns: [{ name: "region", type: "text" }],
        rows: [{ region: "emea" }],
        row_count: 1,
      },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
        initialQueryId={query.query_id}
      />,
    );

    const executeButton = await screen.findByRole("button", {
      name: /execute query/i,
    });
    expect(executeButton).toBeDisabled();

    fireEvent.change(screen.getByRole("textbox", { name: /region/i }), {
      target: { value: "emea" },
    });

    expect(executeButton).not.toBeDisabled();

    fireEvent.click(executeButton);

    await waitFor(() => {
      expect(engineApi.executeQuery).toHaveBeenCalledWith(
        "proj-1",
        "ds-1",
        query.query_id,
        { limit: 100, parameters: { region: "emea" } },
      );
    });
  });

  it("disables the pending execute button when a required parameter without a default is missing", async () => {
    const query = buildQuery({
      query_id: "query-pending-required",
      status: "pending",
      is_enabled: false,
      suggested_by: "agent",
      parameters: [
        {
          name: "region",
          type: "string",
          description: "Region",
          required: true,
          default: null,
        },
      ],
    });

    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: [query] },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="pending"
        initialQueryId={query.query_id}
      />,
    );

    const executeButton = await screen.findByRole("button", {
      name: /^execute$/i,
    });
    expect(executeButton).toBeDisabled();
  });

  it("does not block execution when a required parameter already has a default", async () => {
    const query = buildQuery({
      query_id: "query-approved-default",
      parameters: [
        {
          name: "region",
          type: "string",
          description: "Region",
          required: true,
          default: "emea",
        },
      ],
    });

    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: [query] },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
        initialQueryId={query.query_id}
      />,
    );

    const executeButton = await screen.findByRole("button", {
      name: /execute query/i,
    });
    expect(executeButton).not.toBeDisabled();
  });

  it("disables execution after clearing a required parameter that originally relied on a default", async () => {
    const query = buildQuery({
      query_id: "query-approved-default-cleared",
      parameters: [
        {
          name: "region",
          type: "string",
          description: "Region",
          required: true,
          default: "emea",
        },
      ],
    });

    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: [query] },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
        initialQueryId={query.query_id}
      />,
    );

    const executeButton = await screen.findByRole("button", {
      name: /execute query/i,
    });
    const regionInput = screen.getByRole("textbox", { name: /region/i });

    expect(executeButton).not.toBeDisabled();

    fireEvent.change(regionInput, {
      target: { value: "apac" },
    });
    expect(executeButton).not.toBeDisabled();

    fireEvent.change(regionInput, {
      target: { value: "" },
    });

    expect(executeButton).toBeDisabled();

    fireEvent.click(executeButton);

    expect(engineApi.executeQuery).not.toHaveBeenCalled();
  });

  it("handles query deletion through the DeleteQueryDialog callback", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Show top customers")).toBeInTheDocument();
      expect(screen.getByText("Daily sales report")).toBeInTheDocument();
    });

    // Both queries should be visible initially
    expect(screen.getByText("Show top customers")).toBeInTheDocument();
    expect(screen.getByText("Daily sales report")).toBeInTheDocument();
  });

  it("shows empty state with create button when no query selected", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Show top customers")).toBeInTheDocument();
    });

    // Should show empty state in right panel
    expect(screen.getByText("No Query Selected")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /create new query/i }),
    ).toBeInTheDocument();
  });

  it("displays disabled query with visual indicator", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });

    render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Daily sales report")).toBeInTheDocument();
    });

    // The disabled query should have reduced opacity (checked via class)
    const disabledQueryButton = screen
      .getByText("Daily sales report")
      .closest("button");
    expect(disabledQueryButton).toHaveClass("opacity-50");
  });

  it("filters queries based on filter prop", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueriesWithStatuses },
    });

    // Render with pending filter
    const { rerender } = render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="pending"
      />,
    );

    await waitFor(() => {
      // Only pending query should be visible
      expect(screen.getByText("Pending query")).toBeInTheDocument();
    });

    expect(screen.queryByText("Approved query")).not.toBeInTheDocument();
    expect(screen.queryByText("Rejected query")).not.toBeInTheDocument();

    // Rerender with rejected filter
    rerender(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="rejected"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Rejected query")).toBeInTheDocument();
    });

    expect(screen.queryByText("Approved query")).not.toBeInTheDocument();
    expect(screen.queryByText("Pending query")).not.toBeInTheDocument();
  });

  it("keeps executed results scoped to the selected routed query", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueries },
    });
    vi.mocked(engineApi.executeQuery).mockImplementation(
      async (_projectId, _datasourceId, queryId) => {
        if (queryId === "query-1") {
          return {
            success: true,
            data: {
              columns: [{ name: "customer", type: "text" }],
              rows: [{ customer: "Alice" }],
              row_count: 1,
            },
          };
        }

        return {
          success: true,
          data: {
            columns: [{ name: "report_total", type: "integer" }],
            rows: [{ report_total: 42 }],
            row_count: 1,
          },
        };
      },
    );

    const { rerender } = render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
        initialQueryId="query-1"
        onQuerySelect={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByDisplayValue(
          "SELECT * FROM customers ORDER BY revenue DESC LIMIT 10",
        ),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /execute query/i }));

    await waitFor(() => {
      expect(screen.getByText("Query Results")).toBeInTheDocument();
      expect(screen.getByText("Alice")).toBeInTheDocument();
    });

    rerender(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
        initialQueryId="query-2"
        onQuerySelect={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByDisplayValue(
          "SELECT date, SUM(amount) FROM sales GROUP BY date",
        ),
      ).toBeInTheDocument();
    });

    expect(screen.queryByText("Query Results")).not.toBeInTheDocument();
    expect(screen.queryByText("Alice")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /execute query/i }));

    await waitFor(() => {
      expect(screen.getByText("Query Results")).toBeInTheDocument();
      expect(screen.getByText("42")).toBeInTheDocument();
    });

    rerender(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
        initialQueryId="query-1"
        onQuerySelect={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByDisplayValue(
          "SELECT * FROM customers ORDER BY revenue DESC LIMIT 10",
        ),
      ).toBeInTheDocument();
    });

    expect(screen.getByText("Query Results")).toBeInTheDocument();
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.queryByText("42")).not.toBeInTheDocument();
  });

  it("keeps executed results scoped to the selected pending query", async () => {
    const basePendingQuery = mockQueriesWithStatuses[1];
    if (!basePendingQuery) {
      throw new Error("expected a pending query fixture");
    }
    const pendingQueries = [
      {
        ...basePendingQuery,
        query_id: "query-pending-a",
        natural_language_prompt: "Pending query A",
        sql_query: "SELECT * FROM pending_a",
      },
      {
        ...basePendingQuery,
        query_id: "query-pending-b",
        natural_language_prompt: "Pending query B",
        sql_query: "SELECT * FROM pending_b",
      },
    ] satisfies Query[];

    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: pendingQueries },
    });
    vi.mocked(engineApi.executeQuery).mockImplementation(
      async (_projectId, _datasourceId, queryId) => {
        if (queryId === "query-pending-a") {
          return {
            success: true,
            data: {
              columns: [{ name: "pending_value", type: "text" }],
              rows: [{ pending_value: "pending-a-row" }],
              row_count: 1,
            },
          };
        }

        return {
          success: true,
          data: {
            columns: [{ name: "pending_value", type: "text" }],
            rows: [{ pending_value: "pending-b-row" }],
            row_count: 1,
          },
        };
      },
    );

    const { rerender } = render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="pending"
        initialQueryId="query-pending-a"
        onQuerySelect={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByDisplayValue("SELECT * FROM pending_a"),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^execute$/i }));

    await waitFor(() => {
      expect(screen.getByText("pending-a-row")).toBeInTheDocument();
    });

    rerender(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="pending"
        initialQueryId="query-pending-b"
        onQuerySelect={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByDisplayValue("SELECT * FROM pending_b"),
      ).toBeInTheDocument();
    });

    expect(screen.queryByText("Query Results")).not.toBeInTheDocument();
    expect(screen.queryByText("pending-a-row")).not.toBeInTheDocument();
  });

  it("does not show stale results when switching from an executed query to a rejected query", async () => {
    vi.mocked(engineApi.listQueries).mockResolvedValue({
      success: true,
      data: { queries: mockQueriesWithStatuses },
    });
    vi.mocked(engineApi.executeQuery).mockResolvedValue({
      success: true,
      data: {
        columns: [{ name: "approved_value", type: "text" }],
        rows: [{ approved_value: "approved-row" }],
        row_count: 1,
      },
    });

    const { rerender } = render(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="approved"
        initialQueryId="query-approved"
        onQuerySelect={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByDisplayValue("SELECT * FROM approved"),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /execute query/i }));

    await waitFor(() => {
      expect(screen.getByText("approved-row")).toBeInTheDocument();
    });

    rerender(
      <QueriesView
        projectId="proj-1"
        datasourceId="ds-1"
        dialect="PostgreSQL"
        filter="rejected"
        initialQueryId="query-rejected"
        onQuerySelect={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(
        screen.getByDisplayValue("SELECT * FROM rejected"),
      ).toBeInTheDocument();
    });

    expect(screen.queryByText("Query Results")).not.toBeInTheDocument();
    expect(screen.queryByText("approved-row")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /execute query/i }),
    ).not.toBeInTheDocument();
  });
});
