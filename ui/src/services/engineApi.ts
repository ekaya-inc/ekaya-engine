/**
 * Engine API Service
 * Handles communication with the Ekaya Engine REST API
 */

import { fetchWithAuth } from '../lib/api';
import type {
  ApiResponse,
  ApproveQueryResponse,
  ConnectionDetails,
  CreateDatasourceResponse,
  CreateGlossaryTermRequest,
  CreateProjectKnowledgeRequest,
  CreateQueryRequest,
  CreateRelationshipRequest,
  DAGStatusResponse,
  DatasourceConfig,
  DatasourceSchema,
  DatasourceType,
  DeleteDatasourceResponse,
  DeleteQueryResponse,
  DiscoveryResults,
  EntitiesListResponse,
  ExecuteQueryRequest,
  ExecuteQueryResponse,
  GetDatasourceResponse,
  GlossaryListResponse,
  GlossaryTerm,
  InstalledApp,
  InstalledAppsResponse,
  ListDatasourcesResponse,
  ListPendingQueriesResponse,
  ListQueriesResponse,
  MCPConfigResponse,
  ParseProjectKnowledgeResponse,
  ProjectKnowledge,
  ProjectKnowledgeListResponse,
  Query,
  RejectQueryResponse,
  RelationshipDetail,
  RelationshipsResponse,
  SaveSelectionsResponse,
  SchemaRefreshResponse,
  TestConnectionRequest,
  TestConnectionResponse,
  TestQueryRequest,
  TestSQLResult,
  UpdateGlossaryTermRequest,
  UpdateMCPConfigRequest,
  UpdateProjectKnowledgeRequest,
  UpdateQueryRequest,
  ValidateQueryRequest,
  ValidateQueryResponse,
} from '../types';

const ENGINE_BASE_URL = '/api/projects';

class EngineApiService {
  private baseURL: string;

  constructor() {
    this.baseURL = ENGINE_BASE_URL;
  }

  /**
   * Make HTTP request with error handling
   * Uses fetchWithAuth for automatic OAuth handling and cookie-based authentication
   */
  private async makeRequest<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<ApiResponse<T>> {
    const url = `${this.baseURL}${endpoint}`;
    const config: RequestInit = {
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      ...options,
    };

    // Authentication is handled via httpOnly cookies (automatic with fetchWithAuth)
    // No need to manually set Authorization header

    try {
      const response = await fetchWithAuth(url, config);
      const data = (await response.json()) as ApiResponse<T>;

      if (!response.ok) {
        throw new Error(
          data.error ??
            data.message ??
            `HTTP ${response.status}: ${response.statusText}`
        );
      }

      return data;
    } catch (error) {
      console.error(`Engine API Error (${endpoint}):`, error);
      throw error;
    }
  }

  /**
   * Test datasource connection
   */
  async testDatasourceConnection(
    projectId: string,
    connectionDetails: TestConnectionRequest
  ): Promise<ApiResponse<TestConnectionResponse>> {
    return this.makeRequest<TestConnectionResponse>(`/${projectId}/datasources/test`, {
      method: 'POST',
      body: JSON.stringify(connectionDetails),
    });
  }

  /**
   * Create datasource for a project
   */
  async createDataSource({
    projectId,
    name,
    datasourceType,
    config,
  }: {
    projectId: string;
    name: string;
    datasourceType: DatasourceType;
    config: DatasourceConfig;
  }): Promise<ApiResponse<CreateDatasourceResponse>> {
    return this.makeRequest<CreateDatasourceResponse>(
      `/${projectId}/datasources`,
      {
        method: 'POST',
        body: JSON.stringify({
          project_id: projectId,
          name: name,
          type: datasourceType,
          config: config,
        }),
      }
    );
  }

