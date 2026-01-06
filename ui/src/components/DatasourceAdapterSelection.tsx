import { ArrowLeft, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import {
  ADAPTER_ICON_PATHS,
  POSTGRES_PROVIDERS,
  type ProviderInfo,
} from "../constants/adapters";
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

interface DatasourceAdapterSelectionProps {
  selectedAdapter: string | null;
  onAdapterSelect: (adapterId: string, provider?: ProviderInfo) => void;
  datasources?: ConnectionDetails[];
}

const DatasourceAdapterSelection = ({
  selectedAdapter,
  onAdapterSelect,
  datasources = [],
}: DatasourceAdapterSelectionProps) => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const [availableAdapters, setAvailableAdapters] = useState<
    DatasourceAdapter[]
  >([]);
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
          icon: ADAPTER_ICON_PATHS[t.icon] ?? null,
        }));
        setAvailableAdapters(adapters);
      } catch (err) {
        console.error("Failed to load adapter types:", err);
        setError(
          err instanceof Error ? err.message : "Failed to load adapters"
        );
      } finally {
        setLoading(false);
      }
    };
    void loadAdapters();
  }, []);

  const getConnectedAdapterTypes = (): Set<string> => {
    return new Set(datasources.map((ds) => ds.type));
  };

  const getAdapterDisabledState = (adapterType: string): boolean => {
    // If there are existing connections, only allow the same adapter type
    const connectedTypes = getConnectedAdapterTypes();
    if (connectedTypes.size > 0) {
      return !connectedTypes.has(adapterType);
    }
    return false;
  };

  // Check if postgres adapter is available from the API
  const hasPostgresAdapter = availableAdapters.some((a) => a.id === "postgres");

  // Get non-postgres adapters for the "Other Databases" section
  const otherAdapters = availableAdapters.filter((a) => a.id !== "postgres");

  const handleProviderSelect = (provider: ProviderInfo) => {
    // Pass the provider's adapter type (postgres) and the provider info
    onAdapterSelect(provider.adapterType, provider);
  };

  const handleAdapterSelect = (adapter: DatasourceAdapter) => {
    onAdapterSelect(adapter.id);
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
              Select Your Database
            </h1>
            <p className="text-text-secondary">
              Choose your database provider to get started
            </p>
          </div>
        </div>
      </div>

      {/* PostgreSQL-Compatible Databases Section */}
      {hasPostgresAdapter && (
        <div className="mb-8">
          <h2 className="text-lg font-semibold text-text-primary mb-4">
            PostgreSQL-Compatible
          </h2>
          <div className="grid gap-4 md:grid-cols-3">
            {POSTGRES_PROVIDERS.map((provider) => {
              const isDisabled = getAdapterDisabledState(provider.adapterType);
              const isSelected = selectedAdapter === provider.adapterType;

              return (
                <Card
                  key={provider.id}
                  className={`transition-all ${
                    isDisabled
                      ? "cursor-not-allowed opacity-50 bg-gray-100 dark:bg-gray-800"
                      : "cursor-pointer hover:shadow-md"
                  } ${
                    isSelected
                      ? "ring-2 ring-blue-500 bg-blue-50 dark:bg-blue-950"
                      : ""
                  }`}
                  onClick={() => !isDisabled && handleProviderSelect(provider)}
                >
                  <CardContent className="p-4">
                    <div className="flex items-center gap-3">
                      {provider.icon ? (
                        <img
                          src={provider.icon}
                          alt={provider.name}
                          className={`h-10 w-10 object-contain ${
                            isDisabled ? "grayscale" : ""
                          }`}
                        />
                      ) : (
                        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-gray-200 dark:bg-gray-700">
                          <span className="text-lg font-bold text-gray-500 dark:text-gray-400">
                            ?
                          </span>
                        </div>
                      )}
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <h3
                            className={`font-semibold truncate ${
                              isDisabled
                                ? "text-text-tertiary"
                                : "text-text-primary"
                            }`}
                          >
                            {provider.name}
                          </h3>
                        </div>
                        {provider.connectionStringHelp && (
                          <p
                            className={`text-xs truncate ${
                              isDisabled
                                ? "text-text-tertiary"
                                : "text-text-secondary"
                            }`}
                          >
                            {provider.connectionStringHelp}
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
      )}

      {/* Other Databases Section */}
      {otherAdapters.length > 0 && (
        <div>
          <h2 className="text-lg font-semibold text-text-primary mb-4">
            Other Databases
          </h2>
          <div className="grid gap-4 md:grid-cols-3">
            {otherAdapters.map((adapter) => {
              const isDisabled = getAdapterDisabledState(adapter.id);
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
                  onClick={() => !isDisabled && handleAdapterSelect(adapter)}
                >
                  <CardContent className="p-4">
                    <div className="flex items-center gap-3">
                      {adapter.icon ? (
                        <img
                          src={adapter.icon}
                          alt={adapter.name}
                          className={`h-10 w-10 object-contain ${
                            isDisabled ? "grayscale" : ""
                          }`}
                        />
                      ) : (
                        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-gray-200 dark:bg-gray-700">
                          <span className="text-lg font-bold text-gray-500 dark:text-gray-400">
                            ?
                          </span>
                        </div>
                      )}
                      <div className="flex-1 min-w-0">
                        <h3
                          className={`font-semibold truncate ${
                            isDisabled
                              ? "text-text-tertiary"
                              : "text-text-primary"
                          }`}
                        >
                          {adapter.name}
                        </h3>
                        <p
                          className={`text-xs truncate ${
                            isDisabled
                              ? "text-text-tertiary"
                              : "text-text-secondary"
                          }`}
                        >
                          {adapter.description}
                        </p>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};

export default DatasourceAdapterSelection;
