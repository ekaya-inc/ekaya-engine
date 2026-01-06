import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { POSTGRES_PROVIDERS, type ProviderInfo } from "../../constants/adapters";
import DatasourceConfiguration from "../DatasourceConfiguration";

// Mock the DatasourceConnectionContext
const mockTestConnection = vi.fn();
const mockSaveDataSource = vi.fn();
const mockUpdateDataSource = vi.fn();
const mockDeleteDataSource = vi.fn();
const mockClearError = vi.fn();

const mockUseDatasourceConnection = vi.fn();
vi.mock("../../contexts/DatasourceConnectionContext", () => ({
  useDatasourceConnection: () => mockUseDatasourceConnection(),
}));

// Mock the toast hook
const mockToast = vi.fn();
vi.mock("../../hooks/useToast", () => ({
  useToast: () => ({ toast: mockToast }),
}));

// Mock useNavigate
const mockNavigate = vi.fn();
vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual("react-router-dom");
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

// Get provider by ID helper
const getProvider = (id: string): ProviderInfo | undefined =>
  POSTGRES_PROVIDERS.find((p) => p.id === id);

interface RenderProps {
  selectedAdapter?: string | null;
  selectedProvider?: ProviderInfo | undefined;
  onBackToSelection?: () => void;
}

const renderComponent = (props: RenderProps = {}) => {
  const defaultProps = {
    selectedAdapter: "postgres",
    onBackToSelection: vi.fn(),
    ...props,
  };

  return render(
    <MemoryRouter initialEntries={["/projects/test-project/datasources"]}>
      <Routes>
        <Route
          path="/projects/:pid/datasources"
          element={<DatasourceConfiguration {...defaultProps} />}
        />
      </Routes>
    </MemoryRouter>
  );
};

