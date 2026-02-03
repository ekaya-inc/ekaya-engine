import { render, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { DAGNodeProgress } from '../../../types';
import { ExtractionProgress } from '../ExtractionProgress';

describe('ExtractionProgress', () => {
  describe('rendering phases', () => {
    it('renders all phases when no progress is provided', () => {
      render(<ExtractionProgress progress={undefined} nodeStatus={undefined} />);

      expect(screen.getByText('Collecting column metadata')).toBeInTheDocument();
      expect(screen.getByText('Classifying columns')).toBeInTheDocument();
      expect(screen.getByText('Analyzing enum values')).toBeInTheDocument();
      expect(screen.getByText('Resolving foreign key targets')).toBeInTheDocument();
      expect(screen.getByText('Analyzing column relationships')).toBeInTheDocument();
      expect(screen.getByText('Saving results')).toBeInTheDocument();
    });

    it('marks all phases complete when node status is completed', () => {
      render(<ExtractionProgress progress={undefined} nodeStatus="completed" />);

      // All phases should have green checkmark icons (aria-label="Complete")
      const completeIcons = screen.getAllByLabelText('Complete');
      expect(completeIcons).toHaveLength(6);
    });

    it('renders phases with pending status by default', () => {
      render(<ExtractionProgress progress={undefined} nodeStatus={undefined} />);

      // All phases should have gray circle icons (aria-label="Pending")
      const pendingIcons = screen.getAllByLabelText('Pending');
      expect(pendingIcons).toHaveLength(6);
    });
  });

  describe('phase detection from progress message', () => {
    it('detects phase 1 from "Collecting column metadata..." message', () => {
      const progress: DAGNodeProgress = {
        current: 10,
        total: 38,
        message: 'Collecting column metadata...',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      // Phase 1 should be in progress
      const phase1Row = screen.getByText('Collecting column metadata').closest('[role="listitem"]');
      expect(phase1Row).toHaveAttribute('aria-current', 'step');

      // Should show progress counter
      expect(screen.getByText('10/38')).toBeInTheDocument();
    });

    it('detects phase 1 from "Found X columns in Y tables" message', () => {
      const progress: DAGNodeProgress = {
        current: 38,
        total: 38,
        message: 'Found 38 columns in 12 tables',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      // Phase 1 should be in progress
      const phase1Row = screen.getByText('Collecting column metadata').closest('[role="listitem"]');
      expect(phase1Row).toHaveAttribute('aria-current', 'step');
    });

    it('detects phase 2 from "Classifying columns" message', () => {
      const progress: DAGNodeProgress = {
        current: 23,
        total: 38,
        message: 'Classifying columns',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      // Phase 1 should be complete
      const phase1Row = screen.getByText('Collecting column metadata').closest('[role="listitem"]') as HTMLElement;
      expect(within(phase1Row).getByLabelText('Complete')).toBeInTheDocument();

      // Phase 2 should be in progress
      const phase2Row = screen.getByText('Classifying columns').closest('[role="listitem"]');
      expect(phase2Row).toHaveAttribute('aria-current', 'step');
      expect(screen.getByText('23/38')).toBeInTheDocument();
    });

    it('detects phase 3 from "Analyzing enum values" message', () => {
      const progress: DAGNodeProgress = {
        current: 3,
        total: 7,
        message: 'Analyzing enum values',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      // Phases 1-2 should be complete
      expect(within(screen.getByText('Collecting column metadata').closest('[role="listitem"]') as HTMLElement).getByLabelText('Complete')).toBeInTheDocument();
      expect(within(screen.getByText('Classifying columns').closest('[role="listitem"]') as HTMLElement).getByLabelText('Complete')).toBeInTheDocument();

      // Phase 3 should be in progress
      const phase3Row = screen.getByText('Analyzing enum values').closest('[role="listitem"]');
      expect(phase3Row).toHaveAttribute('aria-current', 'step');
      expect(screen.getByText('3/7')).toBeInTheDocument();
    });

    it('detects phase 4 from "Resolving foreign key targets" message', () => {
      const progress: DAGNodeProgress = {
        current: 2,
        total: 4,
        message: 'Resolving foreign key targets',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      // Phase 4 should be in progress
      const phase4Row = screen.getByText('Resolving foreign key targets').closest('[role="listitem"]');
      expect(phase4Row).toHaveAttribute('aria-current', 'step');
    });

    it('detects phase 5 from "Analyzing column relationships" message', () => {
      const progress: DAGNodeProgress = {
        current: 1,
        total: 3,
        message: 'Analyzing column relationships',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      // Phase 5 should be in progress
      const phase5Row = screen.getByText('Analyzing column relationships').closest('[role="listitem"]');
      expect(phase5Row).toHaveAttribute('aria-current', 'step');
    });

    it('detects phase 6 from "Saving column features..." message', () => {
      const progress: DAGNodeProgress = {
        current: 0,
        total: 1,
        message: 'Saving column features...',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      // Phases 1-5 should be complete
      expect(within(screen.getByText('Collecting column metadata').closest('[role="listitem"]') as HTMLElement).getByLabelText('Complete')).toBeInTheDocument();
      expect(within(screen.getByText('Classifying columns').closest('[role="listitem"]') as HTMLElement).getByLabelText('Complete')).toBeInTheDocument();
      expect(within(screen.getByText('Analyzing enum values').closest('[role="listitem"]') as HTMLElement).getByLabelText('Complete')).toBeInTheDocument();
      expect(within(screen.getByText('Resolving foreign key targets').closest('[role="listitem"]') as HTMLElement).getByLabelText('Complete')).toBeInTheDocument();
      expect(within(screen.getByText('Analyzing column relationships').closest('[role="listitem"]') as HTMLElement).getByLabelText('Complete')).toBeInTheDocument();

      // Phase 6 should be in progress
      const phase6Row = screen.getByText('Saving results').closest('[role="listitem"]');
      expect(phase6Row).toHaveAttribute('aria-current', 'step');
    });

    it('marks all phases complete when message ends with "complete"', () => {
      const progress: DAGNodeProgress = {
        current: 1,
        total: 1,
        message: 'Column feature extraction complete',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      // All phases should be complete
      const completeIcons = screen.getAllByLabelText('Complete');
      expect(completeIcons).toHaveLength(6);
    });
  });

  describe('progress bar', () => {
    it('shows progress bar for in-progress phase', () => {
      const progress: DAGNodeProgress = {
        current: 15,
        total: 38,
        message: 'Classifying columns',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      const progressBar = screen.getByRole('progressbar');
      expect(progressBar).toHaveAttribute('aria-valuenow', '15');
      expect(progressBar).toHaveAttribute('aria-valuemax', '38');
    });

    it('does not show progress bar when total is 0', () => {
      const progress: DAGNodeProgress = {
        current: 0,
        total: 0,
        message: 'Classifying columns',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    });

    it('calculates progress percentage correctly', () => {
      const progress: DAGNodeProgress = {
        current: 20,
        total: 40,
        message: 'Classifying columns',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      const progressBar = screen.getByRole('progressbar');
      // 20/40 = 50%
      expect(progressBar).toHaveStyle({ width: '50%' });
    });
  });

  describe('accessibility', () => {
    it('has proper list role', () => {
      render(<ExtractionProgress progress={undefined} nodeStatus={undefined} />);

      expect(screen.getByRole('list')).toHaveAttribute('aria-label', 'Extraction phases');
    });

    it('has proper listitem roles', () => {
      render(<ExtractionProgress progress={undefined} nodeStatus={undefined} />);

      const listItems = screen.getAllByRole('listitem');
      expect(listItems).toHaveLength(6);
    });

    it('marks current phase with aria-current', () => {
      const progress: DAGNodeProgress = {
        current: 10,
        total: 38,
        message: 'Classifying columns',
      };

      render(<ExtractionProgress progress={progress} nodeStatus="running" />);

      const currentPhase = screen.getByText('Classifying columns').closest('[role="listitem"]');
      expect(currentPhase).toHaveAttribute('aria-current', 'step');

      // Other phases should not have aria-current
      const otherPhase = screen.getByText('Collecting column metadata').closest('[role="listitem"]');
      expect(otherPhase).not.toHaveAttribute('aria-current');
    });
  });

  describe('className prop', () => {
    it('applies custom className', () => {
      const { container } = render(
        <ExtractionProgress progress={undefined} nodeStatus={undefined} className="custom-class" />
      );

      expect(container.firstChild).toHaveClass('custom-class');
    });
  });
});
