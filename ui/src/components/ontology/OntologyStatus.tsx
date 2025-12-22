/**
 * OntologyStatus Component
 * Displays the extraction workflow status bar with progress, stats, and controls
 */

import {
  CheckCircle,
  Circle,
  Clock,
  HelpCircle,
  Loader2,
  Trash2,
  X,
  Zap,
} from 'lucide-react';

import type { WorkflowProgress } from '../../types';
import { Button } from '../ui/Button';

interface OntologyStatusProps {
  progress: WorkflowProgress;
  pendingQuestionCount?: number;
  onCancel: () => void;
  onRefresh?: () => void; // Optional - refresh button currently hidden
  onDelete: () => void;
}

const formatTimeRemaining = (ms: number | undefined): string => {
  if (!ms || ms <= 0) return '--';

  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `~${seconds}s`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `~${minutes} min`;

  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return `~${hours}h ${remainingMinutes}m`;
};

const OntologyStatus = ({
  progress,
  pendingQuestionCount = 0,
  onCancel,
  onDelete,
}: OntologyStatusProps) => {
  const { state, current, total, tokensPerSecond, timeRemainingMs } = progress;
  const progressPercent = total > 0 ? Math.round((current / total) * 100) : 0;

  const getStatusIcon = () => {
    switch (state) {
      case 'complete':
        return <CheckCircle className="h-5 w-5 text-green-500" />;
      case 'building':
        return <Loader2 className="h-5 w-5 text-blue-500 animate-spin" />;
      case 'awaiting_input':
        return <HelpCircle className="h-5 w-5 text-purple-500 animate-pulse" />;
      case 'initializing':
        return <Circle className="h-5 w-5 text-blue-400 animate-pulse" />;
      default:
        return <Circle className="h-5 w-5 text-text-tertiary" />;
    }
  };

  const getStatusText = () => {
    switch (state) {
      case 'complete':
        return 'Complete';
      case 'building':
        return 'Building';
      case 'awaiting_input':
        return 'Questions Ready';
      case 'initializing':
        return 'Initializing';
      default:
        return 'Idle';
    }
  };

  const getStatusColor = () => {
    switch (state) {
      case 'complete':
        return 'text-green-600';
      case 'building':
        return 'text-blue-600';
      case 'awaiting_input':
        return 'text-purple-600';
      case 'initializing':
        return 'text-blue-400';
      default:
        return 'text-text-tertiary';
    }
  };

  return (
    <div className="rounded-lg border border-border-light bg-surface-primary p-4 shadow-sm">
      <div className="flex items-center justify-between gap-4">
        {/* Status indicator */}
        <div className="flex items-center gap-3">
          {getStatusIcon()}
          <div>
            <div className={`font-semibold ${getStatusColor()}`}>
              {getStatusText()}
              {state === 'awaiting_input' && pendingQuestionCount > 0 && (
                <span className="ml-2 inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-purple-100 text-purple-800">
                  {pendingQuestionCount} {pendingQuestionCount === 1 ? 'question' : 'questions'}
                </span>
              )}
            </div>
            {state !== 'idle' && (
              <div className="text-sm text-text-secondary">
                {current}/{total} entities analyzed
              </div>
            )}
          </div>
        </div>

        {/* Progress bar */}
        {state !== 'idle' && (
          <div className="flex-1 max-w-md">
            <div className="h-2 rounded-full bg-surface-tertiary overflow-hidden">
              <div
                className={`h-full rounded-full transition-all duration-300 ${
                  state === 'complete'
                    ? 'bg-green-500'
                    : state === 'awaiting_input'
                    ? 'bg-purple-500'
                    : 'bg-blue-500'
                }`}
                style={{ width: `${progressPercent}%` }}
              />
            </div>
            <div className="mt-1 text-xs text-text-tertiary text-center">
              {progressPercent}%
            </div>
          </div>
        )}

        {/* Performance stats */}
        {state === 'building' && (
          <div className="flex items-center gap-4 text-sm text-text-secondary">
            {tokensPerSecond !== undefined && tokensPerSecond > 0 && (
              <div className="flex items-center gap-1">
                <Zap className="h-4 w-4 text-amber-500" />
                <span>{tokensPerSecond} tok/s</span>
              </div>
            )}
            {timeRemainingMs !== undefined && (
              <div className="flex items-center gap-1">
                <Clock className="h-4 w-4 text-blue-500" />
                <span>{formatTimeRemaining(timeRemainingMs)} left</span>
              </div>
            )}
          </div>
        )}

        {/* Controls */}
        <div className="flex items-center gap-2">
          {(state === 'building' || state === 'initializing') && (
            <Button
              variant="outline"
              size="sm"
              onClick={onCancel}
              className="text-red-600 hover:text-red-700 hover:bg-red-50"
            >
              <X className="h-4 w-4 mr-1" />
              Cancel
            </Button>
          )}
          {(state === 'complete' || state === 'awaiting_input') && (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={onDelete}
                className="text-red-600 hover:text-red-700 hover:bg-red-50"
              >
                <Trash2 className="h-4 w-4 mr-1" />
                Delete
              </Button>
              {/* TODO: Refresh button hidden until we implement incremental refresh
                  that can diff existing ontology vs schema and only process changes.
                  Workaround: Delete and rebuild from scratch. */}
            </>
          )}
        </div>
      </div>
    </div>
  );
};

export default OntologyStatus;
