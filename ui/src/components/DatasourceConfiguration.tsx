import { ArrowLeft, CheckCircle, XCircle, Loader2 } from "lucide-react";
import { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";

import { useDatasourceConnection } from "../contexts/DatasourceConnectionContext";
import { useToast } from "../hooks/useToast";
import sdapApi from "../services/sdapApi";
import type { DatasourceType, SSLMode } from "../types";

import { Button } from "./ui/Button";
import { Card, CardContent } from "./ui/Card";
import { Input } from "./ui/Input";
import { Label } from "./ui/Label";
import { Switch } from "./ui/Switch";

const TYPE_MAPPING: Record<string, DatasourceType> = {
  postgresql: "postgres",
  mysql: "mysql",
  clickhouse: "clickhouse",
  mssql: "mssql",
  snowflake: "snowflake",
  bigquery: "bigquery",
  databricks: "databricks",
  redshift: "redshift",
};

interface AdapterInfo {
  name: string;
  icon: string | null;
  description?: string;
}

const getAdapterInfo = (adapterId?: string): AdapterInfo => {
  const postgresInfo: AdapterInfo = {
    name: "PostgreSQL",
    icon: "/icons/adapters/PostgreSQL.png",
  };
  const adapterMap: Record<string, AdapterInfo> = {
    postgres: postgresInfo,
    postgresql: postgresInfo,
    mysql: { name: "MySQL", icon: "/icons/adapters/MySQL.png" },
    mssql: {
      name: "Microsoft SQL Server",
      icon: "/icons/adapters/MSSQL.png",
    },
    clickhouse: {
      name: "ClickHouse",
      icon: "/icons/adapters/ClickHouse.png",
    },
    snowflake: { name: "Snowflake", icon: "/icons/adapters/Snowflake.png" },
    bigquery: {
      name: "Google BigQuery",
      icon: "/icons/adapters/BigQuery.png",
    },
    databricks: {
      name: "Databricks",
      icon: "/icons/adapters/Databricks.png",
    },
    redshift: {
      name: "Amazon Redshift",
      icon: "/icons/adapters/AmazonRedshift.png",
    },
  };

  return adapterMap[adapterId ?? ""] ?? { name: "Datasource", icon: null };
};

interface DatasourceFormConfig {
  host: string;
  port: string;
  user: string;
  password: string;
  name: string;
  useSSL: boolean;
}

interface DatasourceConfigurationProps {
  selectedAdapter: string | null;
  onBackToSelection: () => void;
}

const DatasourceConfiguration = ({
  selectedAdapter,
  onBackToSelection,
}: DatasourceConfigurationProps) => {
  const { pid } = useParams<{ pid: string }>();
  const navigate = useNavigate();
  const { toast } = useToast();
  const {
    testConnection,
    connectionStatus,
    error,
    isConnected,
    connectionDetails,
    selectedDatasource,
    clearError,
    saveDataSource,
    updateDataSource,
    deleteDataSource,
  } = useDatasourceConnection();

  const [testingConnection, setTestingConnection] = useState<boolean>(false);
  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [isDisconnecting, setIsDisconnecting] = useState<boolean>(false);
  const [config, setConfig] = useState<DatasourceFormConfig>({
    host: "",
    port: "5432",
    user: "",
    password: "",
    name: "",
    useSSL: false,
  });

  const adapterInfo = getAdapterInfo(
    selectedAdapter ?? connectionDetails?.type
  );

  // Determine if this is editing an existing datasource or configuring a new one
  const isEditingExisting = Boolean(
    selectedDatasource?.datasourceId || connectionDetails?.datasourceId
  );

  const handleConfigChange = (
    field: keyof DatasourceFormConfig,
    value: string | boolean
  ): void => {
    setConfig((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  useEffect(() => {
    if (connectionDetails) {
      const formData: DatasourceFormConfig = {
        host: connectionDetails.host || "",
        port: connectionDetails.port?.toString() || "5432",
        user: connectionDetails.user || "",
        password: connectionDetails.password || "",
        name: connectionDetails.name || "",
        useSSL:
          connectionDetails.ssl_mode === "require" ||
          connectionDetails.ssl_mode === "prefer",
      };

      setConfig(formData);
    }
  }, [connectionDetails]);

  const handleTestConnection = async (): Promise<void> => {
    clearError();
    setTestingConnection(true);

    try {
      const testDetails = {
        type: (TYPE_MAPPING[selectedAdapter ?? ""] ??
          selectedAdapter) as DatasourceType,
        host: config.host,
        port: parseInt(config.port),
        name: config.name,
        user: config.user,
        password: config.password,
        ssl_mode: (config.useSSL ? "require" : "disable") as SSLMode,
      };
      await testConnection(testDetails);
    } catch (error) {
      console.error("Connection test failed:", error);
    } finally {
      setTestingConnection(false);
    }
  };

  const handleDisconnect = async (): Promise<void> => {
    setIsDisconnecting(true);

    try {
      const projectId =
        connectionDetails?.projectId ?? selectedDatasource?.projectId;
      const datasourceId =
        connectionDetails?.datasourceId ?? selectedDatasource?.datasourceId;

      if (!projectId || !datasourceId) {
        throw new Error("Missing project or datasource ID");
      }

      const result = await deleteDataSource(projectId, datasourceId);
      if (result.success) {
        toast({
          title: "Success",
          description: "Datasource disconnected successfully!",
          variant: "success",
        });
        onBackToSelection();
      } else {
        toast({
          title: "Error",
          description: "Failed to disconnect datasource. Please try again.",
          variant: "destructive",
        });
      }
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : "Unknown error";
      console.error("Failed to disconnect datasource:", error);
      toast({
        title: "Error",
        description: `Failed to disconnect datasource: ${errorMessage}`,
        variant: "destructive",
      });
    } finally {
      setIsDisconnecting(false);
    }
  };

  const saveOrUpdateDataSource = async (): Promise<void> => {
    setIsSaving(true);

    try {
      const datasourceId =
        connectionDetails?.datasourceId ?? selectedDatasource?.datasourceId;
      const datasourceType = (TYPE_MAPPING[selectedAdapter ?? ""] ??
        selectedAdapter) as DatasourceType;
      const apiConfig = {
        type: datasourceType,
        host: config.host,
        port: parseInt(config.port),
        name: config.name,
        user: config.user,
        password: config.password,
        ssl_mode: (config.useSSL ? "require" : "disable") as SSLMode,
      };

      sdapApi.validateConnectionDetails(apiConfig);

      if (!pid) {
        throw new Error("Project ID not available from route");
      }

      const isEditing = datasourceId !== undefined && datasourceId !== null;

      const result = isEditing
        ? await updateDataSource(pid, datasourceId, datasourceType, apiConfig)
        : await saveDataSource(pid, datasourceType, apiConfig);

      if (result.success) {
        const action = isEditing ? "updated" : "saved";
        toast({
          title: "Success",
          description: `Datasource ${action} successfully!`,
          variant: "success",
        });
        navigate(`/projects/${pid}`);
      } else {
        toast({
          title: "Error",
          description: "Failed to save datasource. Please try again.",
          variant: "destructive",
        });
      }
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : "Unknown error";
      console.error("Failed to save datasource:", error);
      toast({
        title: "Error",
        description: `Failed to save datasource: ${errorMessage}`,
        variant: "destructive",
      });
    } finally {
      setIsSaving(false);
    }
  };

  const renderTestConnection = () => (
    <div className="border-t pt-6 mt-6">
      <h3 className="text-lg font-semibold text-text-primary mb-4">
        Test Connection
      </h3>

      <div className="mb-6">
        <Button
          onClick={handleTestConnection}
          disabled={
            testingConnection || !config.host || !config.user || !config.name
          }
          className="min-w-[160px] bg-blue-600 hover:bg-blue-700 text-white font-semibold"
          size="default"
        >
          {testingConnection ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Testing Connection...
            </>
          ) : (
            "Test Connection"
          )}
        </Button>
        {(testingConnection ||
          !config.host ||
          !config.user ||
          !config.name) && (
          <p className="text-sm text-text-secondary mt-2">
            {testingConnection
              ? "Please wait while we test your database connection..."
              : "Please fill in Host, Username, and Database Name to test connection"}
          </p>
        )}
      </div>

      {connectionStatus && (
        <div
          className={`p-4 rounded-lg border ${
            connectionStatus.success
              ? "bg-green-50 dark:bg-green-950 border-green-200 dark:border-green-800"
              : "bg-red-50 dark:bg-red-950 border-red-200 dark:border-red-800"
          }`}
        >
          <div
            className={`flex items-center gap-2 text-sm font-medium mb-2 ${
              connectionStatus.success
                ? "text-green-700 dark:text-green-400"
                : "text-red-700 dark:text-red-400"
            }`}
          >
            {connectionStatus.success ? (
              <CheckCircle className="w-4 h-4 text-green-500" />
            ) : (
              <XCircle className="w-4 h-4 text-red-500" />
            )}
            {connectionStatus.message}
          </div>

          {!connectionStatus.success && (
            <div className="mt-3">
              <p className="text-sm font-medium text-red-700 dark:text-red-400 mb-2">
                Troubleshooting:
              </p>
              <ul className="text-sm text-red-600 dark:text-red-300 space-y-1">
                <li className="flex items-start gap-2">
                  <span className="text-red-400 mt-0.5">•</span>
                  <span>
                    Verify that your database server is running and accessible
                  </span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-red-400 mt-0.5">•</span>
                  <span>
                    Check that your database connection details are correct
                  </span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-red-400 mt-0.5">•</span>
                  <span>Ensure the host and port information is accurate</span>
                </li>
                <li className="flex items-start gap-2">
                  <span className="text-red-400 mt-0.5">•</span>
                  <span>
                    Confirm your database allows connections from this network
                  </span>
                </li>
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );

  const renderDatasourceSetup = () => (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="host">Host</Label>
          <Input
            id="host"
            placeholder="localhost or IP address"
            value={config.host}
            onChange={(e) => handleConfigChange("host", e.target.value)}
          />
          <p className="text-sm text-text-secondary">
            Your database server&apos;s IP address or domain name.
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="port">Port</Label>
          <Input
            id="port"
            type="number"
            placeholder="5432"
            value={config.port}
            onChange={(e) => handleConfigChange("port", e.target.value)}
          />
          <p className="text-sm text-text-secondary">
            Your database server port.
          </p>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="user">Username</Label>
          <Input
            id="user"
            placeholder="Database user"
            value={config.user}
            onChange={(e) => handleConfigChange("user", e.target.value)}
          />
          <p className="text-sm text-text-secondary">
            The database user for the account that you want to use to connect to
            your database.
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            type="password"
            placeholder="Database password"
            value={config.password}
            onChange={(e) => handleConfigChange("password", e.target.value)}
          />
          <p className="text-sm text-text-secondary">
            The password for the user that you use to connect to the database.
          </p>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="name">Database Name</Label>
        <Input
          id="name"
          placeholder="Database name"
          value={config.name}
          onChange={(e) => handleConfigChange("name", e.target.value)}
          disabled={isEditingExisting}
        />
        <p className="text-sm text-text-secondary">
          {isEditingExisting
            ? "Database name cannot be changed after creation."
            : "The name of the database you want to connect to."}
        </p>
      </div>

      <div className="flex items-center space-x-2">
        <Switch
          id="useSSL"
          checked={config.useSSL}
          onCheckedChange={(checked) => handleConfigChange("useSSL", checked)}
        />
        <Label htmlFor="useSSL">Use SSL</Label>
      </div>

      {renderTestConnection()}
    </div>
  );

  const handleBack = (): void => {
    if (isEditingExisting) {
      navigate(`/projects/${pid}`);
    } else {
      onBackToSelection();
    }
  };

  return (
    <div className="mx-auto max-w-4xl">
      <div className="mb-8">
        <Button variant="ghost" onClick={handleBack} className="mb-4">
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back
        </Button>
        <div className="flex items-center gap-4 mb-4">
          {adapterInfo?.icon ? (
            <img
              src={adapterInfo.icon}
              alt={adapterInfo.name}
              className="h-12 w-12 object-contain"
            />
          ) : (
            <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-gray-200 dark:bg-gray-700">
              <span className="text-xl font-bold text-gray-500 dark:text-gray-400">
                ?
              </span>
            </div>
          )}
          <div>
            <h1 className="text-3xl font-bold text-text-primary">
              Configure {adapterInfo?.name}
            </h1>
            <p className="text-text-secondary">{adapterInfo?.description}</p>
          </div>
        </div>
      </div>

      <Card>
        <CardContent className="p-6">
          {error && (
            <div className="mb-4 p-4 bg-red-50 dark:bg-red-950 border border-red-200 dark:border-red-800 rounded-lg">
              <div className="flex items-center gap-2 text-sm font-medium text-red-700 dark:text-red-400">
                <XCircle className="w-4 h-4" />
                {error}
              </div>
            </div>
          )}
          {renderDatasourceSetup()}
        </CardContent>
      </Card>

      <div className="mt-8 flex justify-between items-center">
        <div className="flex gap-2">
          <Button variant="outline" onClick={handleBack}>
            Back
          </Button>
          {isConnected && connectionDetails && (
            <Button
              variant="outline"
              onClick={handleDisconnect}
              disabled={isDisconnecting}
              className="text-red-600 border-red-300 hover:bg-red-50 dark:text-red-400 dark:border-red-700 dark:hover:bg-red-950"
            >
              {isDisconnecting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Disconnecting...
                </>
              ) : (
                "Disconnect"
              )}
            </Button>
          )}
        </div>
        <Button
          onClick={saveOrUpdateDataSource}
          disabled={!connectionStatus?.success || isSaving}
          className={`${
            connectionStatus?.success
              ? "bg-green-600 hover:bg-green-700"
              : "bg-gray-400"
          } text-white`}
        >
          {isSaving ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Saving...
            </>
          ) : selectedDatasource?.datasourceId ||
            connectionDetails?.datasourceId ? (
            "Update Datasource"
          ) : (
            "Save Datasource"
          )}
        </Button>
      </div>
    </div>
  );
};

export default DatasourceConfiguration;
