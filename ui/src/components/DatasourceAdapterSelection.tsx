import { ArrowLeft } from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";

import type { ConnectionDetails, DatasourceType } from "../types";

import { Button } from "./ui/Button";
import { Card, CardContent } from "./ui/Card";

interface DatasourceAdapter {
  id: DatasourceType | "other";
  name: string;
  description: string;
  icon: string | null;
  supported: boolean;
}

const datasourceAdapters: DatasourceAdapter[] = [
  {
    id: "mssql",
    name: "Microsoft SQL Server",
    description: "Enterprise database platform",
    icon: "/icons/adapters/MSSQL.png",
    supported: true,
  },
  {
    id: "postgres",
    name: "PostgreSQL",
    description: "Advanced relational database",
    icon: "/icons/adapters/PostgreSQL.png",
    supported: true,
  },
  {
    id: "clickhouse",
    name: "ClickHouse",
    description: "High-performance analytics database",
    icon: "/icons/adapters/ClickHouse.png",
    supported: true,
  },
  {
    id: "snowflake",
    name: "Snowflake",
    description: "Cloud data platform",
    icon: "/icons/adapters/Snowflake.png",
    supported: false,
  },
  {
    id: "bigquery",
    name: "Google BigQuery",
    description: "Serverless data warehouse",
    icon: "/icons/adapters/BigQuery.png",
    supported: false,
  },
  {
    id: "databricks",
    name: "Databricks",
    description: "Unified analytics platform",
    icon: "/icons/adapters/Databricks.png",
    supported: false,
  },
  {
    id: "redshift",
    name: "Amazon Redshift",
    description: "Cloud data warehouse",
    icon: "/icons/adapters/AmazonRedshift.png",
    supported: false,
  },
  {
    id: "mysql",
    name: "MySQL",
    description: "Popular open source database",
    icon: "/icons/adapters/MySQL.png",
    supported: false,
  },
  {
    id: "other",
    name: "ODBC/Other",
    description: "Custom database adapter",
    icon: null,
    supported: false,
  },
];

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
  const getConnectedAdapterTypes = (): Set<string> => {
    return new Set(datasources.map((ds) => ds.type));
  };

  const getAdapterDisabledState = (adapter: DatasourceAdapter): boolean => {
    // First check if adapter is supported
    if (!adapter.supported) {
      return true; // Always disable unsupported adapters
    }

    // For supported adapters, apply existing connection logic
    const connectedTypes = getConnectedAdapterTypes();
    if (connectedTypes.size > 0) {
      return !connectedTypes.has(adapter.id);
    }
    return false;
  };

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
        {datasourceAdapters.map((adapter) => {
          const isDisabled = getAdapterDisabledState(adapter);
          const isSelected = selectedAdapter === adapter.id;
          const isUnsupported = !adapter.supported;

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
                          {isUnsupported ? "Coming Soon" : "No Connection"}
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
                        {isUnsupported
                          ? "This adapter is not yet supported"
                          : "No active connections for this adapter"}
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
