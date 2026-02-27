import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { transformEntityQueue, transformTaskQueue, transformQuestions } from '../ontologyService';
import type { EntityProgressResponse, TaskProgressResponse, WorkflowQuestion, WorkflowStatusResponse } from '../../types';

// Mock ontologyApi before importing ontologyService (which imports it)
vi.mock('../ontologyApi', () => ({
  default: {
    getStatus: vi.fn(),
    getQuestions: vi.fn(),
    extractOntology: vi.fn(),
    cancelWorkflow: vi.fn(),
    deleteOntology: vi.fn(),
    submitProjectAnswers: vi.fn(),
  },
}));

import ontologyApi from '../ontologyApi';
import { ontologyService } from '../ontologyService';

const mockOntologyApi = vi.mocked(ontologyApi);

function makeStatusResponse(overrides: Partial<WorkflowStatusResponse> = {}): WorkflowStatusResponse {
  return {
    workflow_id: 'wf-1',
    current_phase: 'tier1_generation',
    completed_phases: [],
    confidence_score: 0.5,
    iteration_count: 1,
    is_complete: false,
    status_label: 'Processing',
    status_type: 'processing',
    can_start_new: false,
    has_result: false,
    ...overrides,
  };
}

describe('transformEntityQueue', () => {
  it('returns empty array when input is undefined', () => {
    expect(transformEntityQueue(undefined)).toEqual([]);
  });

  it('returns empty array when input is an empty array', () => {
    expect(transformEntityQueue([])).toEqual([]);
  });

  it('maps entity_name and status to camelCase WorkItem fields', () => {
    const input: EntityProgressResponse[] = [
      { entity_name: 'users', status: 'complete' },
      { entity_name: 'orders', status: 'processing' },
    ];

    const result = transformEntityQueue(input);

    expect(result).toEqual([
      { entityName: 'users', status: 'complete' },
      { entityName: 'orders', status: 'processing' },
    ]);
  });

  it('includes optional fields only when present in input', () => {
    const input: EntityProgressResponse[] = [
      {
        entity_name: 'products',
        status: 'complete',
        token_count: 1500,
        last_updated: '2025-01-15T10:30:00Z',
      },
    ];

    const result = transformEntityQueue(input);

    expect(result).toEqual([
      {
        entityName: 'products',
        status: 'complete',
        tokenCount: 1500,
        lastUpdated: '2025-01-15T10:30:00Z',
      },
    ]);
  });

  it('includes error_message as errorMessage when present', () => {
    const input: EntityProgressResponse[] = [
      {
        entity_name: 'payments',
        status: 'failed',
        error_message: 'Connection timeout',
      },
    ];

    const result = transformEntityQueue(input);

    expect(result).toHaveLength(1);
    expect(result[0]).toEqual({
      entityName: 'payments',
      status: 'failed',
      errorMessage: 'Connection timeout',
    });
  });

  it('omits optional fields when they are undefined in input', () => {
    const input: EntityProgressResponse[] = [
      { entity_name: 'sessions', status: 'queued' },
    ];

    const result = transformEntityQueue(input);

    expect(result[0]).not.toHaveProperty('tokenCount');
    expect(result[0]).not.toHaveProperty('lastUpdated');
    expect(result[0]).not.toHaveProperty('errorMessage');
  });

  it('handles all possible entity statuses', () => {
    const statuses: EntityProgressResponse['status'][] = [
      'queued', 'processing', 'complete', 'updating', 'schema-changed', 'outdated', 'failed',
    ];

    const input: EntityProgressResponse[] = statuses.map((status, i) => ({
      entity_name: `table_${i}`,
      status,
    }));

    const result = transformEntityQueue(input);

    expect(result).toHaveLength(statuses.length);
    statuses.forEach((status, i) => {
      expect(result[i]?.status).toBe(status);
    });
  });

  it('transforms a full entity with all optional fields', () => {
    const input: EntityProgressResponse[] = [
      {
        entity_name: 'invoices',
        status: 'processing',
        token_count: 3200,
        last_updated: '2025-06-01T12:00:00Z',
        error_message: 'Rate limit exceeded',
      },
    ];

    const result = transformEntityQueue(input);

    expect(result).toEqual([
      {
        entityName: 'invoices',
        status: 'processing',
        tokenCount: 3200,
        lastUpdated: '2025-06-01T12:00:00Z',
        errorMessage: 'Rate limit exceeded',
      },
    ]);
  });
});

