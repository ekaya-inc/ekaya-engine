/**
 * ExtractionProgress Component
 * Displays multi-phase progress for extraction workflows like Column Feature Extraction.
 * Shows each phase with status indicator, progress counter, and currently processing item.
 *
 * Visual indicators:
 * - Checkmark (green): complete
 * - Filled circle (blue, animated): in progress
 * - Empty circle (gray): pending
 * - X (red): failed
 */

import { Check, Circle, Loader2, X } from 'lucide-react';

import type { DAGNodeProgress, ExtractionPhase, ExtractionPhaseStatus } from '../../types';

/**
 * Default phases for Column Feature Extraction workflow.
 * These match the phases in pkg/services/column_feature_extraction.go
 */
const COLUMN_FEATURE_EXTRACTION_PHASES: readonly ExtractionPhase[] = [
  { id: 'phase1', name: 'Collecting column metadata', status: 'pending' },
  { id: 'phase2', name: 'Classifying columns', status: 'pending' },
  { id: 'phase3', name: 'Analyzing enum values', status: 'pending' },
  { id: 'phase4', name: 'Resolving foreign key targets', status: 'pending' },
  { id: 'phase5', name: 'Analyzing column relationships', status: 'pending' },
  { id: 'phase6', name: 'Saving results', status: 'pending' },
] as const;

/**
 * Maps progress message patterns to phase IDs for ColumnFeatureExtraction.
 * Order matters - first match wins.
 */
const COLUMN_FEATURE_MESSAGE_TO_PHASE: readonly [RegExp, string][] = [
  [/collecting.*column.*metadata|found.*columns.*in.*tables/i, 'phase1'],
  [/classifying.*columns/i, 'phase2'],
  [/analyzing.*enum.*values|labeled.*enum/i, 'phase3'],
  [/resolving.*(fk|foreign\s*key)|resolved.*(fk|foreign\s*key)/i, 'phase4'],
  [/analyzing.*column.*relationships|cross.?column|monetary.*pair|soft.*delete/i, 'phase5'],
  [/saving|complete/i, 'phase6'],
];

/**
 * Default phases for PK Match Discovery workflow (relationship discovery).
 * These match the phases in pkg/services/relationship_discovery_service.go
 */
const PK_MATCH_DISCOVERY_PHASES: readonly ExtractionPhase[] = [
  { id: 'phase1', name: 'Preserving datasource foreign keys', status: 'pending' },
  { id: 'phase2', name: 'Processing pre-resolved foreign keys', status: 'pending' },
  { id: 'phase3', name: 'Collecting relationship candidates', status: 'pending' },
  { id: 'phase4', name: 'Validating relationships', status: 'pending' },
  { id: 'phase5', name: 'Storing results', status: 'pending' },
] as const;

/**
 * Maps progress message patterns to phase IDs for PKMatchDiscovery.
 * Order matters - first match wins.
 */
const PK_MATCH_MESSAGE_TO_PHASE: readonly [RegExp, string][] = [
  [/preserving.*db.*fk/i, 'phase1'],
  [/processing.*columnfeatures|columnfeatures.*fk/i, 'phase2'],
  [/collecting.*candidates|loading.*schema|found.*potential.*fk|fk.*targets|generated.*candidate|analyzing.*candidates/i, 'phase3'],
  [/validating.*relationships/i, 'phase4'],
  [/storing.*results|discovery.*complete/i, 'phase5'],
];

/** Supported node types for multi-phase progress display */
export type ProgressNodeType = 'ColumnFeatureExtraction' | 'PKMatchDiscovery';

/** Get phases config for a node type */
const getPhasesForNode = (nodeType: ProgressNodeType): readonly ExtractionPhase[] => {
  switch (nodeType) {
    case 'PKMatchDiscovery':
      return PK_MATCH_DISCOVERY_PHASES;
    case 'ColumnFeatureExtraction':
    default:
      return COLUMN_FEATURE_EXTRACTION_PHASES;
  }
};

/** Get message-to-phase mapping for a node type */
const getMessageToPhaseMappingForNode = (
  nodeType: ProgressNodeType
): readonly [RegExp, string][] => {
  switch (nodeType) {
    case 'PKMatchDiscovery':
      return PK_MATCH_MESSAGE_TO_PHASE;
    case 'ColumnFeatureExtraction':
    default:
      return COLUMN_FEATURE_MESSAGE_TO_PHASE;
  }
};

interface ExtractionProgressProps {
  /** Progress data from the DAG node */
  progress: DAGNodeProgress | undefined;
  /** Node status (running, completed, failed, etc.) */
  nodeStatus: string | undefined;
  /** Node type to determine which phases to show */
  nodeType?: ProgressNodeType;
  /** Optional class name for styling */
  className?: string;
}

/**
 * Get the status icon for a phase
 */
const getPhaseIcon = (status: ExtractionPhaseStatus) => {
  switch (status) {
    case 'complete':
      return <Check className="h-4 w-4 text-green-500" aria-label="Complete" />;
    case 'in_progress':
      return (
        <Loader2
          className="h-4 w-4 text-blue-500 animate-spin"
          aria-label="In progress"
        />
      );
    case 'failed':
      return <X className="h-4 w-4 text-red-500" aria-label="Failed" />;
    case 'pending':
    default:
      return <Circle className="h-4 w-4 text-gray-300" aria-label="Pending" />;
  }
};

/**
 * Get the text color class for a phase status
 */
const getStatusTextClass = (status: ExtractionPhaseStatus): string => {
  switch (status) {
    case 'complete':
      return 'text-green-700 dark:text-green-400';
    case 'in_progress':
      return 'text-blue-700 dark:text-blue-400';
    case 'failed':
      return 'text-red-700 dark:text-red-400';
    case 'pending':
    default:
      return 'text-gray-500 dark:text-gray-400';
  }
};

