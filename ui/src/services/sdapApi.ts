/**
 * SDAP API Service
 * Handles communication with the Ekaya SDAP REST API
 */

import { fetchWithAuth } from '../lib/api';
import type {
  ApiResponse,
  ConnectionDetails,
  CreateDatasourceResponse,
  CreateRelationshipRequest,
  DatasourceConfig,
  DatasourceSchema,
  DatasourceType,
  DeleteDatasourceResponse,
  DiscoveryResults,
  GetDatasourceResponse,
  ListDatasourcesResponse,
  RelationshipCandidatesResponse,
  RelationshipDetail,
  RelationshipsResponse,
  SaveSelectionsRequest,
  SaveSelectionsResponse,
  SchemaRefreshResponse,
  SelectedTablesResponse,
  TestConnectionRequest,
  TestConnectionResponse,
} from '../types';

const SDAP_BASE_URL = '/sdap/v1';

class SdapApiService {
  private baseURL: string;

  constructor() {
    this.baseURL = SDAP_BASE_URL;
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
          data.error ||
            data.message ||
            `HTTP ${response.status}: ${response.statusText}`
        );
      }

      return data;
    } catch (error) {
      console.error(`SDAP API Error (${endpoint}):`, error);
      throw error;
    }
  }

  /**
   * Test datasource connection
   */
  async testDatasourceConnection(
    connectionDetails: TestConnectionRequest
  ): Promise<ApiResponse<TestConnectionResponse>> {
    return this.makeRequest<TestConnectionResponse>('/test', {
      method: 'POST',
      body: JSON.stringify(connectionDetails),
    });
  }

  /**
   * Create datasource for a project
   */
  async createDataSource({
    projectId,
    datasourceType,
    config,
  }: {
    projectId: string;
    datasourceType: DatasourceType;
    config: DatasourceConfig;
  }): Promise<ApiResponse<CreateDatasourceResponse>> {
    return this.makeRequest<CreateDatasourceResponse>(
      `/${projectId}/datasources`,
      {
        method: 'POST',
        body: JSON.stringify({
          project_id: projectId,
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
   * Note: Database name is omitted from updates - it cannot be changed after creation
   */
  async updateDataSource(
    projectId: string,
    datasourceId: string,
    datasourceType: DatasourceType,
    config: DatasourceConfig
  ): Promise<ApiResponse<CreateDatasourceResponse>> {
    // Database name cannot be changed after creation
    const { name: _name, ...configEdit } = config;

    return this.makeRequest<CreateDatasourceResponse>(
      `/${projectId}/datasources/${datasourceId}`,
      {
        method: 'PUT',
        body: JSON.stringify({
          project_id: projectId,
          type: datasourceType,
          config: configEdit,
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
   * List tables with row counts for a project
   * GET /sdap/v1/{project_id}/tables
   */
  async listTables(projectId: string): Promise<ApiResponse<any>> {
    return this.makeRequest<any>(`/${projectId}/tables`);
  }

  /**
   * Get datasource schema for a project
   * Returns comprehensive schema information including tables, columns, and relationships
   */
  async getSchema(projectId: string): Promise<ApiResponse<DatasourceSchema>> {
    return this.makeRequest<DatasourceSchema>(`/${projectId}/schema`);
  }

  /**
   * Get detailed relationships for a project
   * Returns comprehensive relationship information including type, cardinality, and approval status
   * GET /sdap/v1/{project_id}/schema/relationships
   */
  async getRelationships(projectId: string): Promise<ApiResponse<RelationshipsResponse>> {
    return this.makeRequest<RelationshipsResponse>(`/${projectId}/schema/relationships`);
  }

  /**
   * Create a manual relationship between two columns
   * The relationship will be analyzed to determine cardinality
   * POST /sdap/v1/{project_id}/schema/relationships
   */
  async createRelationship(
    projectId: string,
    request: CreateRelationshipRequest
  ): Promise<ApiResponse<RelationshipDetail>> {
    return this.makeRequest<RelationshipDetail>(
      `/${projectId}/schema/relationships`,
      {
        method: 'POST',
        body: JSON.stringify(request),
      }
    );
  }

  /**
   * Remove a relationship (sets is_approved = false)
   * The relationship remains in the database to prevent re-discovery
   * DELETE /sdap/v1/{project_id}/schema/relationships/{relationship_id}
   */
  async removeRelationship(
    projectId: string,
    relationshipId: string
  ): Promise<ApiResponse<{ message: string }>> {
    return this.makeRequest<{ message: string }>(
      `/${projectId}/schema/relationships/${relationshipId}`,
      {
        method: 'DELETE',
      }
    );
  }

  /**
   * Get saved table selections for a project
   * Returns null if no selections have been saved (default to all tables)
   * @deprecated Use getSchemaSelections() to get both tables and columns
   */
  async getSelectedTables(projectId: string): Promise<string[] | null> {
    const response = await this.makeRequest<SelectedTablesResponse>(
      `/${projectId}/schema/selections`
    );

    // Extract selected_tables from the data object
    return response.data?.selected_tables ?? null;
  }

  /**
   * Get saved schema selections (both tables and columns) for a project
   * Returns both selected tables and columns, or null for each if not saved
   * GET /sdap/v1/{project_id}/schema/selections
   */
  async getSchemaSelections(projectId: string): Promise<{
    tables: string[] | null;
    columns: Record<string, string[]> | null;
  }> {
    const response = await this.makeRequest<SelectedTablesResponse>(
      `/${projectId}/schema/selections`
    );

    return {
      tables: response.data?.selected_tables ?? null,
      columns: response.data?.selected_columns ?? null,
    };
  }

  /**
   * Save schema selections (tables and columns) for a project
   * POST /sdap/v1/{project_id}/schema/selections
   */
  async saveSchemaSelections(
    projectId: string,
    selectedTables: string[],
    selectedColumns: Record<string, string[]>
  ): Promise<ApiResponse<SaveSelectionsResponse>> {
    const requestBody: SaveSelectionsRequest = {
      selected_tables: selectedTables,
      selected_columns: selectedColumns,
    };

    return this.makeRequest<SaveSelectionsResponse>(
      `/${projectId}/schema/selections`,
      {
        method: 'POST',
        body: JSON.stringify(requestBody),
      }
    );
  }

  /**
   * Refresh schema from datasource
   * Re-discovers tables and columns from the datasource and updates the schema cache
   * POST /sdap/v1/{project_id}/schema/refresh
   */
  async refreshSchema(
    projectId: string,
    datasourceId: string
  ): Promise<ApiResponse<SchemaRefreshResponse>> {
    return this.makeRequest<SchemaRefreshResponse>(
      `/${projectId}/schema/refresh`,
      {
        method: 'POST',
        body: JSON.stringify({ datasource_id: datasourceId }),
      }
    );
  }

  // --- Relationship Discovery Methods ---

  /**
   * Discover relationships for a project (synchronous)
   * POST /sdap/v1/{project_id}/schema/relationships/discover
   */
  async discoverRelationships(
    projectId: string
  ): Promise<ApiResponse<DiscoveryResults>> {
    return this.makeRequest<DiscoveryResults>(
      `/${projectId}/schema/relationships/discover`,
      {
        method: 'POST',
      }
    );
  }

  /**
   * Get relationship candidates (verified and rejected)
   * Useful for understanding what was discovered and why some candidates were rejected
   * GET /sdap/v1/{project_id}/schema/relationships/candidates
   */
  async getRelationshipCandidates(
    projectId: string
  ): Promise<ApiResponse<RelationshipCandidatesResponse>> {
    return this.makeRequest<RelationshipCandidatesResponse>(
      `/${projectId}/schema/relationships/candidates`
    );
  }

  /**
   * Check SDAP API health
   */
  async healthCheck(): Promise<ApiResponse<{ status: string }> | null> {
    try {
      return this.makeRequest<{ status: string }>('/health');
    } catch (error) {
      console.error(`SDAP API Error (health check):`, error);
      return null;
    }
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
    return defaults[type] || 5432;
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
const sdapApi = new SdapApiService();
export default sdapApi;
