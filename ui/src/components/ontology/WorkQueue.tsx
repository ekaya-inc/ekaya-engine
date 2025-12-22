/**
 * WorkQueue Component
 * Displays the list of tasks or entities being processed with their status
 * Supports both task-based (NEW) and entity-based (LEGACY) queues
 */

import {
  AlertTriangle,
  Brain,
  Check,
  Circle,
  Loader2,
  Pause,
  RefreshCw,
  Star,
  XCircle,
} from 'lucide-react';
import { useRef, useEffect } from 'react';

import type { EntityStatus, TaskStatus, WorkItem, WorkQueueTaskItem } from '../../types';

interface WorkQueueProps {
  items: WorkItem[];           // LEGACY: entity-based queue
  taskItems?: WorkQueueTaskItem[];  // NEW: task-based queue
  maxHeight?: string;
}

// ============================================================================
// Entity-based helpers (LEGACY)
// ============================================================================

const getEntityStatusIcon = (status: EntityStatus) => {
  switch (status) {
    case 'complete':
      return <Check className="h-4 w-4 text-green-500" />;
    case 'processing':
      return <Loader2 className="h-4 w-4 text-blue-500 animate-spin" />;
    case 'queued':
      return <Circle className="h-4 w-4 text-text-tertiary" />;
    case 'updating':
      return <RefreshCw className="h-4 w-4 text-purple-500 animate-spin" />;
    case 'schema-changed':
      return <Star className="h-4 w-4 text-amber-500" />;
    case 'outdated':
      return <AlertTriangle className="h-4 w-4 text-amber-500" />;
    case 'failed':
      return <XCircle className="h-4 w-4 text-red-500" />;
    default:
      return <Circle className="h-4 w-4 text-text-tertiary" />;
  }
};

const getEntityStatusLabel = (status: EntityStatus): string => {
  switch (status) {
    case 'complete':
      return 'Complete';
    case 'processing':
      return 'Processing';
    case 'queued':
      return 'Queued';
    case 'updating':
      return 'Updating';
    case 'schema-changed':
      return 'Schema Changed';
    case 'outdated':
      return 'Outdated';
    case 'failed':
      return 'Failed';
    default:
      return 'Unknown';
  }
};

const getEntityRowBackground = (status: EntityStatus): string => {
  switch (status) {
    case 'processing':
      return 'bg-blue-500/5 border-l-2 border-l-blue-500';
    case 'updating':
      return 'bg-purple-500/5 border-l-2 border-l-purple-500';
    case 'failed':
      return 'bg-red-500/5 border-l-2 border-l-red-500';
    case 'schema-changed':
    case 'outdated':
      return 'bg-amber-500/5 border-l-2 border-l-amber-500';
    default:
      return 'border-l-2 border-l-transparent';
  }
};

// Sort priority: processing > updating > schema-changed > outdated > queued > complete > failed
const entityStatusPriority: Record<EntityStatus, number> = {
  processing: 0,
  updating: 1,
  'schema-changed': 2,
  outdated: 3,
  queued: 4,
  complete: 5,
  failed: 6,
};

// ============================================================================
// Task-based helpers (NEW)
// ============================================================================

const getTaskStatusIcon = (status: TaskStatus, requiresLlm: boolean) => {
  switch (status) {
    case 'complete':
      return <Check className="h-4 w-4 text-green-500" />;
    case 'processing':
      // Use Brain icon for LLM tasks, regular spinner for others
      return requiresLlm ? (
        <Brain className="h-4 w-4 text-purple-500 animate-pulse" />
      ) : (
        <Loader2 className="h-4 w-4 text-blue-500 animate-spin" />
      );
    case 'queued':
      return <Circle className="h-4 w-4 text-text-tertiary" />;
    case 'paused':
      return <Pause className="h-4 w-4 text-amber-500" />;
    case 'failed':
      return <XCircle className="h-4 w-4 text-red-500" />;
    default:
      return <Circle className="h-4 w-4 text-text-tertiary" />;
  }
};

const getTaskStatusLabel = (status: TaskStatus): string => {
  switch (status) {
    case 'complete':
      return 'Complete';
    case 'processing':
      return 'Processing';
    case 'queued':
      return 'Queued';
    case 'paused':
      return 'Paused';
    case 'failed':
      return 'Failed';
    default:
      return 'Unknown';
  }
};