/**
 * Get the background class for a phase row
 */
const getPhaseBackground = (status: ExtractionPhaseStatus): string => {
  switch (status) {
    case 'in_progress':
      return 'bg-blue-50/50 dark:bg-blue-900/10';
    case 'failed':
      return 'bg-red-50/50 dark:bg-red-900/10';
    default:
      return '';
  }
};

/**
 * Detect current phase from progress message
 */
const detectPhaseFromMessage = (
  message: string | undefined,
  nodeType: ProgressNodeType
): string | null => {
  if (!message) return null;

  const messageToPhase = getMessageToPhaseMappingForNode(nodeType);
  for (const [pattern, phaseId] of messageToPhase) {
    if (pattern.test(message)) {
      return phaseId;
    }
  }
  return null;
};

/**
 * Build phases array with inferred statuses based on progress
 */
const buildPhasesFromProgress = (
  progress: DAGNodeProgress | undefined,
  nodeStatus: string | undefined,
  nodeType: ProgressNodeType
): ExtractionPhase[] => {
  // Clone the default phases for this node type
  const defaultPhases = getPhasesForNode(nodeType);
  const phases: ExtractionPhase[] = defaultPhases.map((p) => ({ ...p }));

  // If node is not running or has no progress, return pending phases
  if (!progress?.message) {
    // If node completed, mark all phases complete
    if (nodeStatus === 'completed') {
      return phases.map((p) => ({ ...p, status: 'complete' as const }));
    }
    // If node failed, leave phases as pending (we don't know which one failed)
    if (nodeStatus === 'failed') {
      return phases;
    }
    return phases;
  }

  // Detect current phase from message
  const currentPhaseId = detectPhaseFromMessage(progress.message, nodeType);
  const currentPhaseIndex = phases.findIndex((p) => p.id === currentPhaseId);

  // Mark phases based on current position
  for (let i = 0; i < phases.length; i++) {
    const phase = phases[i];
    if (phase === undefined) continue;

    if (i < currentPhaseIndex) {
      // Phases before current are complete
      phase.status = 'complete';
    } else if (i === currentPhaseIndex) {
      // Current phase is in progress
      phase.status = 'in_progress';
      phase.totalItems = progress.total;
      phase.completedItems = progress.current;

      // Add current item description if we have specific progress
      if (progress.current > 0 && progress.total > 0) {
        phase.currentItem = progress.message;
      }
    } else {
      // Phases after current are pending
      phase.status = 'pending';
    }
  }

  // Handle completion - if message indicates complete, mark all done
  if (/complete$/i.test(progress.message ?? '')) {
    return phases.map((p) => ({ ...p, status: 'complete' as const }));
  }

  return phases;
};

/**
 * Format progress as a string (e.g., "23/38" or empty if not meaningful)
 * Returns empty for atomic operations (total <= 1) since showing "0/1" or "1/1" is distracting.
 */
const formatProgress = (phase: ExtractionPhase): string => {
  if (phase.totalItems === undefined || phase.totalItems <= 1) {
    return '';
  }
  return `${phase.completedItems ?? 0}/${phase.totalItems}`;
};

/**
 * Calculate progress percentage
 */
const getProgressPercent = (phase: ExtractionPhase): number => {
  if (!phase.totalItems || phase.totalItems === 0) {
    return 0;
  }
  return Math.round(((phase.completedItems ?? 0) / phase.totalItems) * 100);
};

/**
 * ExtractionProgress displays multi-phase extraction progress with visual indicators.
 *
 * When given DAG node progress, it infers which phases are complete, in progress,
 * or pending based on the progress message patterns.
 */
export const ExtractionProgress = ({
  progress,
  nodeStatus,
  nodeType = 'ColumnFeatureExtraction',
  className = '',
}: ExtractionProgressProps) => {
  const phases = buildPhasesFromProgress(progress, nodeStatus, nodeType);

  return (
    <div className={`space-y-1 ${className}`} role="list" aria-label="Extraction phases">
      {phases.map((phase) => {
        const progressText = formatProgress(phase);
        const progressPercent = getProgressPercent(phase);
        const hasProgress = phase.totalItems !== undefined && phase.totalItems > 1;

        return (
          <div
            key={phase.id}
            className={`flex items-start gap-3 px-3 py-2 rounded-md transition-colors ${getPhaseBackground(phase.status)}`}
            role="listitem"
            aria-current={phase.status === 'in_progress' ? 'step' : undefined}
          >
            {/* Status icon */}
            <div className="flex-shrink-0 mt-0.5">{getPhaseIcon(phase.status)}</div>

            {/* Phase content */}
            <div className="flex-1 min-w-0">
              {/* Phase name and progress counter */}
              <div className="flex items-center justify-between gap-2">
                <span className={`text-sm font-medium ${getStatusTextClass(phase.status)}`}>
                  {phase.name}
                </span>
                {progressText && (
                  <span className="text-xs text-gray-500 dark:text-gray-400 tabular-nums">
                    {progressText}
                  </span>
                )}
              </div>

              {/* Progress bar for in-progress phases */}
              {phase.status === 'in_progress' && hasProgress && (
                <div className="mt-1.5">
                  <div className="h-1 w-full bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                    <div
                      className="h-full bg-blue-500 rounded-full transition-all duration-300 ease-out"
                      style={{ width: `${progressPercent}%` }}
                      role="progressbar"
                      aria-valuenow={phase.completedItems ?? 0}
                      aria-valuemin={0}
                      aria-valuemax={phase.totalItems}
                    />
                  </div>
                </div>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
};

export default ExtractionProgress;