describe("DatasourceConfiguration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseDatasourceConnection.mockReturnValue({
      testConnection: mockTestConnection,
      connectionStatus: null,
      error: null,
      isConnected: false,
      connectionDetails: null,
      selectedDatasource: null,
      clearError: mockClearError,
      saveDataSource: mockSaveDataSource,
      updateDataSource: mockUpdateDataSource,
      deleteDataSource: mockDeleteDataSource,
    });
  });

  describe("Connection String Parser", () => {
    it("renders connection string input for postgres adapter", () => {
      renderComponent({ selectedAdapter: "postgres" });

      expect(
        screen.getByText("Quick Setup: Paste Connection String")
      ).toBeInTheDocument();
      expect(
        screen.getByPlaceholderText(
          "postgresql://user:password@host:port/database"
        )
      ).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Parse" })).toBeInTheDocument();
    });

    it("does not render connection string input for non-postgres adapter", () => {
      renderComponent({ selectedAdapter: "mysql" });

      expect(
        screen.queryByText("Quick Setup: Paste Connection String")
      ).not.toBeInTheDocument();
    });

    it("parses a valid PostgreSQL connection string", () => {
      renderComponent({ selectedAdapter: "postgres" });

      const input = screen.getByPlaceholderText(
        "postgresql://user:password@host:port/database"
      );
      fireEvent.change(input, {
        target: { value: "postgresql://myuser:mypass@localhost:5432/mydb" },
      });
      fireEvent.click(screen.getByRole("button", { name: "Parse" }));

      // Check that form fields were populated
      expect(screen.getByLabelText("Host")).toHaveValue("localhost");
      expect(screen.getByLabelText("Port")).toHaveValue(5432);
      expect(screen.getByLabelText("Username")).toHaveValue("myuser");
      expect(screen.getByLabelText("Password")).toHaveValue("mypass");
      expect(screen.getByLabelText("Database Name")).toHaveValue("mydb");
    });

    it("shows error for invalid connection string", () => {
      renderComponent({ selectedAdapter: "postgres" });

      const input = screen.getByPlaceholderText(
        "postgresql://user:password@host:port/database"
      );
      fireEvent.change(input, {
        target: { value: "invalid-connection-string" },
      });
      fireEvent.click(screen.getByRole("button", { name: "Parse" }));

      expect(
        screen.getByText(/Invalid connection string format/i)
      ).toBeInTheDocument();
    });

    it("shows error when connection string is empty", () => {
      renderComponent({ selectedAdapter: "postgres" });

      fireEvent.click(screen.getByRole("button", { name: "Parse" }));

      expect(
        screen.getByText("Please enter a connection string")
      ).toBeInTheDocument();
    });

    it("auto-detects Supabase provider from connection string", () => {
      renderComponent({ selectedAdapter: "postgres" });

      const input = screen.getByPlaceholderText(
        "postgresql://user:password@host:port/database"
      );
      fireEvent.change(input, {
        target: {
          value:
            "postgresql://postgres.abcdefgh:password@aws-0-us-west-1.pooler.supabase.com:6543/postgres",
        },
      });
      fireEvent.click(screen.getByRole("button", { name: "Parse" }));

      // Check that Supabase-specific values are used
      expect(screen.getByLabelText("Port")).toHaveValue(6543);
      // Page title should show Supabase
      expect(screen.getByText("Configure Supabase")).toBeInTheDocument();
    });

    it("auto-detects Neon provider from connection string", () => {
      renderComponent({ selectedAdapter: "postgres" });

      const input = screen.getByPlaceholderText(
        "postgresql://user:password@host:port/database"
      );
      fireEvent.change(input, {
        target: {
          value:
            "postgresql://myuser:mypass@ep-cool-darkness-123456.us-east-1.aws.neon.tech/neondb",
        },
      });
      fireEvent.click(screen.getByRole("button", { name: "Parse" }));

      // Check that the page title shows Neon
      expect(screen.getByText("Configure Neon")).toBeInTheDocument();
    });

    it("clears connection string input after successful parse", () => {
      renderComponent({ selectedAdapter: "postgres" });

      const input = screen.getByPlaceholderText(
        "postgresql://user:password@host:port/database"
      ) as HTMLInputElement;
      fireEvent.change(input, {
        target: { value: "postgresql://myuser:mypass@localhost:5432/mydb" },
      });
      fireEvent.click(screen.getByRole("button", { name: "Parse" }));

      expect(input.value).toBe("");
    });
  });

  describe("Provider-specific defaults", () => {
    it("sets default port from provider", () => {
      const supabaseProvider = getProvider("supabase");
      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: supabaseProvider,
      });

      expect(screen.getByLabelText("Port")).toHaveValue(6543);
    });

    it("sets default port for CockroachDB", () => {
      const cockroachProvider = getProvider("cockroachdb");
      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: cockroachProvider,
      });

      expect(screen.getByLabelText("Port")).toHaveValue(26257);
    });

    it("sets default port for YugabyteDB", () => {
      const yugabyteProvider = getProvider("yugabytedb");
      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: yugabyteProvider,
      });

      expect(screen.getByLabelText("Port")).toHaveValue(5433);
    });

    it("sets default SSL enabled for Supabase", () => {
      const supabaseProvider = getProvider("supabase");
      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: supabaseProvider,
      });

      // SSL switch should be checked
      expect(screen.getByRole("switch", { name: "Use SSL" })).toBeChecked();
    });
  });

  describe("Provider help text and links", () => {
    it("displays connection string help for Supabase", () => {
      const supabaseProvider = getProvider("supabase");
      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: supabaseProvider,
      });

      expect(
        screen.getByText(
          "Find in: Project Settings → Database → Connection string"
        )
      ).toBeInTheDocument();
    });

    it("displays connection string help for Neon", () => {
      const neonProvider = getProvider("neon");
      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: neonProvider,
      });

      expect(
        screen.getByText("Find in: Dashboard → Connection Details")
      ).toBeInTheDocument();
    });

    it("displays documentation link when helpUrl is available", () => {
      const supabaseProvider = getProvider("supabase");
      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: supabaseProvider,
      });

      const docLink = screen.getByRole("link", { name: /view documentation/i });
      expect(docLink).toBeInTheDocument();
      expect(docLink).toHaveAttribute(
        "href",
        "https://supabase.com/docs/guides/database/connecting-to-postgres"
      );
      expect(docLink).toHaveAttribute("target", "_blank");
    });
  });

  describe("Provider persistence in config", () => {
    it("includes provider in saved config", async () => {
      const supabaseProvider = getProvider("supabase");
      mockTestConnection.mockResolvedValue({ success: true });
      mockSaveDataSource.mockResolvedValue({ success: true });

      // Set up successful connection status
      mockUseDatasourceConnection.mockReturnValue({
        testConnection: mockTestConnection,
        connectionStatus: { success: true, message: "Connected" },
        error: null,
        isConnected: false,
        connectionDetails: null,
        selectedDatasource: null,
        clearError: mockClearError,
        saveDataSource: mockSaveDataSource,
        updateDataSource: mockUpdateDataSource,
        deleteDataSource: mockDeleteDataSource,
      });

      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: supabaseProvider,
      });

      // Fill in required fields
      fireEvent.change(screen.getByLabelText("Host"), {
        target: { value: "test.supabase.com" },
      });
      fireEvent.change(screen.getByLabelText("Username"), {
        target: { value: "postgres" },
      });
      fireEvent.change(screen.getByLabelText("Password"), {
        target: { value: "password" },
      });
      fireEvent.change(screen.getByLabelText("Database Name"), {
        target: { value: "postgres" },
      });

      // Click save button
      fireEvent.click(screen.getByRole("button", { name: /save datasource/i }));

      await waitFor(() => {
        expect(mockSaveDataSource).toHaveBeenCalledWith(
          "test-project",
          "Supabase",
          "postgres",
          expect.objectContaining({
            provider: "supabase",
            host: "test.supabase.com",
            port: 6543,
          })
        );
      });
    });
  });

  describe("Page header display", () => {
    it("shows PostgreSQL in header when no provider is selected", () => {
      renderComponent({ selectedAdapter: "postgres" });

      expect(screen.getByText("Configure PostgreSQL")).toBeInTheDocument();
    });

    it("shows provider name in header when provider is selected", () => {
      const supabaseProvider = getProvider("supabase");
      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: supabaseProvider,
      });

      expect(screen.getByText("Configure Supabase")).toBeInTheDocument();
    });

    it("shows provider name as default datasource name", () => {
      const neonProvider = getProvider("neon");
      renderComponent({
        selectedAdapter: "postgres",
        selectedProvider: neonProvider,
      });

      // The datasource name button should show Neon
      expect(
        screen.getByRole("button", { name: /Neon/i })
      ).toBeInTheDocument();
    });
  });

  describe("Editing existing datasource", () => {
    it("does not show connection string parser when editing", () => {
      mockUseDatasourceConnection.mockReturnValue({
        testConnection: mockTestConnection,
        connectionStatus: null,
        error: null,
        isConnected: true,
        connectionDetails: {
          datasourceId: "ds-1",
          type: "postgres",
          host: "localhost",
          port: 5432,
          user: "test",
          name: "testdb",
          ssl_mode: "require",
          displayName: "My Database",
        },
        selectedDatasource: {
          datasourceId: "ds-1",
        },
        clearError: mockClearError,
        saveDataSource: mockSaveDataSource,
        updateDataSource: mockUpdateDataSource,
        deleteDataSource: mockDeleteDataSource,
      });

      renderComponent({ selectedAdapter: "postgres" });

      // Connection string section should not be visible when editing
      expect(
        screen.queryByText("Quick Setup: Paste Connection String")
      ).not.toBeInTheDocument();
    });

    it("loads provider from existing config when editing", () => {
      mockUseDatasourceConnection.mockReturnValue({
        testConnection: mockTestConnection,
        connectionStatus: null,
        error: null,
        isConnected: true,
        connectionDetails: {
          datasourceId: "ds-1",
          type: "postgres",
          provider: "neon",
          host: "ep-cool.neon.tech",
          port: 5432,
          user: "neonuser",
          name: "neondb",
          ssl_mode: "require",
          displayName: "My Neon DB",
        },
        selectedDatasource: {
          datasourceId: "ds-1",
        },
        clearError: mockClearError,
        saveDataSource: mockSaveDataSource,
        updateDataSource: mockUpdateDataSource,
        deleteDataSource: mockDeleteDataSource,
      });

      renderComponent({ selectedAdapter: "postgres" });

      // Should show Neon in the header
      expect(screen.getByText("Configure Neon")).toBeInTheDocument();
    });
  });
});