  /**
   * Get datasource by datasource_id
   */
  async getDataSource(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<GetDatasourceResponse>> {
    return this.makeRequest<GetDatasourceResponse>(
      `/${projectId}/datasources/${datasourceId}`
    );
  }

  /**
   * Update datasource for a project
   */
  async updateDataSource(
    projectId: string,
    datasourceId: string,
    name: string,
    datasourceType: DatasourceType,
    config: DatasourceConfig
  ): Promise<ApiResponse<CreateDatasourceResponse>> {
    return this.makeRequest<CreateDatasourceResponse>(
      `/${projectId}/datasources/${datasourceId}`,
      {
        method: 'PUT',
        body: JSON.stringify({
          name: name,
          type: datasourceType,
          config: config,
        }),
      }
    );
  }

  /**
   * Delete datasource for a project
   */
  async deleteDataSource(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<DeleteDatasourceResponse>> {
    return this.makeRequest<DeleteDatasourceResponse>(
      `/${projectId}/datasources/${datasourceId}`,
      {
        method: 'DELETE',
      }
    );
  }

  /**
   * List all datasources for a project
   */
  async listDataSources(
    projectId: string
  ): Promise<ApiResponse<ListDatasourcesResponse>> {
    return this.makeRequest<ListDatasourcesResponse>(
      `/${projectId}/datasources`
    );
  }

  /**
   * Get datasource schema
   * Returns comprehensive schema information including tables, columns, and relationships
   * GET /api/projects/{projectId}/datasources/{datasourceId}/schema
   */
  async getSchema(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<DatasourceSchema>> {
    return this.makeRequest<DatasourceSchema>(
      `/${projectId}/datasources/${datasourceId}/schema`
    );
  }

  /**
   * Get entity relationships for a project
   * Returns comprehensive relationship information including type, cardinality, and approval status
   * GET /api/projects/{projectId}/relationships
   */
  async getRelationships(
    projectId: string,
    _datasourceId: string // kept for API compatibility, not used
  ): Promise<ApiResponse<RelationshipsResponse>> {
    return this.makeRequest<RelationshipsResponse>(
      `/${projectId}/relationships`
    );
  }

  /**
   * Create a manual relationship between two columns
   * The relationship will be analyzed to determine cardinality
   * POST /api/projects/{projectId}/datasources/{datasourceId}/schema/relationships
   */
  async createRelationship(
    projectId: string,
    datasourceId: string,
    request: CreateRelationshipRequest
  ): Promise<ApiResponse<RelationshipDetail>> {
    return this.makeRequest<RelationshipDetail>(
      `/${projectId}/datasources/${datasourceId}/schema/relationships`,
      {
        method: 'POST',
        body: JSON.stringify(request),
      }
    );
  }

  /**
   * Remove a relationship (sets is_approved = false)
   * The relationship remains in the database to prevent re-discovery
   * DELETE /api/projects/{projectId}/datasources/{datasourceId}/schema/relationships/{relationshipId}
   */
  async removeRelationship(
    projectId: string,
    datasourceId: string,
    relationshipId: string
  ): Promise<ApiResponse<{ message: string }>> {
    return this.makeRequest<{ message: string }>(
      `/${projectId}/datasources/${datasourceId}/schema/relationships/${relationshipId}`,
      {
        method: 'DELETE',
      }
    );
  }

  /**
   * Save schema selections (tables and columns) for a datasource
   * POST /api/projects/{projectId}/datasources/{datasourceId}/schema/selections
   * @param tableSelections - Map of table ID (UUID) to selection status
   * @param columnSelections - Map of table ID (UUID) to array of selected column IDs (UUIDs)
   */
  async saveSchemaSelections(
    projectId: string,
    datasourceId: string,
    tableSelections: Record<string, boolean>,
    columnSelections: Record<string, string[]>
  ): Promise<ApiResponse<SaveSelectionsResponse>> {
    return this.makeRequest<SaveSelectionsResponse>(
      `/${projectId}/datasources/${datasourceId}/schema/selections`,
      {
        method: 'POST',
        body: JSON.stringify({
          table_selections: tableSelections,
          column_selections: columnSelections,
        }),
      }
    );
  }

  /**
   * Refresh schema from datasource
   * Re-discovers tables and columns from the datasource and updates the schema cache
   * POST /api/projects/{projectId}/datasources/{datasourceId}/schema/refresh
   */
  async refreshSchema(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<SchemaRefreshResponse>> {
    return this.makeRequest<SchemaRefreshResponse>(
      `/${projectId}/datasources/${datasourceId}/schema/refresh`,
      {
        method: 'POST',
      }
    );
  }

  // --- Relationship Discovery Methods ---

  /**
   * Discover relationships for a datasource (synchronous)
   * POST /api/projects/{projectId}/datasources/{datasourceId}/schema/relationships/discover
   */
  async discoverRelationships(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<DiscoveryResults>> {
    return this.makeRequest<DiscoveryResults>(
      `/${projectId}/datasources/${datasourceId}/schema/relationships/discover`,
      {
        method: 'POST',
      }
    );
  }

  // --- Pre-Approved Queries Methods ---

  /**
   * List all queries for a datasource
   * GET /api/projects/{projectId}/datasources/{datasourceId}/queries
   */
  async listQueries(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<ListQueriesResponse>> {
    return this.makeRequest<ListQueriesResponse>(
      `/${projectId}/datasources/${datasourceId}/queries`
    );
  }

  /**
   * Get a single query by ID
   * GET /api/projects/{projectId}/datasources/{datasourceId}/queries/{queryId}
   */
  async getQuery(
    projectId: string,
    datasourceId: string,
    queryId: string
  ): Promise<ApiResponse<Query>> {
    return this.makeRequest<Query>(
      `/${projectId}/datasources/${datasourceId}/queries/${queryId}`
    );
  }

  /**
   * Create a new query
   * Note: Dialect is derived from datasource type by the backend
   * POST /api/projects/{projectId}/datasources/{datasourceId}/queries
   */
  async createQuery(
    projectId: string,
    datasourceId: string,
    request: CreateQueryRequest
  ): Promise<ApiResponse<Query>> {
    return this.makeRequest<Query>(
      `/${projectId}/datasources/${datasourceId}/queries`,
      {
        method: 'POST',
        body: JSON.stringify(request),
      }
    );
  }

  /**
   * Update an existing query
   * Note: Dialect cannot be updated - it's derived from datasource type
   * PUT /api/projects/{projectId}/datasources/{datasourceId}/queries/{queryId}
   */
  async updateQuery(
    projectId: string,
    datasourceId: string,
    queryId: string,
    request: UpdateQueryRequest
  ): Promise<ApiResponse<Query>> {
    return this.makeRequest<Query>(
      `/${projectId}/datasources/${datasourceId}/queries/${queryId}`,
      {
        method: 'PUT',
        body: JSON.stringify(request),
      }
    );
  }

  /**
   * Delete a query (soft delete)
   * DELETE /api/projects/{projectId}/datasources/{datasourceId}/queries/{queryId}
   */
  async deleteQuery(
    projectId: string,
    datasourceId: string,
    queryId: string
  ): Promise<ApiResponse<DeleteQueryResponse>> {
    return this.makeRequest<DeleteQueryResponse>(
      `/${projectId}/datasources/${datasourceId}/queries/${queryId}`,
      {
        method: 'DELETE',
      }
    );
  }

  /**
   * Execute a saved query
   * POST /api/projects/{projectId}/datasources/{datasourceId}/queries/{queryId}/execute
   */
  async executeQuery(
    projectId: string,
    datasourceId: string,
    queryId: string,
    request?: ExecuteQueryRequest
  ): Promise<ApiResponse<ExecuteQueryResponse>> {
    const options: RequestInit = {
      method: 'POST',
    };
    if (request) {
      options.body = JSON.stringify(request);
    }
    return this.makeRequest<ExecuteQueryResponse>(
      `/${projectId}/datasources/${datasourceId}/queries/${queryId}/execute`,
      options
    );
  }

  /**
   * Test a SQL query without saving it
   * POST /api/projects/{projectId}/datasources/{datasourceId}/queries/test
   */
  async testQuery(
    projectId: string,
    datasourceId: string,
    request: TestQueryRequest
  ): Promise<ApiResponse<ExecuteQueryResponse>> {
    return this.makeRequest<ExecuteQueryResponse>(
      `/${projectId}/datasources/${datasourceId}/queries/test`,
      {
        method: 'POST',
        body: JSON.stringify(request),
      }
    );
  }

  /**
   * Validate SQL syntax without executing
   * POST /api/projects/{projectId}/datasources/{datasourceId}/queries/validate
   */
  async validateQuery(
    projectId: string,
    datasourceId: string,
    request: ValidateQueryRequest
  ): Promise<ApiResponse<ValidateQueryResponse>> {
    return this.makeRequest<ValidateQueryResponse>(
      `/${projectId}/datasources/${datasourceId}/queries/validate`,
      {
        method: 'POST',
        body: JSON.stringify(request),
      }
    );
  }

  /**
   * List pending query suggestions for admin review
   * GET /api/projects/{projectId}/queries/pending
   */
  async listPendingQueries(
    projectId: string
  ): Promise<ApiResponse<ListPendingQueriesResponse>> {
    return this.makeRequest<ListPendingQueriesResponse>(
      `/${projectId}/queries/pending`
    );
  }

  /**
   * Approve a pending query suggestion
   * POST /api/projects/{projectId}/queries/{queryId}/approve
   */
  async approveQuery(
    projectId: string,
    queryId: string
  ): Promise<ApiResponse<ApproveQueryResponse>> {
    return this.makeRequest<ApproveQueryResponse>(
      `/${projectId}/queries/${queryId}/approve`,
      {
        method: 'POST',
      }
    );
  }

  /**
   * Reject a pending query suggestion
   * POST /api/projects/{projectId}/queries/{queryId}/reject
   */
  async rejectQuery(
    projectId: string,
    queryId: string,
    reason: string
  ): Promise<ApiResponse<RejectQueryResponse>> {
    return this.makeRequest<RejectQueryResponse>(
      `/${projectId}/queries/${queryId}/reject`,
      {
        method: 'POST',
        body: JSON.stringify({ reason }),
      }
    );
  }

  /**
   * Move a rejected query back to pending status
   * POST /api/projects/{projectId}/queries/{queryId}/move-to-pending
   */
  async moveToPending(
    projectId: string,
    queryId: string
  ): Promise<ApiResponse<{ success: boolean; message: string }>> {
    return this.makeRequest<{ success: boolean; message: string }>(
      `/${projectId}/queries/${queryId}/move-to-pending`,
      {
        method: 'POST',
      }
    );
  }

  /**
   * Check Engine API health
   */
  async healthCheck(): Promise<ApiResponse<{ status: string }> | null> {
    try {
      return this.makeRequest<{ status: string }>('/health');
    } catch (error) {
      console.error(`Engine API Error (health check):`, error);
      return null;
    }
  }

  // --- Entity Management Methods ---

  /**
   * List all entities for a project
   * GET /api/projects/{projectId}/entities
   */
  async listEntities(
    projectId: string
  ): Promise<ApiResponse<EntitiesListResponse>> {
    return this.makeRequest<EntitiesListResponse>(`/${projectId}/entities`);
  }

  // --- Glossary Methods ---

  /**
   * List all glossary terms for a project
   * GET /api/projects/{projectId}/glossary
   */
  async listGlossaryTerms(
    projectId: string
  ): Promise<ApiResponse<GlossaryListResponse>> {
    return this.makeRequest<GlossaryListResponse>(`/${projectId}/glossary`);
  }

  /**
   * Test SQL for a glossary term
   * POST /api/projects/{projectId}/glossary/test-sql
   */
  async testGlossarySQL(
    projectId: string,
    sql: string
  ): Promise<ApiResponse<TestSQLResult>> {
    return this.makeRequest<TestSQLResult>(`/${projectId}/glossary/test-sql`, {
      method: 'POST',
      body: JSON.stringify({ sql }),
    });
  }

  /**
   * Create a new glossary term
   * POST /api/projects/{projectId}/glossary
   */
  async createGlossaryTerm(
    projectId: string,
    request: CreateGlossaryTermRequest
  ): Promise<ApiResponse<GlossaryTerm>> {
    return this.makeRequest<GlossaryTerm>(`/${projectId}/glossary`, {
      method: 'POST',
      body: JSON.stringify(request),
    });
  }

  /**
   * Update an existing glossary term
   * PUT /api/projects/{projectId}/glossary/{termId}
   */
  async updateGlossaryTerm(
    projectId: string,
    termId: string,
    request: UpdateGlossaryTermRequest
  ): Promise<ApiResponse<GlossaryTerm>> {
    return this.makeRequest<GlossaryTerm>(`/${projectId}/glossary/${termId}`, {
      method: 'PUT',
      body: JSON.stringify(request),
    });
  }

  /**
   * Delete a glossary term
   * DELETE /api/projects/{projectId}/glossary/{termId}
   */
  async deleteGlossaryTerm(
    projectId: string,
    termId: string
  ): Promise<ApiResponse<void>> {
    return this.makeRequest<void>(`/${projectId}/glossary/${termId}`, {
      method: 'DELETE',
    });
  }

  // --- Project Knowledge Methods ---

  /**
   * List all project knowledge facts for a project
   * GET /api/projects/{projectId}/project-knowledge
   */
  async listProjectKnowledge(
    projectId: string
  ): Promise<ApiResponse<ProjectKnowledgeListResponse>> {
    return this.makeRequest<ProjectKnowledgeListResponse>(
      `/${projectId}/project-knowledge`
    );
  }

  /**
   * Create a new project knowledge fact
   * POST /api/projects/{projectId}/project-knowledge
   */
  async createProjectKnowledge(
    projectId: string,
    data: CreateProjectKnowledgeRequest
  ): Promise<ApiResponse<ProjectKnowledge>> {
    return this.makeRequest<ProjectKnowledge>(
      `/${projectId}/project-knowledge`,
      {
        method: 'POST',
        body: JSON.stringify(data),
      }
    );
  }

  /**
   * Parse a free-form fact using LLM and create structured knowledge facts
   * POST /api/projects/{projectId}/project-knowledge/parse
   */
  async parseProjectKnowledge(
    projectId: string,
    text: string
  ): Promise<ApiResponse<ParseProjectKnowledgeResponse>> {
    return this.makeRequest<ParseProjectKnowledgeResponse>(
      `/${projectId}/project-knowledge/parse`,
      {
        method: 'POST',
        body: JSON.stringify({ text }),
      }
    );
  }

  /**
   * Update an existing project knowledge fact
   * PUT /api/projects/{projectId}/project-knowledge/{id}
   */
  async updateProjectKnowledge(
    projectId: string,
    id: string,
    data: UpdateProjectKnowledgeRequest
  ): Promise<ApiResponse<ProjectKnowledge>> {
    return this.makeRequest<ProjectKnowledge>(
      `/${projectId}/project-knowledge/${id}`,
      {
        method: 'PUT',
        body: JSON.stringify(data),
      }
    );
  }

  /**
   * Delete a project knowledge fact
   * DELETE /api/projects/{projectId}/project-knowledge/{id}
   */
  async deleteProjectKnowledge(
    projectId: string,
    id: string
  ): Promise<ApiResponse<void>> {
    return this.makeRequest<void>(`/${projectId}/project-knowledge/${id}`, {
      method: 'DELETE',
    });
  }

  // --- MCP Configuration Methods ---

  /**
   * Get MCP configuration for a project
   * GET /api/projects/{projectId}/mcp/config
   */
  async getMCPConfig(projectId: string): Promise<ApiResponse<MCPConfigResponse>> {
    return this.makeRequest<MCPConfigResponse>(`/${projectId}/mcp/config`);
  }

  /**
   * Update MCP configuration for a project
   * PATCH /api/projects/{projectId}/mcp/config
   */
  async updateMCPConfig(
    projectId: string,
    request: UpdateMCPConfigRequest
  ): Promise<ApiResponse<MCPConfigResponse>> {
    return this.makeRequest<MCPConfigResponse>(`/${projectId}/mcp/config`, {
      method: 'PATCH',
      body: JSON.stringify(request),
    });
  }

  /**
   * Get agent API key for a project
   * GET /api/projects/{projectId}/mcp/agent-key
   * @param reveal - If true, returns the full key; otherwise returns masked key
   */
  async getAgentAPIKey(
    projectId: string,
    reveal: boolean = false
  ): Promise<ApiResponse<{ key: string; masked: boolean }>> {
    const query = reveal ? '?reveal=true' : '';
    return this.makeRequest<{ key: string; masked: boolean }>(
      `/${projectId}/mcp/agent-key${query}`
    );
  }

  /**
   * Regenerate agent API key for a project
   * POST /api/projects/{projectId}/mcp/agent-key/regenerate
   */
  async regenerateAgentAPIKey(
    projectId: string
  ): Promise<ApiResponse<{ key: string }>> {
    return this.makeRequest<{ key: string }>(
      `/${projectId}/mcp/agent-key/regenerate`,
      { method: 'POST' }
    );
  }

  /**
   * Get default port for datasource type
   */
  getDefaultPort(type: DatasourceType): number {
    const defaults: Record<DatasourceType, number> = {
      postgres: 5432,
      mysql: 3306,
      clickhouse: 9000,
      mssql: 1433,
      snowflake: 443,
      bigquery: 443,
      databricks: 443,
      redshift: 5439,
      sqlite: 0,
    };
    return defaults[type] ?? 5432;
  }

  /**
   * Validate connection details
   * Validates based on datasource type and auth method
   */
  validateConnectionDetails(
    connectionDetails: Partial<ConnectionDetails> & { 
      auth_method?: string;
      tenant_id?: string;
      client_id?: string;
      client_secret?: string;
    }
  ): boolean {
    const { type, host, user, name, port, ssl_mode, password, auth_method, tenant_id, client_id, client_secret } =
      connectionDetails;
    const errors: string[] = [];

    // Common required fields for all datasources
    if (!type) errors.push('Datasource type is required');
    if (!host) errors.push('Host is required');
    if (!name) errors.push('Database name is required');
    if (!port) errors.push('Port is required');
    if (!ssl_mode) errors.push('SSL mode is required');

    // MSSQL-specific validation based on auth method
    if (type === 'mssql') {
      if (auth_method === 'sql') {
        // SQL Authentication: requires username and password
        if (!user) errors.push('Username is required for SQL authentication');
        if (!password) errors.push('Password is required for SQL authentication');
      } else if (auth_method === 'service_principal') {
        // Service Principal: requires tenant_id, client_id, client_secret
        // NO username/password needed
        if (!tenant_id) errors.push('Azure Tenant ID is required for Service Principal authentication');
        if (!client_id) errors.push('Azure Client ID is required for Service Principal authentication');
        if (!client_secret) errors.push('Azure Client Secret is required for Service Principal authentication');
      } else if (auth_method === 'user_delegation') {
        // User Delegation: no credentials needed (token from JWT)
        // NO username/password/client credentials needed
        // Validation will happen server-side when checking for Azure token in context
      } else {
        errors.push('Authentication method is required for MSSQL (sql, service_principal, or user_delegation)');
      }
    } else {
      // PostgreSQL and other datasources: require username
      // Password is typically required but may be optional for some auth methods
      if (!user) errors.push('Username is required');
      // Note: Password validation is lenient here - some databases allow empty passwords
      // Backend will handle actual authentication validation
    }

    if (errors.length > 0) {
      throw new Error(`Validation errors: ${errors.join(', ')}`);
    }

    return true;
  }

  // --- Installed Apps Methods ---

  /**
   * List all installed apps for a project
   * GET /api/projects/{projectId}/apps
   */
  async listInstalledApps(
    projectId: string
  ): Promise<ApiResponse<InstalledAppsResponse>> {
    return this.makeRequest<InstalledAppsResponse>(`/${projectId}/apps`);
  }

  /**
   * Get a specific installed app
   * GET /api/projects/{projectId}/apps/{appId}
   * Returns 404 if not installed
   */
  async getInstalledApp(
    projectId: string,
    appId: string
  ): Promise<ApiResponse<InstalledApp>> {
    return this.makeRequest<InstalledApp>(`/${projectId}/apps/${appId}`);
  }

  /**
   * Install an app for a project
   * POST /api/projects/{projectId}/apps/{appId}
   */
  async installApp(
    projectId: string,
    appId: string
  ): Promise<ApiResponse<InstalledApp>> {
    return this.makeRequest<InstalledApp>(`/${projectId}/apps/${appId}`, {
      method: 'POST',
    });
  }

  /**
   * Uninstall an app from a project
   * DELETE /api/projects/{projectId}/apps/{appId}
   */
  async uninstallApp(
    projectId: string,
    appId: string
  ): Promise<ApiResponse<void>> {
    return this.makeRequest<void>(`/${projectId}/apps/${appId}`, {
      method: 'DELETE',
    });
  }

  /**
   * Get app settings
   * GET /api/projects/{projectId}/apps/{appId}
   * Same as getInstalledApp - settings are in the response
   */
  async getAppSettings(
    projectId: string,
    appId: string
  ): Promise<ApiResponse<InstalledApp>> {
    return this.getInstalledApp(projectId, appId);
  }

  /**
   * Update app settings
   * PATCH /api/projects/{projectId}/apps/{appId}
   */
  async updateAppSettings(
    projectId: string,
    appId: string,
    settings: Record<string, unknown>
  ): Promise<ApiResponse<InstalledApp>> {
    return this.makeRequest<InstalledApp>(`/${projectId}/apps/${appId}`, {
      method: 'PATCH',
      body: JSON.stringify({ settings }),
    });
  }

  // --- Ontology DAG Methods ---

  /**
   * Get project overview (for repopulating textarea on re-extraction)
   * GET /api/projects/{projectId}/project-knowledge/overview
   */
  async getProjectOverview(
    projectId: string
  ): Promise<ApiResponse<{ overview: string | null }>> {
    return this.makeRequest<{ overview: string | null }>(
      `/${projectId}/project-knowledge/overview`
    );
  }

  /**
   * Start or refresh ontology extraction (DAG-based)
   * POST /api/projects/{projectId}/datasources/{datasourceId}/ontology/extract
   */
  async startOntologyExtraction(
    projectId: string,
    datasourceId: string,
    projectOverview?: string
  ): Promise<ApiResponse<DAGStatusResponse>> {
    return this.makeRequest<DAGStatusResponse>(
      `/${projectId}/datasources/${datasourceId}/ontology/extract`,
      {
        method: 'POST',
        body: JSON.stringify({ project_overview: projectOverview }),
      }
    );
  }

  /**
   * Get ontology DAG status (for polling)
   * GET /api/projects/{projectId}/datasources/{datasourceId}/ontology/dag
   */
  async getOntologyDAGStatus(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<DAGStatusResponse | null>> {
    return this.makeRequest<DAGStatusResponse | null>(
      `/${projectId}/datasources/${datasourceId}/ontology/dag`
    );
  }

  /**
   * Cancel a running ontology DAG
   * POST /api/projects/{projectId}/datasources/{datasourceId}/ontology/dag/cancel
   */
  async cancelOntologyDAG(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<{ status: string }>> {
    return this.makeRequest<{ status: string }>(
      `/${projectId}/datasources/${datasourceId}/ontology/dag/cancel`,
      { method: 'POST' }
    );
  }

  /**
   * Delete all ontology data for project
   */
  async deleteOntology(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<{ message: string }>> {
    return this.makeRequest<{ message: string }>(
      `/${projectId}/datasources/${datasourceId}/ontology`,
      { method: 'DELETE' }
    );
  }

  /**
   * Delete project and all associated data
   * DELETE /api/projects/{projectId}
   */
  async deleteProject(projectId: string): Promise<void> {
    const url = `${this.baseURL}/${projectId}`;
    const response = await fetchWithAuth(url, { method: 'DELETE' });
    if (!response.ok) {
      const data = await response.json();
      throw new Error(
        data.error ?? data.message ?? `HTTP ${response.status}: ${response.statusText}`
      );
    }
    // DELETE returns 204 No Content on success
  }
}

// Create and export singleton instance
const engineApi = new EngineApiService();
export default engineApi;
