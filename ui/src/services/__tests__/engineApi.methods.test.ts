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
