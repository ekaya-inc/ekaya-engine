/**
 * OntologyStatus Component
 * Displays the extraction workflow status bar with progress, stats, and controls
 */

import {
  CheckCircle,
  Circle,
  Clock,
  Loader2,
  Pause,
  Play,
  RefreshCw,
  Zap,
} from 'lucide-react';

import type { WorkflowProgress } from '../../types';
import { Button } from '../ui/Button';

interface OntologyStatusProps {
  progress: WorkflowProgress;
  onPause: () => void;
  onResume: () => void;
  onRestart: () => void;
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
  onPause,
  onResume,
  onRestart,
}: OntologyStatusProps) => {
  const { state, current, total, tokensPerSecond, timeRemainingMs } = progress;
  const progressPercent = total > 0 ? Math.round((current / total) * 100) : 0;

  const getStatusIcon = () => {
    switch (state) {
      case 'complete':
        return <CheckCircle className="h-5 w-5 text-green-500" />;
      case 'building':
        return <Loader2 className="h-5 w-5 text-blue-500 animate-spin" />;
      case 'paused':
        return <Pause className="h-5 w-5 text-amber-500" />;
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
      case 'paused':
        return 'Paused';
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
      case 'paused':
        return 'text-amber-600';
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
            </div>
            {state !== 'idle' && (
              <div className="text-sm text-text-secondary">
                {current}/{total} entities
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
                    : state === 'paused'
                    ? 'bg-amber-500'
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
        {(state === 'building' || state === 'paused') && (
          <div className="flex items-center gap-4 text-sm text-text-secondary">
            {tokensPerSecond !== undefined && tokensPerSecond > 0 && (
              <div className="flex items-center gap-1">
                <Zap className="h-4 w-4 text-amber-500" />
                <span>{tokensPerSecond} tok/s</span>
              </div>
            )}
            {timeRemainingMs !== undefined && state === 'building' && (
              <div className="flex items-center gap-1">
                <Clock className="h-4 w-4 text-blue-500" />
                <span>{formatTimeRemaining(timeRemainingMs)} left</span>
              </div>
            )}
          </div>
        )}

        {/* Controls */}
        <div className="flex items-center gap-2">
          {state === 'building' && (
            <Button
              variant="outline"
              size="sm"
              onClick={onPause}
            >
              <Pause className="h-4 w-4 mr-1" />
              Pause
            </Button>
          )}
          {state === 'paused' && (
            <Button
              variant="outline"
              size="sm"
              onClick={onResume}
            >
              <Play className="h-4 w-4 mr-1" />
              Resume
            </Button>
          )}
          {(state === 'complete' || state === 'paused') && (
            <Button
              variant="outline"
              size="sm"
              onClick={onRestart}
            >
              <RefreshCw className="h-4 w-4 mr-1" />
              Restart
            </Button>
          )}
        </div>
      </div>
    </div>
  );
};

export default OntologyStatus;
