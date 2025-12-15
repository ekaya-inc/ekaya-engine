import { ArrowLeft, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import type { ConnectionDetails, DatasourceType } from "../types";

import { Button } from "./ui/Button";
import { Card, CardContent } from "./ui/Card";

interface DatasourceAdapter {
  id: DatasourceType;
  name: string;
  description: string;
  icon: string | null;
}

interface DatasourceTypeFromAPI {
  type: string;
  display_name: string;
  description: string;
  icon: string;
}

// Map icon identifier from API to actual image path
const ICON_PATHS: Record<string, string> = {
  postgres: "/icons/adapters/PostgreSQL.png",
  mssql: "/icons/adapters/MSSQL.png",
  clickhouse: "/icons/adapters/ClickHouse.png",
  mysql: "/icons/adapters/MySQL.png",
  snowflake: "/icons/adapters/Snowflake.png",
  bigquery: "/icons/adapters/BigQuery.png",
  databricks: "/icons/adapters/Databricks.png",
  redshift: "/icons/adapters/AmazonRedshift.png",
};

interface DatasourceAdapterSelectionProps {
  selectedAdapter: string | null;
  onAdapterSelect: (adapterId: string) => void;
  datasources?: ConnectionDetails[];
}

const DatasourceAdapterSelection = ({
  selectedAdapter,
  onAdapterSelect,
  datasources = [],
}: DatasourceAdapterSelectionProps) => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const [availableAdapters, setAvailableAdapters] = useState<DatasourceAdapter[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const loadAdapters = async () => {
      try {
        const response = await fetch("/api/config/datasource-types");
        if (!response.ok) {
          throw new Error(`Failed to load adapters: ${response.statusText}`);
        }
        const types: DatasourceTypeFromAPI[] = await response.json();
        const adapters = types.map((t) => ({
          id: t.type as DatasourceType,
          name: t.display_name,
          description: t.description,
          icon: ICON_PATHS[t.icon] ?? null,
        }));
        setAvailableAdapters(adapters);
      } catch (err) {
        console.error("Failed to load adapter types:", err);
        setError(err instanceof Error ? err.message : "Failed to load adapters");
      } finally {
        setLoading(false);
      }
    };
    void loadAdapters();
  }, []);

  const getConnectedAdapterTypes = (): Set<string> => {
    return new Set(datasources.map((ds) => ds.type));
  };

  const getAdapterDisabledState = (adapter: DatasourceAdapter): boolean => {
    // If there are existing connections, only allow the same adapter type
    const connectedTypes = getConnectedAdapterTypes();
    if (connectedTypes.size > 0) {
      return !connectedTypes.has(adapter.id);
    }
    return false;
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-6xl">
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-blue-600" />
          <span className="ml-3 text-text-secondary">Loading adapters...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="mx-auto max-w-6xl">
        <div className="p-8 text-center">
          <p className="text-red-600 dark:text-red-400">{error}</p>
          <Button
            variant="outline"
            onClick={() => window.location.reload()}
            className="mt-4"
          >
            Retry
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-6xl">
      <div className="mb-8">
        <Button
          variant="ghost"
          onClick={() => navigate(`/projects/${pid}`)}
          className="mb-4"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back
        </Button>
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-text-primary mb-2">
              Select Your Datasource Adapter
            </h1>
          </div>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        {availableAdapters.map((adapter) => {
          const isDisabled = getAdapterDisabledState(adapter);
          const isSelected = selectedAdapter === adapter.id;

          return (
            <Card
              key={adapter.id}
              className={`transition-all ${
                isDisabled
                  ? "cursor-not-allowed opacity-50 bg-gray-100 dark:bg-gray-800"
                  : "cursor-pointer hover:shadow-md"
              } ${
                isSelected
                  ? "ring-2 ring-blue-500 bg-blue-50 dark:bg-blue-950"
                  : ""
              }`}
              onClick={() => !isDisabled && onAdapterSelect(adapter.id)}
            >
              <CardContent className="p-6">
                <div className="flex items-start gap-4">
                  {adapter.icon ? (
                    <img
                      src={adapter.icon}
                      alt={adapter.name}
                      className={`h-12 w-12 object-contain ${
                        isDisabled ? "grayscale" : ""
                      }`}
                    />
                  ) : (
                    <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-gray-200 dark:bg-gray-700">
                      <span className="text-xl font-bold text-gray-500 dark:text-gray-400">
                        ?
                      </span>
                    </div>
                  )}
                  <div className="flex-1">
                    <div className="flex items-center gap-2 mb-1">
                      <h3
                        className={`font-semibold ${
                          isDisabled
                            ? "text-text-tertiary"
                            : "text-text-primary"
                        }`}
                      >
                        {adapter.name}
                      </h3>
                      {isDisabled && (
                        <span className="text-xs bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-400 px-2 py-1 rounded">
                          No Connection
                        </span>
                      )}
                    </div>
                    <p
                      className={`text-sm ${
                        isDisabled
                          ? "text-text-tertiary"
                          : "text-text-secondary"
                      }`}
                    >
                      {adapter.description}
                    </p>
                    {isDisabled && (
                      <p className="text-xs text-text-tertiary mt-2">
                        No active connections for this adapter
                      </p>
                    )}
                  </div>
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
};

export default DatasourceAdapterSelection;