const getTaskRowBackground = (status: TaskStatus): string => {
  switch (status) {
    case 'processing':
      return 'bg-blue-500/5 border-l-2 border-l-blue-500';
    case 'paused':
      return 'bg-amber-500/5 border-l-2 border-l-amber-500';
    case 'failed':
      return 'bg-red-500/5 border-l-2 border-l-red-500';
    default:
      return 'border-l-2 border-l-transparent';
  }
};

// Sort priority: processing > paused > queued > complete > failed
const taskStatusPriority: Record<TaskStatus, number> = {
  processing: 0,
  paused: 1,
  queued: 2,
  complete: 3,
  failed: 4,
};

// ============================================================================
// Component
// ============================================================================

const WorkQueue = ({ items, taskItems, maxHeight = '400px' }: WorkQueueProps) => {
  const processingRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Use task queue if available, otherwise fall back to entity queue
  const useTaskQueue = taskItems && taskItems.length > 0;

  // Sort items by status priority and filter out completed tasks from display
  // (completed tasks still count in summary but don't clutter the list)
  const sortedEntityItems = [...items]
    .filter((item) => item.status !== 'complete')
    .sort((a, b) => {
      return entityStatusPriority[a.status] - entityStatusPriority[b.status];
    });

  const sortedTaskItems = useTaskQueue
    ? [...taskItems]
        .filter((task) => task.status !== 'complete')
        .sort((a, b) => {
          return taskStatusPriority[a.status] - taskStatusPriority[b.status];
        })
    : [];

  // Group items by status for summary
  const entityStatusCounts = items.reduce(
    (acc, item) => {
      acc[item.status] = (acc[item.status] || 0) + 1;
      return acc;
    },
    {} as Record<EntityStatus, number>
  );

  const taskStatusCounts = useTaskQueue
    ? taskItems.reduce(
        (acc, item) => {
          acc[item.status] = (acc[item.status] || 0) + 1;
          return acc;
        },
        {} as Record<TaskStatus, number>
      )
    : ({} as Record<TaskStatus, number>);

  // Get the currently processing item name for auto-scroll
  const processingEntityName = items.find((i) => i.status === 'processing')?.entityName;
  const processingTaskId = useTaskQueue
    ? taskItems.find((t) => t.status === 'processing')?.id
    : undefined;

  // Auto-scroll to processing item
  useEffect(() => {
    if (processingRef.current && containerRef.current) {
      processingRef.current.scrollIntoView({
        behavior: 'smooth',
        block: 'center',
      });
    }
  }, [processingEntityName, processingTaskId]);

  // Render task-based queue (NEW)
  const renderTaskQueue = () => (
    <>
      {/* Header with summary */}
      <div className="p-4 border-b border-border-light">
        <h3 className="font-semibold text-text-primary">Work Queue</h3>
        <div className="mt-2 flex flex-wrap gap-3 text-sm">
          {taskStatusCounts.complete && taskStatusCounts.complete > 0 && (
            <span className="flex items-center gap-1 text-green-600">
              <Check className="h-3 w-3" />
              {taskStatusCounts.complete} complete
            </span>
          )}
          {taskStatusCounts.processing && taskStatusCounts.processing > 0 && (
            <span className="flex items-center gap-1 text-blue-600">
              <Loader2 className="h-3 w-3 animate-spin" />
              {taskStatusCounts.processing} processing
            </span>
          )}
          {taskStatusCounts.queued && taskStatusCounts.queued > 0 && (
            <span className="flex items-center gap-1 text-text-secondary">
              <Circle className="h-3 w-3" />
              {taskStatusCounts.queued} queued
            </span>
          )}
          {taskStatusCounts.failed && taskStatusCounts.failed > 0 && (
            <span className="flex items-center gap-1 text-red-600">
              <XCircle className="h-3 w-3" />
              {taskStatusCounts.failed} failed
            </span>
          )}
        </div>
      </div>

      {/* Scrollable list */}
      <div ref={containerRef} className="overflow-y-auto" style={{ maxHeight }}>
        {sortedTaskItems.length === 0 ? (
          <div className="p-8 text-center text-text-secondary">No tasks in queue</div>
        ) : (
          <div className="divide-y divide-border-light">
            {sortedTaskItems.map((task) => (
              <div
                key={task.id}
                ref={task.status === 'processing' ? processingRef : undefined}
                className={`flex items-center justify-between px-4 py-3 ${getTaskRowBackground(task.status)}`}
              >
                <div className="flex items-center gap-3">
                  {getTaskStatusIcon(task.status, task.requiresLlm)}
                  <div>
                    <span className="text-sm text-text-primary">{task.name}</span>
                    {task.requiresLlm && task.status === 'processing' && (
                      <p className="text-xs text-purple-500 mt-0.5">Using AI...</p>
                    )}
                    {task.errorMessage && (
                      <p className="text-xs text-red-500 mt-0.5">{task.errorMessage}</p>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  {task.requiresLlm && (
                    <Brain
                      className="h-3 w-3 text-purple-400"
                      aria-label="Requires AI processing"
                    />
                  )}
                  <span
                    className={`text-xs px-2 py-0.5 rounded-full ${
                      task.status === 'complete'
                        ? 'bg-green-100 text-green-700'
                        : task.status === 'processing'
                          ? 'bg-blue-100 text-blue-700'
                          : task.status === 'paused'
                            ? 'bg-amber-100 text-amber-700'
                            : task.status === 'failed'
                              ? 'bg-red-100 text-red-700'
                              : 'bg-surface-tertiary text-text-secondary'
                    }`}
                  >
                    {getTaskStatusLabel(task.status)}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );

  // Render entity-based queue (LEGACY)
  const renderEntityQueue = () => (
    <>
      {/* Header with summary */}
      <div className="p-4 border-b border-border-light">
        <h3 className="font-semibold text-text-primary">Work Queue</h3>
        <div className="mt-2 flex flex-wrap gap-3 text-sm">
          {entityStatusCounts.complete && entityStatusCounts.complete > 0 && (
            <span className="flex items-center gap-1 text-green-600">
              <Check className="h-3 w-3" />
              {entityStatusCounts.complete} complete
            </span>
          )}
          {entityStatusCounts.processing && entityStatusCounts.processing > 0 && (
            <span className="flex items-center gap-1 text-blue-600">
              <Loader2 className="h-3 w-3 animate-spin" />
              {entityStatusCounts.processing} processing
            </span>
          )}
          {entityStatusCounts.queued && entityStatusCounts.queued > 0 && (
            <span className="flex items-center gap-1 text-text-secondary">
              <Circle className="h-3 w-3" />
              {entityStatusCounts.queued} queued
            </span>
          )}
          {entityStatusCounts.updating && entityStatusCounts.updating > 0 && (
            <span className="flex items-center gap-1 text-purple-600">
              <RefreshCw className="h-3 w-3 animate-spin" />
              {entityStatusCounts.updating} updating
            </span>
          )}
          {entityStatusCounts.failed && entityStatusCounts.failed > 0 && (
            <span className="flex items-center gap-1 text-red-600">
              <XCircle className="h-3 w-3" />
              {entityStatusCounts.failed} failed
            </span>
          )}
        </div>
      </div>

      {/* Scrollable list */}
      <div ref={containerRef} className="overflow-y-auto" style={{ maxHeight }}>
        {sortedEntityItems.length === 0 ? (
          <div className="p-8 text-center text-text-secondary">No entities in queue</div>
        ) : (
          <div className="divide-y divide-border-light">
            {sortedEntityItems.map((item) => (
              <div
                key={item.entityName}
                ref={item.status === 'processing' ? processingRef : undefined}
                className={`flex items-center justify-between px-4 py-3 ${getEntityRowBackground(item.status)}`}
              >
                <div className="flex items-center gap-3">
                  {getEntityStatusIcon(item.status)}
                  <div>
                    <span className="font-mono text-sm text-text-primary">
                      {item.entityName}
                    </span>
                    {item.errorMessage && (
                      <p className="text-xs text-red-500 mt-0.5">{item.errorMessage}</p>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  {item.tokenCount !== undefined && item.status === 'processing' && (
                    <span className="text-xs text-text-tertiary font-mono">
                      {item.tokenCount} tokens
                    </span>
                  )}
                  <span
                    className={`text-xs px-2 py-0.5 rounded-full ${
                      item.status === 'complete'
                        ? 'bg-green-100 text-green-700'
                        : item.status === 'processing'
                          ? 'bg-blue-100 text-blue-700'
                          : item.status === 'updating'
                            ? 'bg-purple-100 text-purple-700'
                            : item.status === 'failed'
                              ? 'bg-red-100 text-red-700'
                              : item.status === 'schema-changed' || item.status === 'outdated'
                                ? 'bg-amber-100 text-amber-700'
                                : 'bg-surface-tertiary text-text-secondary'
                    }`}
                  >
                    {getEntityStatusLabel(item.status)}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );

  return (
    <div className="rounded-lg border border-border-light bg-surface-primary shadow-sm flex flex-col" style={{ minHeight: maxHeight }}>
      {useTaskQueue ? renderTaskQueue() : renderEntityQueue()}
    </div>
  );
};

export default WorkQueue;
