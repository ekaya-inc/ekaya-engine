/**
 * Engine API Service
 * Handles communication with the Ekaya Engine REST API
 */

import { fetchWithAuth } from '../lib/api';
import type {
  ApiResponse,
  ConnectionDetails,
  CreateDatasourceResponse,
  CreateQueryRequest,
  CreateRelationshipRequest,
  DatasourceConfig,
  DatasourceSchema,
  DatasourceType,
  DeleteDatasourceResponse,
  DeleteQueryResponse,
  DiscoveryResults,
  ExecuteQueryRequest,
  ExecuteQueryResponse,
  GetDatasourceResponse,
  ListDatasourcesResponse,
  ListQueriesResponse,
  MCPConfigResponse,
  Query,
  RelationshipCandidatesResponse,
  RelationshipDetail,
  RelationshipsResponse,
  SaveSelectionsResponse,
  SchemaRefreshResponse,
  TestConnectionRequest,
  TestConnectionResponse,
  TestQueryRequest,
  UpdateMCPConfigRequest,
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
   * Get detailed relationships for a datasource
   * Returns comprehensive relationship information including type, cardinality, and approval status
   * GET /api/projects/{projectId}/datasources/{datasourceId}/schema/relationships
   */
  async getRelationships(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<RelationshipsResponse>> {
    return this.makeRequest<RelationshipsResponse>(
      `/${projectId}/datasources/${datasourceId}/schema/relationships`
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

  /**
   * Get relationship candidates (verified and rejected)
   * Useful for understanding what was discovered and why some candidates were rejected
   * GET /api/projects/{projectId}/datasources/{datasourceId}/schema/relationships/candidates
   */
  async getRelationshipCandidates(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<RelationshipCandidatesResponse>> {
    return this.makeRequest<RelationshipCandidatesResponse>(
      `/${projectId}/datasources/${datasourceId}/schema/relationships/candidates`
    );
  }

  // --- Query Management Methods ---

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
   */
  validateConnectionDetails(
    connectionDetails: Partial<ConnectionDetails>
  ): boolean {
    const { type, host, user, name, port, ssl_mode, password } =
      connectionDetails;
    const errors: string[] = [];

    if (!type) errors.push('Datasource type is required');
    if (!host) errors.push('Host is required');
    if (!user) errors.push('Username is required');
    if (!name) errors.push('Datasource name is required');
    if (!port) errors.push('Port is required');
    if (!ssl_mode) errors.push('SSL mode is required');
    if (!password) errors.push('Password is required');

    if (errors.length > 0) {
      throw new Error(`Validation errors: ${errors.join(', ')}`);
    }

    return true;
  }
}

// Create and export singleton instance
const engineApi = new EngineApiService();
export default engineApi;
