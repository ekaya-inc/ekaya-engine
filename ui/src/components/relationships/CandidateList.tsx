/**
 * CandidateList Component
 * Displays grouped relationship candidates (confirmed, needs_review, rejected)
 */

import {
  AlertTriangle,
  Check,
  ChevronDown,
  ChevronRight,
  X,
} from 'lucide-react';
import { useState } from 'react';

import type { CandidateResponse, CandidatesResponse } from '../../types';

import { CandidateCard } from './CandidateCard';

interface CandidateListProps {
  candidates: CandidatesResponse;
  onAccept: (id: string) => void;
  onReject: (id: string) => void;
  loadingCandidateId?: string | null;
}

interface SectionProps {
  title: string;
  count: number;
  icon: React.ReactNode;
  variant: 'confirmed' | 'needs_review' | 'rejected';
  candidates: CandidateResponse[];
  defaultExpanded?: boolean;
  onAccept: (id: string) => void;
  onReject: (id: string) => void;
  loadingCandidateId: string | null | undefined;
}

function Section({
  title,
  count,
  icon,
  variant,
  candidates,
  defaultExpanded = true,
  onAccept,
  onReject,
  loadingCandidateId,
}: SectionProps): React.ReactElement | null {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);

  if (count === 0) {
    return null;
  }

  return (
    <div className="mb-4">
      {/* Section header */}
      <button
        type="button"
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center gap-2 w-full text-left p-2 rounded-lg hover:bg-surface-secondary/50 transition-colors"
      >
        {isExpanded ? (
          <ChevronDown className="h-4 w-4 text-text-tertiary" />
        ) : (
          <ChevronRight className="h-4 w-4 text-text-tertiary" />
        )}
        {icon}
        <span className="font-medium text-sm text-text-primary">{title}</span>
        <span className="text-xs text-text-secondary">({count})</span>
      </button>

      {/* Section content */}
      {isExpanded && (
        <div className="mt-2 space-y-2 pl-6">
          {candidates.map((candidate) => (
            <CandidateCard
              key={candidate.id}
              candidate={candidate}
              variant={variant}
              onAccept={onAccept}
              onReject={onReject}
              isLoading={loadingCandidateId === candidate.id}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function CandidateList({
  candidates,
  onAccept,
  onReject,
  loadingCandidateId,
}: CandidateListProps): React.ReactElement {
  const totalCount =
    candidates.confirmed.length +
    candidates.needs_review.length +
    candidates.rejected.length;

  if (totalCount === 0) {
    return (
      <div className="text-center py-8 text-text-secondary">
        <p>No relationship candidates found yet.</p>
        <p className="text-sm mt-1">
          Candidates will appear here as the workflow progresses.
        </p>
      </div>
    );
  }

  return (
    <div>
      {/* Needs Review section - always first and expanded */}
      <Section
        title="NEEDS REVIEW"
        count={candidates.needs_review.length}
        icon={<AlertTriangle className="h-4 w-4 text-amber-500" />}
        variant="needs_review"
        candidates={candidates.needs_review}
        defaultExpanded={true}
        onAccept={onAccept}
        onReject={onReject}
        loadingCandidateId={loadingCandidateId}
      />

      {/* Confirmed section */}
      <Section
        title="CONFIRMED"
        count={candidates.confirmed.length}
        icon={<Check className="h-4 w-4 text-green-500" />}
        variant="confirmed"
        candidates={candidates.confirmed}
        defaultExpanded={true}
        onAccept={onAccept}
        onReject={onReject}
        loadingCandidateId={loadingCandidateId}
      />

      {/* Rejected section - collapsed by default */}
      <Section
        title="REJECTED"
        count={candidates.rejected.length}
        icon={<X className="h-4 w-4 text-gray-400" />}
        variant="rejected"
        candidates={candidates.rejected}
        defaultExpanded={false}
        onAccept={onAccept}
        onReject={onReject}
        loadingCandidateId={loadingCandidateId}
      />
    </div>
  );
}
