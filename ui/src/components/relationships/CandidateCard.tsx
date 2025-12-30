/**
 * CandidateCard Component
 * Displays a single relationship candidate with actions
 */

import {
  ArrowRight,
  Brain,
  Check,
  ChevronDown,
  ChevronUp,
  Search,
  Sparkles,
  Trash2,
  Undo2,
  X,
} from 'lucide-react';
import { useState } from 'react';

import type { CandidateResponse, DetectionMethod } from '../../types';
import { Button } from '../ui/Button';

interface CandidateCardProps {
  candidate: CandidateResponse;
  variant: 'confirmed' | 'needs_review' | 'rejected';
  onAccept?: (id: string) => void;
  onReject?: (id: string) => void;
  isLoading?: boolean;
}

const getDetectionMethodIcon = (method: DetectionMethod) => {
  switch (method) {
    case 'value_match':
      return <Search className="h-3 w-3" />;
    case 'name_inference':
      return <Sparkles className="h-3 w-3" />;
    case 'llm':
      return <Brain className="h-3 w-3" />;
    case 'hybrid':
      return <Sparkles className="h-3 w-3" />;
    default:
      return <Search className="h-3 w-3" />;
  }
};

const getDetectionMethodLabel = (method: DetectionMethod): string => {
  switch (method) {
    case 'value_match':
      return 'Value Match';
    case 'name_inference':
      return 'Name Pattern';
    case 'llm':
      return 'LLM Analysis';
    case 'hybrid':
      return 'Multiple Methods';
    default:
      return 'Unknown';
  }
};

const getConfidenceColor = (confidence: number): string => {
  if (confidence >= 0.85) {
    return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400';
  }
  if (confidence >= 0.5) {
    return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400';
  }
  return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400';
};

const getCardStyle = (variant: CandidateCardProps['variant']): string => {
  switch (variant) {
    case 'confirmed':
      return 'border-green-200 bg-green-50/50 dark:border-green-900/50 dark:bg-green-950/20';
    case 'needs_review':
      return 'border-amber-200 bg-amber-50/50 dark:border-amber-900/50 dark:bg-amber-950/20';
    case 'rejected':
      return 'border-gray-200 bg-gray-50/50 dark:border-gray-700 dark:bg-gray-900/20 opacity-60';
    default:
      return 'border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900';
  }
};

export function CandidateCard({
  candidate,
  variant,
  onAccept,
  onReject,
  isLoading = false,
}: CandidateCardProps): React.ReactElement {
  const [showReasoning, setShowReasoning] = useState(false);

  return (
    <div
      className={`border rounded-lg p-4 transition-all ${getCardStyle(variant)}`}
    >
      {/* Main content */}
      <div className="flex justify-between items-start gap-4">
        {/* Left: Relationship info */}
        <div className="flex-1 min-w-0">
          {/* Source -> Target */}
          <div className="flex items-center gap-2 text-sm flex-wrap">
            <span className="font-medium text-text-primary">
              {candidate.source_table}
            </span>
            <span className="text-text-secondary">.</span>
            <span className="font-mono text-text-primary">
              {candidate.source_column}
            </span>
            <ArrowRight className="h-3 w-3 text-text-tertiary flex-shrink-0" />
            <span className="font-medium text-text-primary">
              {candidate.target_table}
            </span>
            <span className="text-text-secondary">.</span>
            <span className="font-mono text-text-primary">
              {candidate.target_column}
            </span>
          </div>

          {/* Badges row */}
          <div className="flex items-center gap-2 mt-2 flex-wrap">
            {/* Confidence badge */}
            <span
              className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium ${getConfidenceColor(candidate.confidence)}`}
            >
              {Math.round(candidate.confidence * 100)}% confidence
            </span>

            {/* Detection method badge */}
            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-surface-secondary text-text-secondary">
              {getDetectionMethodIcon(candidate.detection_method)}
              {getDetectionMethodLabel(candidate.detection_method)}
            </span>

            {/* Cardinality badge (if available) */}
            {candidate.cardinality && (
              <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                {candidate.cardinality}
              </span>
            )}

            {/* Required badge */}
            {candidate.is_required && variant === 'needs_review' && (
              <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">
                Required
              </span>
            )}
          </div>

          {/* LLM Reasoning toggle */}
          {candidate.llm_reasoning && (
            <button
              type="button"
              onClick={() => setShowReasoning(!showReasoning)}
              className="flex items-center gap-1 mt-2 text-xs text-text-secondary hover:text-text-primary transition-colors"
            >
              {showReasoning ? (
                <ChevronUp className="h-3 w-3" />
              ) : (
                <ChevronDown className="h-3 w-3" />
              )}
              {showReasoning ? 'Hide reasoning' : 'Show reasoning'}
            </button>
          )}

          {/* LLM Reasoning content */}
          {showReasoning && candidate.llm_reasoning && (
            <div className="mt-2 p-2 rounded bg-surface-secondary/50 text-xs text-text-secondary italic">
              &ldquo;{candidate.llm_reasoning}&rdquo;
            </div>
          )}
        </div>

        {/* Right: Action buttons */}
        <div className="flex items-center gap-2 flex-shrink-0">
          {variant === 'confirmed' && onReject && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => onReject(candidate.id)}
              disabled={isLoading}
              className="text-gray-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/20"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          )}

          {variant === 'needs_review' && (
            <>
              <Button
                size="sm"
                variant="outline"
                onClick={() => onReject?.(candidate.id)}
                disabled={isLoading}
                className="text-red-600 hover:text-red-700 hover:bg-red-50 dark:hover:bg-red-950/20"
              >
                <X className="h-4 w-4 mr-1" />
                Reject
              </Button>
              <Button
                size="sm"
                variant="default"
                onClick={() => onAccept?.(candidate.id)}
                disabled={isLoading}
              >
                <Check className="h-4 w-4 mr-1" />
                Accept
              </Button>
            </>
          )}

          {variant === 'rejected' && onAccept && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => onAccept(candidate.id)}
              disabled={isLoading}
              className="text-gray-500 hover:text-green-600 hover:bg-green-50 dark:hover:bg-green-950/20"
            >
              <Undo2 className="h-4 w-4 mr-1" />
              Restore
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
