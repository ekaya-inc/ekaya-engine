import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { POSTGRES_PROVIDERS, type ProviderInfo } from "../../constants/adapters";
import type { ConnectionDetails } from "../../types";
import DatasourceAdapterSelection from "../DatasourceAdapterSelection";

// Mock fetch for loading adapters from API
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Mock useNavigate
const mockNavigate = vi.fn();
vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual("react-router-dom");
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

const mockAdaptersResponse = [
  {
    type: "postgres",
    display_name: "PostgreSQL",
    description: "PostgreSQL database",
    icon: "postgres",
  },
  {
    type: "mysql",
    display_name: "MySQL",
    description: "MySQL database",
    icon: "mysql",
  },
  {
    type: "clickhouse",
    display_name: "ClickHouse",
    description: "ClickHouse database",
    icon: "clickhouse",
  },
];

interface RenderProps {
  selectedAdapter?: string | null;
  onAdapterSelect?: (adapterId: string, provider?: ProviderInfo) => void;
  datasources?: ConnectionDetails[];
}

const renderComponent = (props: RenderProps = {}) => {
  const defaultProps = {
    selectedAdapter: null,
    onAdapterSelect: vi.fn(),
    datasources: [],
    ...props,
  };

  return render(
    <MemoryRouter initialEntries={["/projects/test-project/datasources"]}>
      <Routes>
        <Route
          path="/projects/:pid/datasources"
          element={<DatasourceAdapterSelection {...defaultProps} />}
        />
      </Routes>
    </MemoryRouter>
  );
};

describe("DatasourceAdapterSelection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockAdaptersResponse),
    });
  });

  it("renders loading state initially", () => {
    mockFetch.mockReturnValue(new Promise(() => {}));
    renderComponent();
    expect(screen.getByText(/loading adapters/i)).toBeInTheDocument();
  });

  it("renders error state on fetch failure", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      statusText: "Internal Server Error",
    });

    renderComponent();

    await waitFor(() => {
      expect(
        screen.getByText(/failed to load adapters: internal server error/i)
      ).toBeInTheDocument();
    });
  });

  it("renders PostgreSQL-Compatible section header when postgres adapter is available", async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByText("PostgreSQL-Compatible")).toBeInTheDocument();
    });
  });

  it("renders all PostgreSQL providers when postgres adapter is available", async () => {
    renderComponent();

    await waitFor(() => {
      // Check that all providers from POSTGRES_PROVIDERS are rendered
      for (const provider of POSTGRES_PROVIDERS) {
        expect(screen.getByText(provider.name)).toBeInTheDocument();
      }
    });
  });

  it("renders Other Databases section header when non-postgres adapters are available", async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByText("Other Databases")).toBeInTheDocument();
    });
  });

  it("renders non-postgres adapters in Other Databases section", async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByText("MySQL")).toBeInTheDocument();
      expect(screen.getByText("ClickHouse")).toBeInTheDocument();
    });
  });

  it("calls onAdapterSelect with provider info when a PostgreSQL provider is clicked", async () => {
    const onAdapterSelect = vi.fn();
    renderComponent({ onAdapterSelect });

    await waitFor(() => {
      expect(screen.getByText("Supabase")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Supabase"));

    expect(onAdapterSelect).toHaveBeenCalledTimes(1);
    expect(onAdapterSelect).toHaveBeenCalledWith(
      "postgres",
      expect.objectContaining({
        id: "supabase",
        name: "Supabase",
        adapterType: "postgres",
        defaultPort: 6543,
      })
    );
  });

  it("calls onAdapterSelect with just adapter ID when a non-postgres adapter is clicked", async () => {
    const onAdapterSelect = vi.fn();
    renderComponent({ onAdapterSelect });

    await waitFor(() => {
      expect(screen.getByText("MySQL")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("MySQL"));

    expect(onAdapterSelect).toHaveBeenCalledTimes(1);
    expect(onAdapterSelect).toHaveBeenCalledWith("mysql");
  });

  it("does not render PostgreSQL-Compatible section when postgres adapter is not available", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve([
          {
            type: "mysql",
            display_name: "MySQL",
            description: "MySQL database",
            icon: "mysql",
          },
        ]),
    });

    renderComponent();

    await waitFor(() => {
      expect(screen.getByText("MySQL")).toBeInTheDocument();
    });

    expect(screen.queryByText("PostgreSQL-Compatible")).not.toBeInTheDocument();
    // PostgreSQL providers should not be rendered
    expect(screen.queryByText("Supabase")).not.toBeInTheDocument();
    expect(screen.queryByText("Neon")).not.toBeInTheDocument();
  });

  it("navigates back when back button is clicked", async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByText("PostgreSQL-Compatible")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /back/i }));

    expect(mockNavigate).toHaveBeenCalledWith("/projects/test-project");
  });

  it("displays connection string help text for providers that have it", async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByText("Supabase")).toBeInTheDocument();
    });

    // Supabase has connectionStringHelp
    expect(
      screen.getByText("Find in: Project Settings → Database → Connection string")
    ).toBeInTheDocument();
  });

  it("shows correct page title and subtitle", async () => {
    renderComponent();

    await waitFor(() => {
      expect(screen.getByText("Select Your Database")).toBeInTheDocument();
      expect(
        screen.getByText("Choose your database provider to get started")
      ).toBeInTheDocument();
    });
  });

  describe("disabled state with existing datasources", () => {
    it("disables non-matching adapter types when datasources exist", async () => {
      renderComponent({
        datasources: [
          {
            type: "mysql",
            host: "localhost",
            port: 3306,
            name: "test_db",
            ssl_mode: "disable",
          },
        ],
      });

      await waitFor(() => {
        expect(screen.getByText("PostgreSQL")).toBeInTheDocument();
      });

      // PostgreSQL providers should be disabled (shown with opacity-50)
      const postgresCard = screen.getByText("PostgreSQL").closest(".cursor-not-allowed");
      expect(postgresCard).toBeInTheDocument();

      // MySQL should be clickable
      const mysqlCard = screen.getByText("MySQL").closest(".cursor-pointer");
      expect(mysqlCard).toBeInTheDocument();
    });

    it("does not call onAdapterSelect when clicking disabled provider", async () => {
      const onAdapterSelect = vi.fn();
      renderComponent({
        onAdapterSelect,
        datasources: [
          {
            type: "mysql",
            host: "localhost",
            port: 3306,
            name: "test_db",
            ssl_mode: "disable",
          },
        ],
      });

      await waitFor(() => {
        expect(screen.getByText("PostgreSQL")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByText("PostgreSQL"));

      expect(onAdapterSelect).not.toHaveBeenCalled();
    });
  });
});
