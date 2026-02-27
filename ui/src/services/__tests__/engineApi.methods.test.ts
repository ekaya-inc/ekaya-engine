import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock fetchWithAuth before importing engineApi
vi.mock('../../lib/api', () => ({
  fetchWithAuth: vi.fn(),
}));

import { fetchWithAuth } from '../../lib/api';
import engineApi from '../engineApi';

const mockFetchWithAuth = vi.mocked(fetchWithAuth);

function mockJsonResponse(data: unknown, status = 200) {
  mockFetchWithAuth.mockResolvedValue({
    status,
    ok: status >= 200 && status < 300,
    statusText: 'OK',
    json: () => Promise.resolve(data),
  } as unknown as Response);
}

function mock204Response() {
  mockFetchWithAuth.mockResolvedValue({
    status: 204,
    ok: true,
    statusText: 'No Content',
    json: () => Promise.reject(new SyntaxError('Unexpected end of JSON input')),
  } as unknown as Response);
}

describe('engineApi datasource methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('createDataSource', () => {
    it('sends POST to /{projectId}/datasources with correct body', async () => {
      const responseData = { data: { datasource_id: 'ds-1' } };
      mockJsonResponse(responseData);

      const result = await engineApi.createDataSource({
        projectId: 'proj-1',
        name: 'My DB',
        datasourceType: 'postgres',
        config: { host: 'localhost', port: 5432 } as any,
      });

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            project_id: 'proj-1',
            name: 'My DB',
            type: 'postgres',
            config: { host: 'localhost', port: 5432 },
          }),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('updateDataSource', () => {
    it('sends PUT to /{projectId}/datasources/{datasourceId} with correct body', async () => {
      const responseData = { data: { datasource_id: 'ds-1' } };
      mockJsonResponse(responseData);

      const result = await engineApi.updateDataSource(
        'proj-1',
        'ds-1',
        'Renamed DB',
        'mysql',
        { host: 'db.example.com', port: 3306 } as any
      );

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify({
            name: 'Renamed DB',
            type: 'mysql',
            config: { host: 'db.example.com', port: 3306 },
          }),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('deleteDataSource', () => {
    it('sends DELETE to /{projectId}/datasources/{datasourceId}', async () => {
      mock204Response();

      await engineApi.deleteDataSource('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1',
        expect.objectContaining({ method: 'DELETE' })
      );
    });
  });

  describe('listDataSources', () => {
    it('sends GET to /{projectId}/datasources', async () => {
      const responseData = { data: { datasources: [{ id: 'ds-1' }] } };
      mockJsonResponse(responseData);

      const result = await engineApi.listDataSources('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      // GET is the default — no explicit method should be set
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('getDataSource', () => {
    it('sends GET to /{projectId}/datasources/{datasourceId}', async () => {
      const responseData = { data: { datasource_id: 'ds-1', name: 'My DB' } };
      mockJsonResponse(responseData);

      const result = await engineApi.getDataSource('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('renameDatasource', () => {
    it('sends PATCH to /{projectId}/datasources/{datasourceId}/name with name body', async () => {
      const responseData = { data: { datasource_id: 'ds-1', name: 'New Name' } };
      mockJsonResponse(responseData);

      const result = await engineApi.renameDatasource('proj-1', 'ds-1', 'New Name');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/name',
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ name: 'New Name' }),
        })
      );
      expect(result).toEqual(responseData);
    });
  });
});

describe('engineApi schema operation methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('getSchema', () => {
    it('sends GET to /{projectId}/datasources/{datasourceId}/schema', async () => {
      const responseData = {
        data: {
          tables: [{ id: 't-1', name: 'users', columns: [] }],
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.getSchema('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/schema',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('refreshSchema', () => {
    it('sends POST to /{projectId}/datasources/{datasourceId}/schema/refresh', async () => {
      const responseData = { data: { status: 'refreshed', tables_count: 5 } };
      mockJsonResponse(responseData);

      const result = await engineApi.refreshSchema('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/schema/refresh',
        expect.objectContaining({ method: 'POST' })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('getRelationships', () => {
    it('sends GET to /{projectId}/relationships', async () => {
      const responseData = {
        data: {
          relationships: [{ id: 'r-1', type: 'one_to_many' }],
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.getRelationships('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/relationships',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('createRelationship', () => {
    it('sends POST to /{projectId}/datasources/{datasourceId}/schema/relationships with body', async () => {
      const request = {
        source_column_id: 'col-1',
        target_column_id: 'col-2',
      };
      const responseData = { data: { id: 'r-1', ...request } };
      mockJsonResponse(responseData);

      const result = await engineApi.createRelationship('proj-1', 'ds-1', request as any);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/schema/relationships',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('removeRelationship', () => {
    it('sends DELETE to /{projectId}/datasources/{datasourceId}/schema/relationships/{relationshipId}', async () => {
      const responseData = { data: { message: 'Relationship removed' } };
      mockJsonResponse(responseData);

      const result = await engineApi.removeRelationship('proj-1', 'ds-1', 'r-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/schema/relationships/r-1',
        expect.objectContaining({ method: 'DELETE' })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('saveSchemaSelections', () => {
    it('sends POST to /{projectId}/datasources/{datasourceId}/schema/selections with table and column selections', async () => {
      const tableSelections = { 'table-uuid-1': true, 'table-uuid-2': false };
      const columnSelections = { 'table-uuid-1': ['col-uuid-1', 'col-uuid-2'] };
      const responseData = { data: { updated: true } };
      mockJsonResponse(responseData);

      const result = await engineApi.saveSchemaSelections(
        'proj-1',
        'ds-1',
        tableSelections,
        columnSelections
      );

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/schema/selections',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            table_selections: tableSelections,
            column_selections: columnSelections,
          }),
        })
      );
      expect(result).toEqual(responseData);
    });
  });
});

describe('engineApi query CRUD methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('listQueries', () => {
    it('sends GET to /{projectId}/datasources/{datasourceId}/queries', async () => {
      const responseData = {
        data: { queries: [{ query_id: 'q-1', natural_language_prompt: 'get users' }] },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.listQueries('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/queries',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('getQuery', () => {
    it('sends GET to /{projectId}/datasources/{datasourceId}/queries/{queryId}', async () => {
      const responseData = {
        data: { query_id: 'q-1', natural_language_prompt: 'get users', sql_query: 'SELECT * FROM users' },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.getQuery('proj-1', 'ds-1', 'q-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/queries/q-1',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('createQuery', () => {
    it('sends POST to /{projectId}/datasources/{datasourceId}/queries with correct body', async () => {
      const request = {
        natural_language_prompt: 'get all users',
        sql_query: 'SELECT * FROM users',
        is_enabled: true,
      };
      const responseData = { data: { query_id: 'q-1', ...request } };
      mockJsonResponse(responseData);

      const result = await engineApi.createQuery('proj-1', 'ds-1', request as any);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/queries',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('updateQuery', () => {
    it('sends PUT to /{projectId}/datasources/{datasourceId}/queries/{queryId} with correct body', async () => {
      const request = {
        natural_language_prompt: 'get active users',
        sql_query: 'SELECT * FROM users WHERE active = true',
      };
      const responseData = { data: { query_id: 'q-1', ...request } };
      mockJsonResponse(responseData);

      const result = await engineApi.updateQuery('proj-1', 'ds-1', 'q-1', request as any);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/queries/q-1',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('deleteQuery', () => {
    it('sends DELETE to /{projectId}/datasources/{datasourceId}/queries/{queryId}', async () => {
      const responseData = { data: { success: true, message: 'Query deleted' } };
      mockJsonResponse(responseData);

      const result = await engineApi.deleteQuery('proj-1', 'ds-1', 'q-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/queries/q-1',
        expect.objectContaining({ method: 'DELETE' })
      );
      expect(result).toEqual(responseData);
    });
  });
});

describe('engineApi query execution methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('executeQuery', () => {
    it('sends POST to /{projectId}/datasources/{datasourceId}/queries/{queryId}/execute with request body', async () => {
      const responseData = {
        data: {
          columns: [{ name: 'id', type: 'integer' }, { name: 'name', type: 'text' }],
          rows: [{ id: 1, name: 'Alice' }],
          row_count: 1,
        },
      };
      mockJsonResponse(responseData);

      const request = { limit: 50, parameters: { status: 'active' } };
      const result = await engineApi.executeQuery('proj-1', 'ds-1', 'q-1', request);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/queries/q-1/execute',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });

    it('sends POST without body when no request is provided', async () => {
      const responseData = {
        data: {
          columns: [{ name: 'id', type: 'integer' }],
          rows: [{ id: 1 }],
          row_count: 1,
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.executeQuery('proj-1', 'ds-1', 'q-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/queries/q-1/execute',
        expect.objectContaining({ method: 'POST' })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.body).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('testQuery', () => {
    it('sends POST to /{projectId}/datasources/{datasourceId}/queries/test with SQL body', async () => {
      const responseData = {
        data: {
          columns: [{ name: 'count', type: 'bigint' }],
          rows: [{ count: 42 }],
          row_count: 1,
        },
      };
      mockJsonResponse(responseData);

      const request = { sql_query: 'SELECT COUNT(*) AS count FROM users', limit: 100 };
      const result = await engineApi.testQuery('proj-1', 'ds-1', request as any);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/queries/test',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('validateQuery', () => {
    it('sends POST to /{projectId}/datasources/{datasourceId}/queries/validate with SQL body', async () => {
      const responseData = {
        data: { valid: true, message: 'SQL is valid' },
      };
      mockJsonResponse(responseData);

      const request = { sql_query: 'SELECT * FROM users' };
      const result = await engineApi.validateQuery('proj-1', 'ds-1', request);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/queries/validate',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });

    it('returns validation errors for invalid SQL', async () => {
      const responseData = {
        data: {
          valid: false,
          message: 'Syntax error near "SELEC"',
          warnings: ['Unknown keyword: SELEC'],
        },
      };
      mockJsonResponse(responseData);

      const request = { sql_query: 'SELEC * FROM users' };
      const result = await engineApi.validateQuery('proj-1', 'ds-1', request);

      expect(result).toEqual(responseData);
      expect(result.data.valid).toBe(false);
      expect(result.data.warnings).toHaveLength(1);
    });
  });
});

describe('engineApi project knowledge methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('listProjectKnowledge', () => {
    it('sends GET to /{projectId}/project-knowledge', async () => {
      const responseData = {
        data: {
          facts: [{ id: 'pk-1', fact_type: 'business_rule', value: 'Revenue is calculated monthly' }],
          total: 1,
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.listProjectKnowledge('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/project-knowledge',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('createProjectKnowledge', () => {
    it('sends POST to /{projectId}/project-knowledge with correct body', async () => {
      const request = {
        fact_type: 'business_rule',
        value: 'All dates are in UTC',
        context: 'Applies to all timestamp columns',
      };
      const responseData = { data: { id: 'pk-1', project_id: 'proj-1', ...request } };
      mockJsonResponse(responseData);

      const result = await engineApi.createProjectKnowledge('proj-1', request);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/project-knowledge',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('updateProjectKnowledge', () => {
    it('sends PUT to /{projectId}/project-knowledge/{id} with correct body', async () => {
      const request = {
        fact_type: 'convention',
        value: 'All monetary values are in cents',
        context: 'Applies to price and amount columns',
      };
      const responseData = { data: { id: 'pk-1', project_id: 'proj-1', ...request } };
      mockJsonResponse(responseData);

      const result = await engineApi.updateProjectKnowledge('proj-1', 'pk-1', request);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/project-knowledge/pk-1',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('deleteProjectKnowledge', () => {
    it('sends DELETE to /{projectId}/project-knowledge/{id}', async () => {
      mock204Response();

      await engineApi.deleteProjectKnowledge('proj-1', 'pk-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/project-knowledge/pk-1',
        expect.objectContaining({ method: 'DELETE' })
      );
    });
  });
});

describe('engineApi AI config methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('getAIConfig', () => {
    it('sends GET to /{projectId}/ai-config', async () => {
      const responseData = {
        data: {
          project_id: 'proj-1',
          config_type: 'byok',
          llm_base_url: 'https://api.openai.com/v1',
          llm_model: 'gpt-4o',
          embedding_base_url: 'https://api.openai.com/v1',
          embedding_model: 'text-embedding-3-small',
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.getAIConfig('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ai-config',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('saveAIConfig', () => {
    it('sends PUT to /{projectId}/ai-config with config body', async () => {
      const config = {
        config_type: 'byok',
        llm_base_url: 'https://api.openai.com/v1',
        llm_api_key: 'sk-test-key',
        llm_model: 'gpt-4o',
        embedding_base_url: 'https://api.openai.com/v1',
        embedding_api_key: 'sk-test-key',
        embedding_model: 'text-embedding-3-small',
      };
      const responseData = {
        data: {
          project_id: 'proj-1',
          ...config,
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.saveAIConfig('proj-1', config);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ai-config',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify(config),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('deleteAIConfig', () => {
    it('sends DELETE to /{projectId}/ai-config and handles 204 response', async () => {
      mock204Response();

      await engineApi.deleteAIConfig('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ai-config',
        expect.objectContaining({ method: 'DELETE' })
      );
    });
  });

  describe('testAIConnection', () => {
    it('sends POST to /{projectId}/ai-config/test with config body', async () => {
      const testRequest = {
        config_type: 'byok',
        llm_base_url: 'https://api.openai.com/v1',
        llm_api_key: 'sk-test-key',
        llm_model: 'gpt-4o',
        embedding_base_url: 'https://api.openai.com/v1',
        embedding_api_key: 'sk-test-key',
        embedding_model: 'text-embedding-3-small',
      };
      const responseData = {
        data: {
          success: true,
          message: 'Connection successful',
          llm_success: true,
          llm_message: 'LLM responded correctly',
          llm_response_time_ms: 342,
          embedding_success: true,
          embedding_message: 'Embedding model responded correctly',
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.testAIConnection('proj-1', testRequest);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ai-config/test',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(testRequest),
        })
      );
      expect(result).toEqual(responseData);
    });

    it('returns failure details when connection test fails', async () => {
      const testRequest = {
        llm_base_url: 'https://invalid-url.example.com',
        llm_model: 'gpt-4o',
      };
      const responseData = {
        data: {
          success: false,
          message: 'Connection failed',
          llm_success: false,
          llm_message: 'Could not reach endpoint',
          llm_error_type: 'endpoint',
          embedding_success: false,
          embedding_message: 'No embedding config provided',
          embedding_error_type: 'unknown',
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.testAIConnection('proj-1', testRequest);

      expect(result).toEqual(responseData);
      expect(result.data.success).toBe(false);
      expect(result.data.llm_error_type).toBe('endpoint');
    });
  });
});

describe('engineApi alerts methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('getAuditAlerts', () => {
    it('sends GET to /{projectId}/audit/alerts', async () => {
      const responseData = {
        data: {
          items: [{ id: 'alert-1', severity: 'high', message: 'Suspicious query' }],
          total: 1,
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.getAuditAlerts('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/audit/alerts',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });

    it('appends query params when provided', async () => {
      const responseData = { data: { items: [], total: 0 } };
      mockJsonResponse(responseData);

      await engineApi.getAuditAlerts('proj-1', { severity: 'high', status: 'open' });

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/audit/alerts?severity=high&status=open',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
    });
  });

  describe('resolveAuditAlert', () => {
    it('sends POST to /{projectId}/audit/alerts/{alertId}/resolve with body', async () => {
      const responseData = { data: { message: 'Alert resolved' } };
      mockJsonResponse(responseData);

      const body = { resolution: 'false_positive', notes: 'Not a real threat' };
      const result = await engineApi.resolveAuditAlert('proj-1', 'alert-1', body as any);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/audit/alerts/alert-1/resolve',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(body),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('getAlertConfig', () => {
    it('sends GET to /{projectId}/audit/alert-config', async () => {
      const responseData = {
        data: { enabled: true, severity_threshold: 'medium', notify_email: true },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.getAlertConfig('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/audit/alert-config',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('updateAlertConfig', () => {
    it('sends PUT to /{projectId}/audit/alert-config with config body', async () => {
      const config = { enabled: false, severity_threshold: 'high', notify_email: false };
      const responseData = { data: config };
      mockJsonResponse(responseData);

      const result = await engineApi.updateAlertConfig('proj-1', config as any);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/audit/alert-config',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify(config),
        })
      );
      expect(result).toEqual(responseData);
    });
  });
});

describe('engineApi approved query methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('listPendingQueries', () => {
    it('sends GET to /{projectId}/queries/pending', async () => {
      const responseData = {
        data: {
          queries: [
            { query_id: 'q-1', natural_language_prompt: 'get users', status: 'pending' },
          ],
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.listPendingQueries('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/queries/pending',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('approveQuery', () => {
    it('sends POST to /{projectId}/queries/{queryId}/approve', async () => {
      const responseData = { data: { success: true, message: 'Query approved' } };
      mockJsonResponse(responseData);

      const result = await engineApi.approveQuery('proj-1', 'q-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/queries/q-1/approve',
        expect.objectContaining({ method: 'POST' })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('rejectQuery', () => {
    it('sends POST to /{projectId}/queries/{queryId}/reject with reason body', async () => {
      const responseData = { data: { success: true, message: 'Query rejected' } };
      mockJsonResponse(responseData);

      const result = await engineApi.rejectQuery('proj-1', 'q-1', 'SQL is too broad');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/queries/q-1/reject',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ reason: 'SQL is too broad' }),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('moveToPending', () => {
    it('sends POST to /{projectId}/queries/{queryId}/move-to-pending', async () => {
      const responseData = { data: { success: true, message: 'Query moved to pending' } };
      mockJsonResponse(responseData);

      const result = await engineApi.moveToPending('proj-1', 'q-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/queries/q-1/move-to-pending',
        expect.objectContaining({ method: 'POST' })
      );
      expect(result).toEqual(responseData);
    });
  });
});

describe('engineApi glossary methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('listGlossaryTerms', () => {
    it('sends GET to /{projectId}/glossary', async () => {
      const responseData = {
        data: {
          terms: [{ id: 'term-1', name: 'Revenue', definition: 'Total income' }],
          total: 1,
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.listGlossaryTerms('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/glossary',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('createGlossaryTerm', () => {
    it('sends POST to /{projectId}/glossary with correct body', async () => {
      const request = {
        name: 'Revenue',
        definition: 'Total income from sales',
        sql_expression: 'SUM(orders.amount)',
      };
      const responseData = { data: { id: 'term-1', ...request } };
      mockJsonResponse(responseData);

      const result = await engineApi.createGlossaryTerm('proj-1', request as any);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/glossary',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('updateGlossaryTerm', () => {
    it('sends PUT to /{projectId}/glossary/{termId} with correct body', async () => {
      const request = {
        name: 'Net Revenue',
        definition: 'Total income minus refunds',
        sql_expression: 'SUM(orders.amount) - SUM(refunds.amount)',
      };
      const responseData = { data: { id: 'term-1', ...request } };
      mockJsonResponse(responseData);

      const result = await engineApi.updateGlossaryTerm('proj-1', 'term-1', request as any);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/glossary/term-1',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('deleteGlossaryTerm', () => {
    it('sends DELETE to /{projectId}/glossary/{termId}', async () => {
      mock204Response();

      await engineApi.deleteGlossaryTerm('proj-1', 'term-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/glossary/term-1',
        expect.objectContaining({ method: 'DELETE' })
      );
    });
  });

  describe('testGlossarySQL', () => {
    it('sends POST to /{projectId}/glossary/test-sql with sql body', async () => {
      const responseData = {
        data: {
          columns: [{ name: 'total', type: 'numeric' }],
          rows: [{ total: 12345.67 }],
          row_count: 1,
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.testGlossarySQL('proj-1', 'SUM(orders.amount)');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/glossary/test-sql',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ sql: 'SUM(orders.amount)' }),
        })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('autoGenerateGlossary', () => {
    it('sends POST to /{projectId}/glossary/auto-generate', async () => {
      const responseData = {
        data: { status: 'started', message: 'Glossary generation initiated' },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.autoGenerateGlossary('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/glossary/auto-generate',
        expect.objectContaining({ method: 'POST' })
      );
      expect(result).toEqual(responseData);
    });
  });
});

describe('engineApi ontology change methods', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('startOntologyExtraction', () => {
    it('sends POST to /{projectId}/datasources/{datasourceId}/ontology/extract with overview', async () => {
      const responseData = {
        data: { dag_id: 'dag-1', status: 'running', nodes: [] },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.startOntologyExtraction('proj-1', 'ds-1', 'A retail platform');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/ontology/extract',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ project_overview: 'A retail platform' }),
        })
      );
      expect(result).toEqual(responseData);
    });

    it('sends POST with undefined overview when not provided', async () => {
      const responseData = { data: { dag_id: 'dag-1', status: 'running', nodes: [] } };
      mockJsonResponse(responseData);

      await engineApi.startOntologyExtraction('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/ontology/extract',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ project_overview: undefined }),
        })
      );
    });
  });

  describe('getOntologyDAGStatus', () => {
    it('sends GET to /{projectId}/datasources/{datasourceId}/ontology/dag', async () => {
      const responseData = {
        data: { dag_id: 'dag-1', status: 'completed', nodes: [] },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.getOntologyDAGStatus('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/ontology/dag',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('cancelOntologyDAG', () => {
    it('sends POST to /{projectId}/datasources/{datasourceId}/ontology/dag/cancel', async () => {
      const responseData = { data: { status: 'cancelled' } };
      mockJsonResponse(responseData);

      const result = await engineApi.cancelOntologyDAG('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/ontology/dag/cancel',
        expect.objectContaining({ method: 'POST' })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('deleteOntology', () => {
    it('sends DELETE to /{projectId}/datasources/{datasourceId}/ontology', async () => {
      const responseData = { data: { message: 'Ontology deleted' } };
      mockJsonResponse(responseData);

      const result = await engineApi.deleteOntology('proj-1', 'ds-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/datasources/ds-1/ontology',
        expect.objectContaining({ method: 'DELETE' })
      );
      expect(result).toEqual(responseData);
    });
  });

  describe('getOntologyQuestionCounts', () => {
    it('sends GET to /{projectId}/ontology/questions/counts', async () => {
      const responseData = { data: { required: 3, optional: 7 } };
      mockJsonResponse(responseData);

      const result = await engineApi.getOntologyQuestionCounts('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/questions/counts',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });

  describe('listAuditOntologyChanges', () => {
    it('sends GET to /{projectId}/audit/ontology-changes', async () => {
      const responseData = {
        data: {
          items: [{ id: 'oc-1', change_type: 'add', entity: 'users' }],
          total: 1,
        },
      };
      mockJsonResponse(responseData);

      const result = await engineApi.listAuditOntologyChanges('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/audit/ontology-changes',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });

    it('appends query params when provided', async () => {
      const responseData = { data: { items: [], total: 0 } };
      mockJsonResponse(responseData);

      await engineApi.listAuditOntologyChanges('proj-1', { change_type: 'add' });

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/audit/ontology-changes?change_type=add',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
    });
  });

  describe('getProjectOverview', () => {
    it('sends GET to /{projectId}/project-knowledge/overview', async () => {
      const responseData = { data: { overview: 'A SaaS platform for analytics' } };
      mockJsonResponse(responseData);

      const result = await engineApi.getProjectOverview('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/project-knowledge/overview',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      const callArgs = mockFetchWithAuth.mock.calls[0][1] as RequestInit;
      expect(callArgs.method).toBeUndefined();
      expect(result).toEqual(responseData);
    });
  });
});
