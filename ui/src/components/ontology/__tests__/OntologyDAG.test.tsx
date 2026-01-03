import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../../services/engineApi';
import type { DAGStatusResponse } from '../../../types';
import { OntologyDAG } from '../OntologyDAG';

// Mock the engineApi module
vi.mock('../../../services/engineApi', () => ({
  default: {
    getOntologyDAGStatus: vi.fn(),
    startOntologyExtraction: vi.fn(),
    cancelOntologyDAG: vi.fn(),
  },
}));

// Mock the Dialog components to render without portal
vi.mock('../../ui/Dialog', () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <div data-testid="dialog-root">{children}</div> : null,
  DialogContent: ({ children, className }: { children: React.ReactNode; className?: string }) =>
    <div data-testid="dialog-content" className={className}>{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="dialog-header">{children}</div>,
  DialogTitle: ({ children, className }: { children: React.ReactNode; className?: string }) =>
    <h2 data-testid="dialog-title" className={className}>{children}</h2>,
  DialogDescription: ({ children }: { children: React.ReactNode }) =>
    <p data-testid="dialog-description">{children}</p>,
  DialogFooter: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="dialog-footer">{children}</div>,
}));

const mockCompletedDAG: DAGStatusResponse = {
  dag_id: 'dag-1',
  status: 'completed',
  nodes: [
    {
      name: 'EntityDiscovery',
      status: 'completed',
      progress: { current: 38, total: 38, message: 'Discovered 38 entities' },
    },
    {
      name: 'EntityEnrichment',
      status: 'completed',
      progress: { current: 38, total: 38, message: 'Enriched 38 entities' },
    },
  ],
  started_at: '2024-01-20T10:00:00Z',
  completed_at: '2024-01-20T10:15:00Z',
};

const mockFailedDAG: DAGStatusResponse = {
  dag_id: 'dag-2',
  status: 'failed',
  current_node: 'EntityDiscovery',
  nodes: [
    {
      name: 'EntityDiscovery',
      status: 'failed',
      error: 'Connection failed',
    },
  ],
  started_at: '2024-01-20T10:00:00Z',
  completed_at: '2024-01-20T10:01:00Z',
};

describe('OntologyDAG - Re-extraction Confirmation', () => {
  const mockProps = {
    projectId: 'proj-1',
    datasourceId: 'ds-1',
    onComplete: vi.fn(),
    onError: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows confirmation dialog when re-extract button is clicked after completion', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText('Re-extract Ontology')).toBeInTheDocument();
    });

    const reextractButton = screen.getByRole('button', { name: /re-extract ontology/i });
    fireEvent.click(reextractButton);

    await waitFor(() => {
      expect(screen.getByTestId('dialog-title')).toHaveTextContent('Re-extract Ontology?');
      expect(screen.getByText(/This will start a complete re-extraction/)).toBeInTheDocument();
      expect(screen.getByText(/10-15 minutes/)).toBeInTheDocument();
    });
  });

  it('shows warning about full re-extraction in dialog', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText('Re-extract Ontology')).toBeInTheDocument();
    });

    const reextractButton = screen.getByRole('button', { name: /re-extract ontology/i });
    fireEvent.click(reextractButton);

    await waitFor(() => {
      expect(screen.getByText('This is a full re-extraction')).toBeInTheDocument();
      expect(
        screen.getByText(/this feature is not yet implemented/)
      ).toBeInTheDocument();
    });
  });

  it('starts re-extraction when confirmed in dialog', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    const newDAG: DAGStatusResponse = {
      ...mockCompletedDAG,
      dag_id: 'dag-new',
      status: 'running',
    };

    vi.mocked(engineApi.startOntologyExtraction).mockResolvedValue({
      success: true,
      data: newDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText('Re-extract Ontology')).toBeInTheDocument();
    });

    const reextractButton = screen.getByRole('button', { name: /re-extract ontology/i });
    fireEvent.click(reextractButton);

    await waitFor(() => {
      expect(screen.getByTestId('dialog-title')).toHaveTextContent('Re-extract Ontology?');
    });

    const confirmButton = screen.getByRole('button', { name: /start re-extraction/i });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(engineApi.startOntologyExtraction).toHaveBeenCalledWith('proj-1', 'ds-1');
    });
  });

  it('closes dialog when cancel is clicked', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText('Re-extract Ontology')).toBeInTheDocument();
    });

    const reextractButton = screen.getByRole('button', { name: /re-extract ontology/i });
    fireEvent.click(reextractButton);

    await waitFor(() => {
      expect(screen.getByTestId('dialog-title')).toBeInTheDocument();
    });

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    fireEvent.click(cancelButton);

    await waitFor(() => {
      expect(screen.queryByTestId('dialog-title')).not.toBeInTheDocument();
    });

    // API should not have been called
    expect(engineApi.startOntologyExtraction).not.toHaveBeenCalled();
  });

  it('does NOT show confirmation dialog when retrying failed extraction', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockFailedDAG,
    });

    const newDAG: DAGStatusResponse = {
      ...mockFailedDAG,
      dag_id: 'dag-retry',
      status: 'running',
    };

    vi.mocked(engineApi.startOntologyExtraction).mockResolvedValue({
      success: true,
      data: newDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText('Retry Extraction')).toBeInTheDocument();
    });

    const retryButton = screen.getByRole('button', { name: /retry extraction/i });
    fireEvent.click(retryButton);

    // Dialog should NOT appear
    await waitFor(() => {
      expect(engineApi.startOntologyExtraction).toHaveBeenCalledWith('proj-1', 'ds-1');
    });

    // Verify dialog was never shown
    expect(screen.queryByTestId('dialog-title')).not.toBeInTheDocument();
  });

  it('shows "Re-extract Ontology" button text for completed state', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText('Re-extract Ontology')).toBeInTheDocument();
    });
  });

  it('shows "Retry Extraction" button text for failed state', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockFailedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText('Retry Extraction')).toBeInTheDocument();
    });
  });
});