describe('transformTaskQueue', () => {
  it('returns empty array when input is undefined', () => {
    expect(transformTaskQueue(undefined)).toEqual([]);
  });

  it('returns empty array when input is an empty array', () => {
    expect(transformTaskQueue([])).toEqual([]);
  });

  it('maps required fields to camelCase WorkQueueTaskItem fields', () => {
    const input: TaskProgressResponse[] = [
      { id: 'task-1', name: 'Analyze schema', status: 'processing', requires_llm: true },
      { id: 'task-2', name: 'Profile data', status: 'queued', requires_llm: false },
    ];

    const result = transformTaskQueue(input);

    expect(result).toEqual([
      { id: 'task-1', name: 'Analyze schema', status: 'processing', requiresLlm: true },
      { id: 'task-2', name: 'Profile data', status: 'queued', requiresLlm: false },
    ]);
  });

  it('includes optional fields only when present in input', () => {
    const input: TaskProgressResponse[] = [
      {
        id: 'task-3',
        name: 'Generate ontology',
        status: 'complete',
        requires_llm: true,
        started_at: '2025-06-01T10:00:00Z',
        completed_at: '2025-06-01T10:05:00Z',
      },
    ];

    const result = transformTaskQueue(input);

    expect(result).toEqual([
      {
        id: 'task-3',
        name: 'Generate ontology',
        status: 'complete',
        requiresLlm: true,
        startedAt: '2025-06-01T10:00:00Z',
        completedAt: '2025-06-01T10:05:00Z',
      },
    ]);
  });

  it('includes error_message as errorMessage when present', () => {
    const input: TaskProgressResponse[] = [
      {
        id: 'task-4',
        name: 'Failing task',
        status: 'failed',
        requires_llm: false,
        error_message: 'Timeout exceeded',
      },
    ];

    const result = transformTaskQueue(input);

    expect(result).toHaveLength(1);
    expect(result[0]).toEqual({
      id: 'task-4',
      name: 'Failing task',
      status: 'failed',
      requiresLlm: false,
      errorMessage: 'Timeout exceeded',
    });
  });

  it('omits optional fields when they are undefined in input', () => {
    const input: TaskProgressResponse[] = [
      { id: 'task-5', name: 'Basic task', status: 'queued', requires_llm: false },
    ];

    const result = transformTaskQueue(input);

    expect(result[0]).not.toHaveProperty('startedAt');
    expect(result[0]).not.toHaveProperty('completedAt');
    expect(result[0]).not.toHaveProperty('errorMessage');
  });

  it('handles all possible task statuses', () => {
    const statuses: TaskProgressResponse['status'][] = [
      'queued', 'processing', 'complete', 'failed', 'paused',
    ];

    const input: TaskProgressResponse[] = statuses.map((status, i) => ({
      id: `task-${i}`,
      name: `Task ${i}`,
      status,
      requires_llm: i % 2 === 0,
    }));

    const result = transformTaskQueue(input);

    expect(result).toHaveLength(statuses.length);
    statuses.forEach((status, i) => {
      expect(result[i]?.status).toBe(status);
    });
  });

  it('transforms a full task with all optional fields', () => {
    const input: TaskProgressResponse[] = [
      {
        id: 'task-full',
        name: 'Complete task',
        status: 'failed',
        requires_llm: true,
        started_at: '2025-06-01T12:00:00Z',
        completed_at: '2025-06-01T12:30:00Z',
        error_message: 'LLM rate limit',
      },
    ];

    const result = transformTaskQueue(input);

    expect(result).toEqual([
      {
        id: 'task-full',
        name: 'Complete task',
        status: 'failed',
        requiresLlm: true,
        startedAt: '2025-06-01T12:00:00Z',
        completedAt: '2025-06-01T12:30:00Z',
        errorMessage: 'LLM rate limit',
      },
    ]);
  });
});

