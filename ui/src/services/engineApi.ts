/**
 * Engine API Service
 * Handles communication with the Ekaya Engine REST API
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
  SaveSelectionsResponse,
  SchemaRefreshResponse,
  TestConnectionRequest,
  TestConnectionResponse,
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
