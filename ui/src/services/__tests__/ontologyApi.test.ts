import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('../../lib/api', () => ({
  fetchWithAuth: vi.fn(),
}));

import { fetchWithAuth } from '../../lib/api';
import ontologyApi from '../ontologyApi';

const mockFetchWithAuth = vi.mocked(fetchWithAuth);

function mockJsonResponse(data: unknown, status = 200) {
  mockFetchWithAuth.mockResolvedValue({
    status,
    ok: status >= 200 && status < 300,
    statusText: status === 200 ? 'OK' : 'Error',
    json: () => Promise.resolve(data),
  } as unknown as Response);
}

function mockErrorResponse(status: number, statusText: string, body: unknown = {}) {
  mockFetchWithAuth.mockResolvedValue({
    status,
    ok: false,
    statusText,
    json: () => Promise.resolve(body),
  } as unknown as Response);
}

describe('ontologyApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ==========================================================================
  // Wrapped vs unwrapped response handling
  // ==========================================================================

  describe('makeRequest response unwrapping', () => {
    it('unwraps { data: ... } wrapper from API responses', async () => {
      const inner = { workflow_id: 'wf-1', status: 'running' };
      mockJsonResponse({ data: inner });

      const result = await ontologyApi.getStatus('proj-1');
      expect(result).toEqual(inner);
    });

    it('returns raw JSON when no data wrapper is present', async () => {
      const raw = { workflow_id: 'wf-1', status: 'running' };
      mockJsonResponse(raw);

      const result = await ontologyApi.getStatus('proj-1');
      expect(result).toEqual(raw);
    });

    it('throws on non-ok responses with status info', async () => {
      mockErrorResponse(404, 'Not Found', { error: 'not found' });

      await expect(ontologyApi.getStatus('proj-1')).rejects.toThrow(
        'HTTP 404: Not Found'
      );
    });
  });

  // ==========================================================================
  // Ontology workflow methods
  // ==========================================================================

  describe('extractOntology', () => {
    it('sends POST to /{projectId}/ontology/extract with empty body when no options', async () => {
      mockJsonResponse({ data: { workflow_id: 'wf-1' } });

      await ontologyApi.extractOntology('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/extract',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({}),
        })
      );
    });

    it('includes selected_tables and project_description when provided', async () => {
      mockJsonResponse({ data: { workflow_id: 'wf-1' } });

      await ontologyApi.extractOntology('proj-1', {
        selectedTables: ['users', 'orders'],
        projectDescription: 'E-commerce app',
      });

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/extract',
        expect.objectContaining({
          body: JSON.stringify({
            selected_tables: ['users', 'orders'],
            project_description: 'E-commerce app',
          }),
        })
      );
    });

    it('omits selected_tables when array is empty', async () => {
      mockJsonResponse({ data: { workflow_id: 'wf-1' } });

      await ontologyApi.extractOntology('proj-1', { selectedTables: [] });

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/extract',
        expect.objectContaining({
          body: JSON.stringify({}),
        })
      );
    });
  });

  describe('getWorkflowResult', () => {
    it('sends GET to /{projectId}/ontology/result', async () => {
      const payload = { entities: [] };
      mockJsonResponse({ data: payload });

      const result = await ontologyApi.getWorkflowResult('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/result',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      expect(result).toEqual(payload);
    });
  });

  describe('getEnrichment', () => {
    it('sends GET to /{projectId}/ontology/enrichment', async () => {
      const payload = { entities: [], columns: [] };
      mockJsonResponse({ data: payload });

      const result = await ontologyApi.getEnrichment('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/enrichment',
        expect.objectContaining({
          headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
        })
      );
      expect(result).toEqual(payload);
    });
  });

  describe('getBusinessRules', () => {
    it('sends GET to /{projectId}/ontology/business-rules', async () => {
      mockJsonResponse({ data: [] });

      await ontologyApi.getBusinessRules('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/business-rules',
        expect.any(Object)
      );
    });

    it('appends entity_id query param when provided', async () => {
      mockJsonResponse({ data: [] });

      await ontologyApi.getBusinessRules('proj-1', 'entity-42');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/business-rules?entity_id=entity-42',
        expect.any(Object)
      );
    });
  });

  describe('getStatus', () => {
    it('sends GET to /{projectId}/ontology/workflow', async () => {
      const status = { workflow_id: 'wf-1', is_complete: false };
      mockJsonResponse({ data: status });

      const result = await ontologyApi.getStatus('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/workflow',
        expect.any(Object)
      );
      expect(result).toEqual(status);
    });
  });

  describe('getWorkflowById', () => {
    it('sends GET to /{projectId}/ontology/workflow/{workflowId}', async () => {
      const status = { workflow_id: 'wf-42', is_complete: true };
      mockJsonResponse({ data: status });

      const result = await ontologyApi.getWorkflowById('proj-1', 'wf-42');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/workflow/wf-42',
        expect.any(Object)
      );
      expect(result).toEqual(status);
    });
  });

  describe('getQuestions', () => {
    it('sends GET to /{projectId}/ontology/questions', async () => {
      const questions = { questions: [{ id: 'q-1', text: 'What?' }] };
      mockJsonResponse({ data: questions });

      const result = await ontologyApi.getQuestions('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/questions',
        expect.any(Object)
      );
      expect(result).toEqual(questions);
    });
  });

  describe('submitProjectAnswers', () => {
    it('sends POST to /{projectId}/ontology/answers with request body', async () => {
      const request = { answers: [{ question_id: 'q-1', answer: 'Yes' }] };
      mockJsonResponse({ data: { accepted: 1 } });

      const result = await ontologyApi.submitProjectAnswers('proj-1', request as any);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/answers',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify(request),
        })
      );
      expect(result).toEqual({ accepted: 1 });
    });
  });

  describe('cancelWorkflow', () => {
    it('sends POST to /{projectId}/ontology/cancel', async () => {
      const response = { workflow_id: 'wf-1', status: 'cancelled', message: 'Cancelled' };
      mockJsonResponse(response);

      const result = await ontologyApi.cancelWorkflow('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/cancel',
        expect.objectContaining({ method: 'POST' })
      );
      expect(result).toEqual(response);
    });
  });

  describe('deleteOntology', () => {
    it('sends DELETE to /{projectId}/ontology', async () => {
      const response = { message: 'Deleted' };
      mockJsonResponse(response);

      const result = await ontologyApi.deleteOntology('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology',
        expect.objectContaining({ method: 'DELETE' })
      );
      expect(result).toEqual(response);
    });
  });

  // ==========================================================================
  // Question-by-Question methods
  // ==========================================================================

  describe('getNextQuestion', () => {
    it('sends GET to /{projectId}/ontology/questions/next without params by default', async () => {
      const question = { question_id: 'q-1', text: 'What is users?' };
      mockJsonResponse({ data: question });

      const result = await ontologyApi.getNextQuestion('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/questions/next',
        expect.any(Object)
      );
      expect(result).toEqual(question);
    });

    it('appends include_skipped=true when requested', async () => {
      mockJsonResponse({ data: {} });

      await ontologyApi.getNextQuestion('proj-1', true);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/questions/next?include_skipped=true',
        expect.any(Object)
      );
    });
  });

  describe('answerQuestion', () => {
    it('sends POST to /{projectId}/ontology/questions/{questionId}/answer with answer body', async () => {
      const response = { status: 'accepted' };
      mockJsonResponse({ data: response });

      const result = await ontologyApi.answerQuestion('proj-1', 'q-1', 'The users table stores customer data');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/questions/q-1/answer',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ answer: 'The users table stores customer data' }),
        })
      );
      expect(result).toEqual(response);
    });
  });

  describe('skipQuestion', () => {
    it('sends POST to /{projectId}/ontology/questions/{questionId}/skip', async () => {
      const response = { status: 'skipped' };
      mockJsonResponse({ data: response });

      const result = await ontologyApi.skipQuestion('proj-1', 'q-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/questions/q-1/skip',
        expect.objectContaining({ method: 'POST' })
      );
      expect(result).toEqual(response);
    });
  });

  describe('deleteQuestion', () => {
    it('sends DELETE to /{projectId}/ontology/questions/{questionId}', async () => {
      const response = { status: 'deleted' };
      mockJsonResponse({ data: response });

      const result = await ontologyApi.deleteQuestion('proj-1', 'q-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/questions/q-1',
        expect.objectContaining({ method: 'DELETE' })
      );
      expect(result).toEqual(response);
    });
  });

  // ==========================================================================
  // Chat methods
  // ==========================================================================

  describe('initializeChat', () => {
    it('sends POST to /{projectId}/ontology/chat/initialize', async () => {
      const response = { status: 'ready', has_pending_questions: true, pending_count: 3 };
      mockJsonResponse({ data: response });

      const result = await ontologyApi.initializeChat('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/chat/initialize',
        expect.objectContaining({ method: 'POST' })
      );
      expect(result).toEqual(response);
    });
  });

  describe('getChatHistory', () => {
    it('sends GET to /{projectId}/ontology/chat/history without limit', async () => {
      const response = { messages: [], total: 0 };
      mockJsonResponse({ data: response });

      const result = await ontologyApi.getChatHistory('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/chat/history',
        expect.any(Object)
      );
      expect(result).toEqual(response);
    });

    it('appends limit query param when provided', async () => {
      mockJsonResponse({ data: { messages: [], total: 0 } });

      await ontologyApi.getChatHistory('proj-1', 50);

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/chat/history?limit=50',
        expect.any(Object)
      );
    });
  });

  describe('clearChatHistory', () => {
    it('sends DELETE to /{projectId}/ontology/chat/history', async () => {
      const response = { success: true, message: 'History cleared' };
      mockJsonResponse(response);

      const result = await ontologyApi.clearChatHistory('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/chat/history',
        expect.objectContaining({ method: 'DELETE' })
      );
      expect(result).toEqual(response);
    });
  });

  describe('getProjectKnowledge', () => {
    it('sends GET to /{projectId}/ontology/knowledge without factType', async () => {
      const response = { knowledge: [], total: 0 };
      mockJsonResponse({ data: response });

      const result = await ontologyApi.getProjectKnowledge('proj-1');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/knowledge',
        expect.any(Object)
      );
      expect(result).toEqual(response);
    });

    it('appends fact_type query param when provided', async () => {
      mockJsonResponse({ data: { knowledge: [], total: 0 } });

      await ontologyApi.getProjectKnowledge('proj-1', 'business_rule');

      expect(mockFetchWithAuth).toHaveBeenCalledWith(
        '/api/projects/proj-1/ontology/knowledge?fact_type=business_rule',
        expect.any(Object)
      );
    });
  });

  // ==========================================================================
  // pollStatus
  // ==========================================================================

  describe('pollStatus', () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it('resolves immediately when workflow is already complete', async () => {
      const completeStatus = {
        workflow_id: 'wf-1',
        is_complete: true,
        current_phase: 'done',
      };
      mockJsonResponse({ data: completeStatus });

      const promise = ontologyApi.pollStatus('proj-1');
      const result = await promise;

      expect(result).toEqual(completeStatus);
      expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);
    });

    it('polls until workflow is complete', async () => {
      const inProgress = { workflow_id: 'wf-1', is_complete: false };
      const complete = { workflow_id: 'wf-1', is_complete: true };

      mockFetchWithAuth
        .mockResolvedValueOnce({
          status: 200, ok: true, statusText: 'OK',
          json: () => Promise.resolve({ data: inProgress }),
        } as unknown as Response)
        .mockResolvedValueOnce({
          status: 200, ok: true, statusText: 'OK',
          json: () => Promise.resolve({ data: complete }),
        } as unknown as Response);

      const promise = ontologyApi.pollStatus('proj-1', { intervalMs: 1000 });

      // First poll returns in-progress — advance timer for next poll
      await vi.advanceTimersByTimeAsync(1000);

      const result = await promise;
      expect(result).toEqual(complete);
      expect(mockFetchWithAuth).toHaveBeenCalledTimes(2);
    });

    it('calls onStatusUpdate callback on each poll', async () => {
      const status1 = { workflow_id: 'wf-1', is_complete: false };
      const status2 = { workflow_id: 'wf-1', is_complete: true };
      const onStatusUpdate = vi.fn();

      mockFetchWithAuth
        .mockResolvedValueOnce({
          status: 200, ok: true, statusText: 'OK',
          json: () => Promise.resolve({ data: status1 }),
        } as unknown as Response)
        .mockResolvedValueOnce({
          status: 200, ok: true, statusText: 'OK',
          json: () => Promise.resolve({ data: status2 }),
        } as unknown as Response);

      const promise = ontologyApi.pollStatus('proj-1', {
        intervalMs: 1000,
        onStatusUpdate,
      });

      await vi.advanceTimersByTimeAsync(1000);
      await promise;

      expect(onStatusUpdate).toHaveBeenCalledTimes(2);
      expect(onStatusUpdate).toHaveBeenCalledWith(status1);
      expect(onStatusUpdate).toHaveBeenCalledWith(status2);
    });

    it('rejects when workflow has errors with severity "error"', async () => {
      const errorStatus = {
        workflow_id: 'wf-1',
        is_complete: false,
        errors: [{ severity: 'error', message: 'Schema extraction failed' }],
      };
      mockJsonResponse({ data: errorStatus });

      await expect(
        ontologyApi.pollStatus('proj-1')
      ).rejects.toThrow('Schema extraction failed');
    });

    it('continues polling when errors only have non-error severity', async () => {
      const warningStatus = {
        workflow_id: 'wf-1',
        is_complete: false,
        errors: [{ severity: 'warning', message: 'Minor issue' }],
      };
      const complete = { workflow_id: 'wf-1', is_complete: true };

      mockFetchWithAuth
        .mockResolvedValueOnce({
          status: 200, ok: true, statusText: 'OK',
          json: () => Promise.resolve({ data: warningStatus }),
        } as unknown as Response)
        .mockResolvedValueOnce({
          status: 200, ok: true, statusText: 'OK',
          json: () => Promise.resolve({ data: complete }),
        } as unknown as Response);

      const promise = ontologyApi.pollStatus('proj-1', { intervalMs: 500 });
      await vi.advanceTimersByTimeAsync(500);

      const result = await promise;
      expect(result).toEqual(complete);
    });

    it('rejects when getStatus throws a network error', async () => {
      mockFetchWithAuth.mockRejectedValue(new Error('Network error'));

      await expect(
        ontologyApi.pollStatus('proj-1')
      ).rejects.toThrow('Network error');
    });

    it('uses default interval of 2000ms', async () => {
      const inProgress = { workflow_id: 'wf-1', is_complete: false };
      const complete = { workflow_id: 'wf-1', is_complete: true };

      mockFetchWithAuth
        .mockResolvedValueOnce({
          status: 200, ok: true, statusText: 'OK',
          json: () => Promise.resolve({ data: inProgress }),
        } as unknown as Response)
        .mockResolvedValueOnce({
          status: 200, ok: true, statusText: 'OK',
          json: () => Promise.resolve({ data: complete }),
        } as unknown as Response);

      const promise = ontologyApi.pollStatus('proj-1');

      // At 1999ms, second poll should NOT have fired
      await vi.advanceTimersByTimeAsync(1999);
      expect(mockFetchWithAuth).toHaveBeenCalledTimes(1);

      // At 2000ms, second poll fires
      await vi.advanceTimersByTimeAsync(1);
      await promise;

      expect(mockFetchWithAuth).toHaveBeenCalledTimes(2);
    });
  });
});