describe('transformQuestions', () => {
  it('returns empty array when input is an empty array', () => {
    expect(transformQuestions([])).toEqual([]);
  });

  it('maps id, text, and affects from WorkflowQuestion to ExtractionQuestion', () => {
    const input: WorkflowQuestion[] = [
      {
        id: 'q-1',
        text: 'What does the users table represent?',
        context: 'Schema analysis',
        category: 'general',
        priority: 1,
        affects: ['users', 'orders'],
      },
    ];

    const result = transformQuestions(input);

    expect(result).toEqual([
      {
        id: 'q-1',
        text: 'What does the users table represent?',
        affects: ['users', 'orders'],
        isSubmitted: false,
      },
    ]);
  });

  it('defaults affects to empty array when not present in input', () => {
    const input: WorkflowQuestion[] = [
      {
        id: 'q-2',
        text: 'Describe the business domain',
        context: '',
        category: 'domain',
        priority: 2,
        // affects is undefined
      },
    ];

    const result = transformQuestions(input);

    expect(result[0]?.affects).toEqual([]);
  });

  it('sets isSubmitted to true when answered_at is a non-null string', () => {
    const input: WorkflowQuestion[] = [
      {
        id: 'q-3',
        text: 'Already answered question',
        context: '',
        category: 'general',
        priority: 1,
        answered_at: '2025-06-01T12:00:00Z',
      },
    ];

    const result = transformQuestions(input);

    expect(result[0]?.isSubmitted).toBe(true);
  });

  it('sets isSubmitted to false when answered_at is undefined', () => {
    const input: WorkflowQuestion[] = [
      {
        id: 'q-4',
        text: 'Unanswered question',
        context: '',
        category: 'general',
        priority: 1,
        // answered_at is undefined
      },
    ];

    const result = transformQuestions(input);

    expect(result[0]?.isSubmitted).toBe(false);
  });

  it('does not populate answer from backend data', () => {
    const input: WorkflowQuestion[] = [
      {
        id: 'q-5',
        text: 'Question with suggested answer',
        context: '',
        category: 'general',
        priority: 1,
        suggested_answer: 'This should not appear',
        answered_at: '2025-06-01T12:00:00Z',
      },
    ];

    const result = transformQuestions(input);

    expect(result[0]).not.toHaveProperty('answer');
  });

  it('transforms multiple questions preserving order', () => {
    const input: WorkflowQuestion[] = [
      { id: 'q-a', text: 'First', context: '', category: 'general', priority: 1 },
      { id: 'q-b', text: 'Second', context: '', category: 'domain', priority: 2, answered_at: '2025-01-01T00:00:00Z' },
      { id: 'q-c', text: 'Third', context: '', category: 'schema', priority: 3, affects: ['products'] },
    ];

    const result = transformQuestions(input);

    expect(result).toHaveLength(3);
    expect(result[0]?.id).toBe('q-a');
    expect(result[0]?.isSubmitted).toBe(false);
    expect(result[1]?.id).toBe('q-b');
    expect(result[1]?.isSubmitted).toBe(true);
    expect(result[2]?.id).toBe('q-c');
    expect(result[2]?.affects).toEqual(['products']);
  });

  it('only includes id, text, affects, and isSubmitted in output', () => {
    const input: WorkflowQuestion[] = [
      {
        id: 'q-6',
        text: 'Full question',
        context: 'Lots of context',
        category: 'schema',
        priority: 5,
        options: ['opt1', 'opt2'],
        suggested_answer: 'suggestion',
        reasoning: 'some reasoning',
        affects: ['table1'],
        created_at: '2025-01-01T00:00:00Z',
      },
    ];

    const result = transformQuestions(input);

    // Only these keys should be present
    expect(Object.keys(result[0]!)).toEqual(['id', 'text', 'affects', 'isSubmitted']);
  });
});

