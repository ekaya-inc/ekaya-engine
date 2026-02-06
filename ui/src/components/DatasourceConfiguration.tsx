import {
  ArrowLeft,
  CheckCircle,
  XCircle,
  Loader2,
  Pencil,
  ExternalLink,
  AlertTriangle,
} from "lucide-react";
import { useState, useEffect, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";

import {
  getAdapterInfo,
  getProviderById,
  type ProviderInfo,
} from "../constants/adapters";
import { useDatasourceConnection } from "../contexts/DatasourceConnectionContext";
import { useToast } from "../hooks/useToast";
import engineApi from "../services/engineApi";
import type {
  ConnectionDetails,
  DatasourceConfig,
  DatasourceType,
  MSSQLAuthMethod,
  SSLMode,
} from "../types";
import { parsePostgresUrl } from "../utils/connectionString";

/**
 * ConnectionDetails extended with MSSQL-specific fields that may be spread
 * from the API config map at runtime.
 */
type ConnectionDetailsWithMSSQL = ConnectionDetails & {
  auth_method?: MSSQLAuthMethod;
  tenant_id?: string;
  client_id?: string;
  client_secret?: string;
  encrypt?: boolean;
  trust_server_certificate?: boolean;
  connection_timeout?: number | string;
};

/**
 * Config payload sent to test/save/update API endpoints.
 * Combines base DatasourceConfig fields with MSSQL-specific fields and type.
 */
type DatasourceApiConfig = DatasourceConfig & {
  type: DatasourceType;
  provider?: string;
  auth_method?: MSSQLAuthMethod;
  tenant_id?: string;
  client_id?: string;
  client_secret?: string;
  encrypt?: boolean;
  trust_server_certificate?: boolean;
  connection_timeout?: number;
};

import { Button } from "./ui/Button";
import { Card, CardContent } from "./ui/Card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/Dialog";
import { Input } from "./ui/Input";
import { Label } from "./ui/Label";
import { Switch } from "./ui/Switch";

interface DatasourceFormConfig {
  host: string;
  port: string;
  user: string;
  password: string;
  name: string;
  useSSL: boolean;
  displayName: string;

  // MSSQL-specific fields
  authMethod?: "sql" | "service_principal" | "user_delegation";
  tenantId?: string;
  clientId?: string;
  clientSecret?: string;
  encrypt?: boolean;
  trustServerCertificate?: boolean;
  connectionTimeout?: string;
}

interface DatasourceConfigurationProps {
  selectedAdapter: string | null;
  selectedProvider?: ProviderInfo | undefined;
  onBackToSelection: () => void;
}

const DatasourceConfiguration = ({
  selectedAdapter,
  selectedProvider,
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

  const adapterInfo = getAdapterInfo(
    selectedAdapter ?? connectionDetails?.type
  );

  const [testingConnection, setTestingConnection] = useState<boolean>(false);
  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [isDisconnecting, setIsDisconnecting] = useState<boolean>(false);
  const [showDisconnectDialog, setShowDisconnectDialog] =
    useState<boolean>(false);
  const [disconnectConfirmation, setDisconnectConfirmation] =
    useState<string>("");
  const [isEditingName, setIsEditingName] = useState<boolean>(false);
  const [connectionString, setConnectionString] = useState<string>("");
  const [connectionStringError, setConnectionStringError] =
    useState<string>("");
  // Track provider (from selection or parsed from connection string)
  const [activeProvider, setActiveProvider] = useState<
    ProviderInfo | undefined
  >(selectedProvider);
  const nameInputRef = useRef<HTMLInputElement>(null);
  const hasAutoSelectedAuthMethod = useRef<boolean>(false);

  // Use provider info for display if available, otherwise fall back to adapter info
  // activeProvider takes precedence (can be updated from connection string parsing)
  const displayInfo = activeProvider ?? selectedProvider ?? adapterInfo;
  // MSSQL-specific state
  const [hasAzureToken, setHasAzureToken] = useState<boolean>(false);
  const [userEmail, setUserEmail] = useState<string>("");

  const [config, setConfig] = useState<DatasourceFormConfig>({
    host: "",
    port:
      selectedAdapter === "mssql"
        ? "1433"
        : selectedProvider?.defaultPort?.toString() ?? "5432",
    user: "",
    password: "",
    name: "",
    useSSL:
      selectedProvider?.defaultSSL === "require" ||
      selectedProvider?.defaultSSL === "verify-full",
    displayName: "",

    // MSSQL defaults
    authMethod: "sql",
    encrypt: true,
    trustServerCertificate: false,
    connectionTimeout: "30",
  });

  // Determine if this is editing an existing datasource or configuring a new one
  const isEditingExisting = Boolean(
    selectedDatasource?.datasourceId ?? connectionDetails?.datasourceId
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

  const handleParseConnectionString = (): void => {
    setConnectionStringError("");

    if (!connectionString.trim()) {
      setConnectionStringError("Please enter a connection string");
      return;
    }

    const parsed = parsePostgresUrl(connectionString.trim());

    if (!parsed) {
      setConnectionStringError(
        "Invalid connection string format. Expected: postgresql://user:password@host:port/database"
      );
      return;
    }

    // Update form fields with parsed values
    setConfig((prev) => ({
      ...prev,
      host: parsed.host || prev.host,
      port: parsed.port.toString(),
      user: parsed.user || prev.user,
      password: parsed.password || prev.password,
      name: parsed.database || prev.name,
      useSSL:
        parsed.sslMode === "require" ||
        parsed.sslMode === "verify-full" ||
        parsed.sslMode === "prefer",
    }));

    // Update provider if detected from URL
    if (parsed.provider) {
      const provider = getProviderById(parsed.provider);
      if (provider) {
        setActiveProvider(provider);
        // Update displayName if it's still the default
        setConfig((prev) => {
          const currentDefault =
            activeProvider?.name ?? selectedProvider?.name ?? adapterInfo.name;
          if (prev.displayName === currentDefault || !prev.displayName) {
            return { ...prev, displayName: provider.name };
          }
          return prev;
        });
      }
    }

    // Clear the connection string input after successful parse
    setConnectionString("");
  };

  // Sync activeProvider when selectedProvider changes
  useEffect(() => {
    if (selectedProvider) {
      setActiveProvider(selectedProvider);
    }
  }, [selectedProvider]);

  // Update port when provider changes (only for new datasources)
  useEffect(() => {
    // Don't override port when editing an existing datasource - preserve the saved value
    if (isEditingExisting) {
      return;
    }
    const currentProvider = activeProvider ?? selectedProvider;
    if (currentProvider?.defaultPort && selectedAdapter !== "mssql") {
      setConfig((prev) => ({
        ...prev,
        port: currentProvider.defaultPort.toString(),
      }));
    }
  }, [activeProvider, selectedProvider, selectedAdapter, isEditingExisting]);

  // Load provider from existing config when editing
  useEffect(() => {
    if (connectionDetails?.provider) {
      const provider = getProviderById(connectionDetails.provider);
      if (provider) {
        setActiveProvider(provider);
      }
    }
  }, [connectionDetails?.provider]);

  useEffect(() => {
    if (connectionDetails) {
      // Editing existing datasource - load from connectionDetails
      const formData: DatasourceFormConfig = {
        host: connectionDetails.host || "",
        port: connectionDetails.port?.toString() ?? "5432",
        user: connectionDetails.user ?? "",
        password: connectionDetails.password ?? "",
        name: connectionDetails.name || "",
        useSSL:
          connectionDetails.ssl_mode === "require" ||
          connectionDetails.ssl_mode === "prefer",
        displayName: connectionDetails.displayName ?? displayInfo.name,
      };

      // Load MSSQL-specific fields from connectionDetails
      // These are stored in the config map and spread into connectionDetails
      if (selectedAdapter === "mssql") {
        const mssqlConfig = connectionDetails as ConnectionDetailsWithMSSQL;
        if (mssqlConfig.auth_method) {
          formData.authMethod = mssqlConfig.auth_method;
        }
        if (mssqlConfig.tenant_id) {
          formData.tenantId = mssqlConfig.tenant_id;
        }
        if (mssqlConfig.client_id) {
          formData.clientId = mssqlConfig.client_id;
        }
        if (mssqlConfig.client_secret) {
          formData.clientSecret = mssqlConfig.client_secret;
        }
        if (mssqlConfig.encrypt !== undefined) {
          formData.encrypt = mssqlConfig.encrypt;
        }
        if (mssqlConfig.trust_server_certificate !== undefined) {
          formData.trustServerCertificate = mssqlConfig.trust_server_certificate;
        }
        if (mssqlConfig.connection_timeout !== undefined) {
          formData.connectionTimeout = mssqlConfig.connection_timeout?.toString();
        }
      }

      setConfig(formData);
    } else {
      // New datasource - set default displayName from provider/adapter
      setConfig((prev) => ({
        ...prev,
        displayName: displayInfo.name,
      }));
    }
  }, [connectionDetails, displayInfo.name, selectedAdapter]);

  // Focus name input when entering edit mode
  useEffect(() => {
    if (isEditingName && nameInputRef.current) {
      nameInputRef.current.focus();
      nameInputRef.current.select();
    }
  }, [isEditingName]);

  // Check if user has Azure token for MSSQL user delegation (for new datasources)
  // Also fetch email when editing existing datasource with user_delegation
  useEffect(() => {
    if (isEditingExisting) {
      // When editing, check if existing datasource uses user_delegation
      const mssqlConfig = connectionDetails as ConnectionDetailsWithMSSQL;
      const existingAuthMethod = mssqlConfig?.auth_method;
      if (existingAuthMethod === "user_delegation") {
        setHasAzureToken(true);
        // Fetch email from current authenticated user's session
        fetch("/api/auth/me")
          .then((res) => res.json())
          .then((data) => {
            setUserEmail(data.email ?? "");
          })
          .catch((error) => {
            console.error("Failed to fetch user email:", error);
            // Continue without email - validation happens server-side
          });
      }
      return;
    }

    if (selectedAdapter === "mssql") {
      fetch("/api/auth/me")
        .then((res) => res.json())
        .then((data) => {
          const tokenAvailable = data.hasAzureToken ?? false;
          setHasAzureToken(tokenAvailable);
          setUserEmail(data.email ?? "");

          // Auto-select user delegation if token available
          // Only auto-select once on initial load (not when user changes selection)
          if (tokenAvailable && !hasAutoSelectedAuthMethod.current) {
            setConfig((prev) => ({ ...prev, authMethod: "user_delegation" }));
            hasAutoSelectedAuthMethod.current = true;
          }
        })
        .catch((error) => {
          console.error("Failed to check Azure token:", error);
          // Continue without token check - validation happens server-side
        });
    } else {
      // Reset the ref when switching away from MSSQL
      hasAutoSelectedAuthMethod.current = false;
    }
  }, [selectedAdapter, connectionDetails, isEditingExisting]);

  const isConnectionReadyToTest = (): boolean => {
    // Basic fields required for all adapters
    if (!config.host || !config.name) {
      return false;
    }

    // MSSQL-specific validation based on auth method
    if (selectedAdapter === "mssql") {
      if (config.authMethod === "sql") {
        // SQL Authentication: requires username AND password
        return !!(config.user && config.password);
      } else if (config.authMethod === "service_principal") {
        // Service Principal: requires tenant ID, client ID, and client secret
        // NO username/password needed
        return !!(config.tenantId && config.clientId && config.clientSecret);
      } else if (config.authMethod === "user_delegation") {
        // User Delegation: only requires Azure token from SSO
        // NO username/password/client credentials needed
        // If editing existing datasource with user_delegation, allow testing
        // (token might refresh on backend, or was working before)
        return hasAzureToken || (isEditingExisting && connectionDetails !== null);
      }
      // Unknown auth method
      return false;
    } else {
      // PostgreSQL and other adapters: require username
      // Password is typically required but may be empty for some auth methods
      // For now, we require username (password can be empty for trust auth, etc.)
      return !!config.user;
    }
  };

  const handleTestConnection = async (): Promise<void> => {
    clearError();
    setTestingConnection(true);

    try {
      const testDetails: DatasourceApiConfig = {
        type: selectedAdapter as DatasourceType,
        host: config.host,
        port: parseInt(config.port),
        name: config.name,
        ssl_mode: (config.useSSL ? "require" : "disable") as SSLMode,
      };

      // MSSQL-specific configuration
      if (selectedAdapter === "mssql") {
        testDetails.auth_method = config.authMethod ?? "sql";

        if (config.authMethod === "sql") {
          testDetails.user = config.user;
          testDetails.password = config.password;
        } else if (config.authMethod === "service_principal") {
          if (config.tenantId) testDetails.tenant_id = config.tenantId;
          if (config.clientId) testDetails.client_id = config.clientId;
          if (config.clientSecret)
            testDetails.client_secret = config.clientSecret;
        }
        // user_delegation: no credentials needed (token from JWT)
        // Explicitly do NOT send user/password fields to avoid auto-detection issues

        // MSSQL connection options
        testDetails.encrypt = config.encrypt ?? true;
        testDetails.trust_server_certificate =
          config.trustServerCertificate ?? false;
        testDetails.connection_timeout = parseInt(
          config.connectionTimeout ?? "30"
        );
      } else {
        // PostgreSQL and other adapters
        testDetails.user = config.user;
        testDetails.password = config.password;
      }

      if (!pid) {
        throw new Error("Project ID not available from route");
      }
      await testConnection(pid, testDetails);
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
        setShowDisconnectDialog(false);
        setDisconnectConfirmation("");
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
      const datasourceType = selectedAdapter as DatasourceType;
      // Include provider for PostgreSQL-compatible variants (e.g., supabase, neon)
      const currentProvider = activeProvider ?? selectedProvider;

      const apiConfig: DatasourceApiConfig = {
        type: datasourceType,
        ...(currentProvider && { provider: currentProvider.id }),
        host: config.host,
        port: parseInt(config.port),
        name: config.name,
        ssl_mode: (config.useSSL ? "require" : "disable") as SSLMode,
      };

      // MSSQL-specific configuration
      if (selectedAdapter === "mssql") {
        apiConfig.auth_method = config.authMethod ?? "sql";

        if (config.authMethod === "sql") {
          apiConfig.user = config.user;
          apiConfig.password = config.password;
        } else if (config.authMethod === "service_principal") {
          if (config.tenantId) apiConfig.tenant_id = config.tenantId;
          if (config.clientId) apiConfig.client_id = config.clientId;
          if (config.clientSecret)
            apiConfig.client_secret = config.clientSecret;
        }
        // user_delegation: no credentials needed (token from JWT)

        // MSSQL connection options
        apiConfig.encrypt = config.encrypt ?? true;
        apiConfig.trust_server_certificate =
          config.trustServerCertificate ?? false;
        apiConfig.connection_timeout = parseInt(
          config.connectionTimeout ?? "30"
        );
      } else {
        // PostgreSQL and other adapters
        apiConfig.user = config.user;
        apiConfig.password = config.password;
      }

      engineApi.validateConnectionDetails(apiConfig);

      if (!pid) {
        throw new Error("Project ID not available from route");
      }

      const isEditing = datasourceId !== undefined && datasourceId !== null;

      const result = isEditing
        ? await updateDataSource(
            pid,
            datasourceId,
            config.displayName,
            datasourceType,
            apiConfig
          )
        : await saveDataSource(
            pid,
            config.displayName,
            datasourceType,
            apiConfig
          );

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
          disabled={testingConnection || !isConnectionReadyToTest()}
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
        {(testingConnection || !isConnectionReadyToTest()) && (
          <p className="text-sm text-text-secondary mt-2">
            {testingConnection
              ? "Please wait while we test your database connection..."
              : "Please fill in all required fields to test connection"}
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

  // Get provider-specific help info
  const currentProviderInfo = activeProvider ?? selectedProvider;
  const hasConnectionStringHelp = currentProviderInfo?.connectionStringHelp;
  const hasHelpUrl = currentProviderInfo?.helpUrl;

  const renderConnectionStringSection = () => {
    // Only show connection string parser for postgres adapters when not editing
    if (selectedAdapter !== "postgres" || isEditingExisting) {
      return null;
    }

    return (
      <div className="mb-6 pb-6 border-b">
        <Label htmlFor="connectionString" className="mb-2 block">
          Quick Setup: Paste Connection String
        </Label>
        <div className="flex gap-2">
          <Input
            id="connectionString"
            placeholder="postgresql://user:password@host:port/database"
            value={connectionString}
            onChange={(e) => {
              setConnectionString(e.target.value);
              setConnectionStringError("");
            }}
            className="flex-1 font-mono text-sm"
          />
          <Button
            type="button"
            onClick={handleParseConnectionString}
            variant="outline"
            className="shrink-0"
          >
            Parse
          </Button>
        </div>
        {connectionStringError && (
          <p className="text-sm text-red-600 dark:text-red-400 mt-2">
            {connectionStringError}
          </p>
        )}
        {(hasConnectionStringHelp !== undefined ||
          hasHelpUrl !== undefined) && (
          <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-text-secondary">
            {hasConnectionStringHelp && (
              <span>{currentProviderInfo.connectionStringHelp}</span>
            )}
            {hasHelpUrl && (
              <a
                href={currentProviderInfo.helpUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
              >
                View documentation
                <ExternalLink className="w-3 h-3" />
              </a>
            )}
          </div>
        )}
      </div>
    );
  };

  const renderDatasourceSetup = () => (
    <div className="space-y-6">
      {renderConnectionStringSection()}
      {/* Common Fields: Host, Port, Database Name */}
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="host">
            Host <span className="text-red-500">*</span>
          </Label>
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
            placeholder={selectedAdapter === "mssql" ? "1433" : "5432"}
            value={config.port}
            onChange={(e) => handleConfigChange("port", e.target.value)}
          />
          <p className="text-sm text-text-secondary">
            Your database server port.
          </p>
        </div>
      </div>

      <div className="space-y-2">
        <Label htmlFor="name">
          Database Name <span className="text-red-500">*</span>
        </Label>
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

      {/* MSSQL Auth Method Selector */}
      {selectedAdapter === "mssql" && (
        <div className="space-y-2">
          <Label htmlFor="auth-method">Authentication Method</Label>
          <select
            id="auth-method"
            value={config.authMethod}
            onChange={(e) => handleConfigChange("authMethod", e.target.value)}
            className="w-full rounded-md border border-border-medium bg-surface-secondary text-text-primary px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-purple focus:border-transparent"
            disabled={isEditingExisting}
          >
            {hasAzureToken && (
              <option value="user_delegation">
                Use My Microsoft Login ({userEmail})
              </option>
            )}
            <option value="service_principal">
              Azure AD Service Principal
            </option>
            <option value="sql">SQL Server Authentication</option>
          </select>
          <p className="text-sm text-text-secondary">
            Choose how to authenticate with SQL Server.
          </p>
        </div>
      )}

      {/* Conditional Auth Fields based on adapter and auth method */}
      {selectedAdapter !== "mssql" ? (
        // PostgreSQL, MySQL, etc. - always show username/password
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="user">
              Username <span className="text-red-500">*</span>
            </Label>
            <Input
              id="user"
              placeholder="Database user"
              value={config.user}
              onChange={(e) => handleConfigChange("user", e.target.value)}
            />
            <p className="text-sm text-text-secondary">
              The database user for the account that you want to use to connect
              to your database.
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
      ) : config.authMethod === "sql" ? (
        // MSSQL SQL Authentication
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="user">
              Username <span className="text-red-500">*</span>
            </Label>
            <Input
              id="user"
              placeholder="SQL Server user"
              value={config.user}
              onChange={(e) => handleConfigChange("user", e.target.value)}
            />
            <p className="text-sm text-text-secondary">
              SQL Server authentication username.
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="password">
              Password <span className="text-red-500">*</span>
            </Label>
            <Input
              id="password"
              type="password"
              placeholder="SQL Server password"
              value={config.password}
              onChange={(e) => handleConfigChange("password", e.target.value)}
            />
            <p className="text-sm text-text-secondary">
              SQL Server authentication password.
            </p>
          </div>
        </div>
      ) : config.authMethod === "service_principal" ? (
        // MSSQL Service Principal
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="tenantId">
              Azure Tenant ID <span className="text-red-500">*</span>
            </Label>
            <Input
              id="tenantId"
              placeholder="00000000-0000-0000-0000-000000000000"
              value={config.tenantId ?? ""}
              onChange={(e) => handleConfigChange("tenantId", e.target.value)}
            />
            <p className="text-sm text-text-secondary">
              Your Azure AD tenant ID (directory ID).
            </p>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="clientId">
                Azure Client ID <span className="text-red-500">*</span>
              </Label>
              <Input
                id="clientId"
                placeholder="00000000-0000-0000-0000-000000000000"
                value={config.clientId ?? ""}
                onChange={(e) => handleConfigChange("clientId", e.target.value)}
              />
              <p className="text-sm text-text-secondary">
                Application (client) ID from Azure AD.
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="clientSecret">
                Azure Client Secret <span className="text-red-500">*</span>
              </Label>
              <Input
                id="clientSecret"
                type="password"
                placeholder="Client secret value"
                value={config.clientSecret ?? ""}
                onChange={(e) =>
                  handleConfigChange("clientSecret", e.target.value)
                }
              />
              <p className="text-sm text-text-secondary">
                Client secret value from Azure AD.
              </p>
            </div>
          </div>
          <div className="p-3 bg-blue-50 dark:bg-blue-950 border border-blue-200 dark:border-blue-800 rounded-md">
            <p className="text-sm text-blue-700 dark:text-blue-400">
              Need help setting up Service Principal?{" "}
              <a
                href="https://learn.microsoft.com/en-us/azure/azure-sql/database/authentication-aad-service-principal?view=azuresql"
                target="_blank"
                rel="noopener noreferrer"
                className="underline hover:text-blue-800 dark:hover:text-blue-300"
              >
                View setup guide →
              </a>
            </p>
          </div>
        </div>
      ) : config.authMethod === "user_delegation" ? (
        // MSSQL User Delegation
        <div className="p-4 bg-surface-tertiary border border-border-medium rounded-md">
          <p className="text-sm text-text-primary mb-2">
            You&apos;ll connect to SQL Server as <strong>{userEmail}</strong>
          </p>
          <p className="text-sm text-text-secondary mb-3">
            Make sure your Microsoft account has been granted access in Azure
            SQL Database.
          </p>
          <a
            href="https://learn.microsoft.com/en-us/entra/identity/hybrid/connect/how-to-connect-install-sql-delegation"
            target="_blank"
            rel="noopener noreferrer"
            className="text-sm text-brand-purple hover:underline"
          >
            Setup guide →
          </a>
        </div>
      ) : null}

      {/* MSSQL Connection Options */}
      {selectedAdapter === "mssql" && (
        <details className="border border-border-medium rounded-md">
          <summary className="cursor-pointer p-3 text-sm font-medium text-text-primary hover:bg-surface-secondary">
            Advanced Options
          </summary>
          <div className="p-4 pt-2 space-y-4">
            <div className="flex items-center space-x-2">
              <Switch
                id="encrypt"
                checked={config.encrypt ?? true}
                onCheckedChange={(checked) =>
                  handleConfigChange("encrypt", checked)
                }
              />
              <Label htmlFor="encrypt">Encrypt Connection</Label>
            </div>
            <div className="flex items-center space-x-2">
              <Switch
                id="trustServerCertificate"
                checked={config.trustServerCertificate ?? false}
                onCheckedChange={(checked) =>
                  handleConfigChange("trustServerCertificate", checked)
                }
              />
              <Label htmlFor="trustServerCertificate">
                Trust Server Certificate
              </Label>
            </div>
            <div className="space-y-2">
              <Label htmlFor="connectionTimeout">
                Connection Timeout (seconds)
              </Label>
              <Input
                id="connectionTimeout"
                type="number"
                placeholder="30"
                value={config.connectionTimeout ?? "30"}
                onChange={(e) =>
                  handleConfigChange("connectionTimeout", e.target.value)
                }
                className="max-w-xs"
              />
            </div>
          </div>
        </details>
      )}

      {/* Non-MSSQL SSL Toggle */}
      {selectedAdapter !== "mssql" && (
        <div className="flex items-center space-x-2">
          <Switch
            id="useSSL"
            checked={config.useSSL}
            onCheckedChange={(checked) => handleConfigChange("useSSL", checked)}
          />
          <Label htmlFor="useSSL">Use SSL</Label>
        </div>
      )}

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
          {displayInfo?.icon ? (
            <img
              src={displayInfo.icon}
              alt={displayInfo.name}
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
              Configure {displayInfo?.name}
            </h1>
          </div>
        </div>
      </div>

      <Card>
        <CardContent className="p-6">
          {/* Editable Datasource Name */}
          <div className="mb-6 pb-6 border-b">
            <Label
              htmlFor="displayName"
              className="text-sm text-text-secondary mb-2 block"
            >
              Datasource Name
            </Label>
            {isEditingName ? (
              <Input
                ref={nameInputRef}
                id="displayName"
                value={config.displayName}
                onChange={(e) =>
                  handleConfigChange("displayName", e.target.value)
                }
                onBlur={() => setIsEditingName(false)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === "Escape") {
                    setIsEditingName(false);
                  }
                }}
                className="text-xl font-semibold max-w-md"
                placeholder="Enter datasource name"
              />
            ) : (
              <button
                type="button"
                onClick={() => setIsEditingName(true)}
                className="flex items-center gap-2 text-xl font-semibold text-text-primary hover:text-blue-600 transition-colors group"
              >
                {config.displayName || displayInfo.name}
                <Pencil className="w-4 h-4 opacity-0 group-hover:opacity-100 transition-opacity" />
              </button>
            )}
          </div>

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
              onClick={() => setShowDisconnectDialog(true)}
              className="text-red-600 border-red-300 hover:bg-red-50 dark:text-red-400 dark:border-red-700 dark:hover:bg-red-950"
            >
              Disconnect
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

      {/* Disconnect Confirmation Dialog */}
      <Dialog
        open={showDisconnectDialog}
        onOpenChange={(open) => {
          if (!isDisconnecting) {
            setShowDisconnectDialog(open);
            if (!open) {
              setDisconnectConfirmation("");
            }
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5 text-red-500" />
              Disconnect Datasource?
            </DialogTitle>
            <DialogDescription>
              This will disconnect from the datasource and clear all schema,
              approved queries, and associated ontology extractions that were
              attached to this datasource.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <label className="text-sm font-medium text-text-primary">
              Type{" "}
              <span className="font-mono bg-gray-100 dark:bg-gray-800 px-1 rounded">
                disconnect datasource
              </span>{" "}
              to confirm
            </label>
            <Input
              value={disconnectConfirmation}
              onChange={(e) => setDisconnectConfirmation(e.target.value)}
              placeholder="disconnect datasource"
              className="mt-2"
              disabled={isDisconnecting}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowDisconnectDialog(false)}
              disabled={isDisconnecting}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDisconnect}
              disabled={
                disconnectConfirmation !== "disconnect datasource" || isDisconnecting
              }
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
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default DatasourceConfiguration;