describe('OntologyService polling (startPolling / stopPolling)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.spyOn(console, 'debug').mockImplementation(() => {});
    // Reset service state
    ontologyService.stop();
    ontologyService.setProjectId('proj-1');
  });

  afterEach(() => {
    ontologyService.stop();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('startExtraction triggers immediate status fetch then polls at 2s intervals', async () => {
    const inProgress = makeStatusResponse({ is_complete: false });
    mockOntologyApi.extractOntology.mockResolvedValue({
      workflow_id: 'wf-1',
      status: 'started',
    });
    mockOntologyApi.getStatus.mockResolvedValue(inProgress);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-1',
      questions: { critical: [], high: [], medium: [], low: [] },
    });

    await ontologyService.startExtraction();

    // startPolling calls fetchAndUpdateStatus immediately
    // Flush the immediate fetch
    await vi.advanceTimersByTimeAsync(0);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(1);

    // Advance 2000ms — second poll
    await vi.advanceTimersByTimeAsync(2000);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(2);

    // Advance another 2000ms — third poll
    await vi.advanceTimersByTimeAsync(2000);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(3);
  });

  it('emits status updates to subscribers during polling', async () => {
    const inProgress = makeStatusResponse({ is_complete: false, current_entity: 2, total_entities: 5 });
    mockOntologyApi.extractOntology.mockResolvedValue({
      workflow_id: 'wf-1',
      status: 'started',
    });
    mockOntologyApi.getStatus.mockResolvedValue(inProgress);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-1',
      questions: { critical: [], high: [], medium: [], low: [] },
    });

    const callback = vi.fn();
    const unsubscribe = ontologyService.subscribe(callback);

    // subscribe emits current status immediately
    expect(callback).toHaveBeenCalledTimes(1);

    await ontologyService.startExtraction();
    // startExtraction emits 'initializing' status
    expect(callback).toHaveBeenCalledTimes(2);

    // Flush immediate fetch from startPolling
    await vi.advanceTimersByTimeAsync(0);
    // fetchAndUpdateStatus emits transformed status
    expect(callback.mock.calls.length).toBeGreaterThanOrEqual(3);

    const lastCall = callback.mock.calls[callback.mock.calls.length - 1][0];
    expect(lastCall.progress.state).toBe('building');

    unsubscribe();
  });

  it('auto-stops polling when workflow is_complete and not awaiting_input', async () => {
    const inProgress = makeStatusResponse({ is_complete: false });
    const complete = makeStatusResponse({ is_complete: true, current_phase: 'done' });

    mockOntologyApi.extractOntology.mockResolvedValue({
      workflow_id: 'wf-1',
      status: 'started',
    });
    // First fetch returns in-progress, second returns complete
    mockOntologyApi.getStatus
      .mockResolvedValueOnce(inProgress)
      .mockResolvedValueOnce(complete);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-1',
      questions: { critical: [], high: [], medium: [], low: [] },
    });

    await ontologyService.startExtraction();

    // Flush immediate fetch (in-progress)
    await vi.advanceTimersByTimeAsync(0);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(1);

    // Advance to next poll interval — returns complete, which triggers stopPolling
    await vi.advanceTimersByTimeAsync(2000);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(2);

    // No further polls should happen even after more time
    await vi.advanceTimersByTimeAsync(10000);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(2);
  });

  it('auto-stops polling on fetch error and sets error state', async () => {
    mockOntologyApi.extractOntology.mockResolvedValue({
      workflow_id: 'wf-1',
      status: 'started',
    });
    mockOntologyApi.getStatus.mockRejectedValue(new Error('Failed to fetch'));

    await ontologyService.startExtraction();

    // Flush immediate fetch — fails
    await vi.advanceTimersByTimeAsync(0);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(1);

    // Verify error state
    const status = ontologyService.getStatus();
    expect(status.progress.state).toBe('error');
    expect(status.lastError).toBe('Service is currently down.');

    // No more polls
    await vi.advanceTimersByTimeAsync(10000);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(1);
  });

  it('stop() clears the polling interval and resets to idle', async () => {
    const inProgress = makeStatusResponse({ is_complete: false });
    mockOntologyApi.extractOntology.mockResolvedValue({
      workflow_id: 'wf-1',
      status: 'started',
    });
    mockOntologyApi.getStatus.mockResolvedValue(inProgress);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-1',
      questions: { critical: [], high: [], medium: [], low: [] },
    });

    await ontologyService.startExtraction();
    await vi.advanceTimersByTimeAsync(0);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(1);

    // Call stop — should clear interval
    ontologyService.stop();

    // No further polls
    await vi.advanceTimersByTimeAsync(10000);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(1);

    // Status is idle after stop
    const status = ontologyService.getStatus();
    expect(status.progress.state).toBe('idle');
  });

  it('initialize starts polling when workflow is active and not complete', async () => {
    const activeStatus = makeStatusResponse({
      workflow_id: 'wf-existing',
      is_complete: false,
    });
    mockOntologyApi.getStatus.mockResolvedValue(activeStatus);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-existing',
      questions: { critical: [], high: [], medium: [], low: [] },
    });

    await ontologyService.initialize('proj-1');

    // initialize calls getStatus once to check, then startPolling fetches again immediately
    await vi.advanceTimersByTimeAsync(0);

    // Should be polling — advance and check for additional calls
    const callCountAfterInit = mockOntologyApi.getStatus.mock.calls.length;
    await vi.advanceTimersByTimeAsync(2000);
    expect(mockOntologyApi.getStatus.mock.calls.length).toBeGreaterThan(callCountAfterInit);
  });

  it('initialize does not start polling when workflow is complete', async () => {
    const completeStatus = makeStatusResponse({
      workflow_id: 'wf-done',
      is_complete: true,
    });
    mockOntologyApi.getStatus.mockResolvedValue(completeStatus);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-done',
      questions: { critical: [], high: [], medium: [], low: [] },
    });

    await ontologyService.initialize('proj-1');

    // Flush any microtasks
    await vi.advanceTimersByTimeAsync(0);

    const callCountAfterInit = mockOntologyApi.getStatus.mock.calls.length;

    // Should NOT be polling — no further calls after waiting
    await vi.advanceTimersByTimeAsync(10000);
    expect(mockOntologyApi.getStatus.mock.calls.length).toBe(callCountAfterInit);
  });

  it('cancel() stops polling and resets to idle', async () => {
    const inProgress = makeStatusResponse({ is_complete: false });
    mockOntologyApi.extractOntology.mockResolvedValue({
      workflow_id: 'wf-1',
      status: 'started',
    });
    mockOntologyApi.getStatus.mockResolvedValue(inProgress);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-1',
      questions: { critical: [], high: [], medium: [], low: [] },
    });
    mockOntologyApi.cancelWorkflow.mockResolvedValue({
      workflow_id: 'wf-1',
      status: 'cancelled',
      message: 'Workflow cancelled',
    });

    await ontologyService.startExtraction();
    await vi.advanceTimersByTimeAsync(0);

    // Cancel should stop polling
    await ontologyService.cancel();

    const callCountAfterCancel = mockOntologyApi.getStatus.mock.calls.length;
    await vi.advanceTimersByTimeAsync(10000);
    expect(mockOntologyApi.getStatus.mock.calls.length).toBe(callCountAfterCancel);

    // Status is idle
    expect(ontologyService.getStatus().progress.state).toBe('idle');
  });

  it('starting new polling clears the existing interval (no double-polling)', async () => {
    const inProgress = makeStatusResponse({ is_complete: false });
    mockOntologyApi.extractOntology.mockResolvedValue({
      workflow_id: 'wf-1',
      status: 'started',
    });
    mockOntologyApi.getStatus.mockResolvedValue(inProgress);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-1',
      questions: { critical: [], high: [], medium: [], low: [] },
    });
    mockOntologyApi.submitProjectAnswers.mockResolvedValue({
      workflow_id: 'wf-1',
      status: 'processing',
    });

    await ontologyService.startExtraction();
    await vi.advanceTimersByTimeAsync(0);

    // Set up a question so submitAnswer can work
    const withQuestions = makeStatusResponse({
      is_complete: false,
      current_phase: 'awaiting_input',
      pending_questions_count: 1,
    });
    mockOntologyApi.getStatus.mockResolvedValue(withQuestions);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-1',
      questions: {
        critical: [{ id: 'q1', text: 'What?', context: '', category: 'general', priority: 1 }],
        high: [],
        medium: [],
        low: [],
      },
    });

    // Fetch to pick up the question
    await vi.advanceTimersByTimeAsync(2000);

    // submitAnswer triggers startPolling again — should not double the interval
    await ontologyService.submitAnswer('q1', 'test answer');
    await vi.advanceTimersByTimeAsync(0);

    // Reset mock to count fresh
    mockOntologyApi.getStatus.mockClear();
    mockOntologyApi.getStatus.mockResolvedValue(inProgress);
    mockOntologyApi.getQuestions.mockResolvedValue({
      workflow_id: 'wf-1',
      questions: { critical: [], high: [], medium: [], low: [] },
    });

    // Over 2000ms, should get exactly 1 poll (not 2 from double intervals)
    await vi.advanceTimersByTimeAsync(2000);
    expect(mockOntologyApi.getStatus).toHaveBeenCalledTimes(1);
  });
});
