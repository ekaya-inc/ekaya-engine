import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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
    deleteOntology: vi.fn(),
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

// Mock the Input component
vi.mock('../../ui/Input', () => ({
  Input: ({ id, value, onChange, placeholder, className, type }: {
    id?: string;
    value: string;
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
    placeholder?: string;
    className?: string;
    type?: string;
  }) => (
    <input
      id={id}
      data-testid="delete-confirm-input"
      type={type ?? 'text'}
      value={value}
      onChange={onChange}
      placeholder={placeholder}
      className={className}
    />
  ),
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

describe('OntologyDAG - Delete Ontology Functionality', () => {
  const mockProps = {
    projectId: 'proj-1',
    datasourceId: 'ds-1',
    onComplete: vi.fn(),
    onError: vi.fn(),
  };

  const mockRunningDAG: DAGStatusResponse = {
    dag_id: 'dag-running',
    status: 'running',
    current_node: 'EntityDiscovery',
    nodes: [
      {
        name: 'EntityDiscovery',
        status: 'running',
        progress: { current: 10, total: 38, message: 'Processing...' },
      },
    ],
    started_at: '2024-01-20T10:00:00Z',
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows Delete Ontology button when DAG is complete', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete ontology/i })).toBeInTheDocument();
    });
  });

  it('shows Delete Ontology button when DAG is failed', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockFailedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete ontology/i })).toBeInTheDocument();
    });
  });

  it('does NOT show Delete Ontology button when DAG is running', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockRunningDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText(/Extracting ontology\.\.\./)).toBeInTheDocument();
    });

    expect(screen.queryByRole('button', { name: /delete ontology/i })).not.toBeInTheDocument();
  });

  it('shows delete confirmation dialog when Delete Ontology button is clicked', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete ontology/i })).toBeInTheDocument();
    });

    const deleteButton = screen.getByRole('button', { name: /delete ontology/i });
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(screen.getByTestId('dialog-title')).toHaveTextContent('Delete Ontology?');
      expect(screen.getByText(/permanently delete all ontology data/)).toBeInTheDocument();
      expect(screen.getByText(/This is a destructive action/)).toBeInTheDocument();
    });
  });

  it('delete confirm button is disabled until user types "delete ontology"', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete ontology/i })).toBeInTheDocument();
    });

    const deleteButton = screen.getByRole('button', { name: /delete ontology/i });
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(screen.getByTestId('dialog-title')).toBeInTheDocument();
    });

    // Find the confirm button in the dialog (second button with "Delete Ontology" text)
    const dialogFooter = screen.getByTestId('dialog-footer');
    const confirmButton = dialogFooter.querySelector('button:last-child') as HTMLButtonElement;

    // Initially disabled
    expect(confirmButton).toBeDisabled();

    // Type incorrect text
    const input = screen.getByTestId('delete-confirm-input');
    await user.type(input, 'wrong text');
    expect(confirmButton).toBeDisabled();

    // Clear and type correct text
    await user.clear(input);
    await user.type(input, 'delete ontology');
    expect(confirmButton).not.toBeDisabled();
  });

  it('calls deleteOntology API and resets state on successful deletion', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });
    vi.mocked(engineApi.deleteOntology).mockResolvedValue({
      success: true,
    });

    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete ontology/i })).toBeInTheDocument();
    });

    // Click delete button to open dialog
    const deleteButton = screen.getByRole('button', { name: /delete ontology/i });
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(screen.getByTestId('dialog-title')).toBeInTheDocument();
    });

    // Type confirmation text
    const input = screen.getByTestId('delete-confirm-input');
    await user.type(input, 'delete ontology');

    // Click confirm button
    const dialogFooter = screen.getByTestId('dialog-footer');
    const confirmButton = dialogFooter.querySelector('button:last-child') as HTMLButtonElement;
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(engineApi.deleteOntology).toHaveBeenCalledWith('proj-1', 'ds-1');
    });

    // State should reset - shows empty state
    await waitFor(() => {
      expect(screen.getByText('Ready to Extract Ontology')).toBeInTheDocument();
    });
  });

  it('closes dialog and resets confirmation text when cancel is clicked', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete ontology/i })).toBeInTheDocument();
    });

    // Click delete button to open dialog
    const deleteButton = screen.getByRole('button', { name: /delete ontology/i });
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(screen.getByTestId('dialog-title')).toBeInTheDocument();
    });

    // Type some text in the confirmation input
    const input = screen.getByTestId('delete-confirm-input');
    await user.type(input, 'delete');

    // Click cancel button
    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    fireEvent.click(cancelButton);

    // Dialog should close
    await waitFor(() => {
      expect(screen.queryByTestId('dialog-title')).not.toBeInTheDocument();
    });

    // API should not have been called
    expect(engineApi.deleteOntology).not.toHaveBeenCalled();

    // Re-open dialog to verify confirmation text was reset
    fireEvent.click(deleteButton);

    await waitFor(() => {
      const newInput = screen.getByTestId('delete-confirm-input');
      expect(newInput).toHaveValue('');
    });
  });

  it('shows error when delete API fails', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });
    vi.mocked(engineApi.deleteOntology).mockRejectedValue(new Error('Network error'));

    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete ontology/i })).toBeInTheDocument();
    });

    // Click delete button to open dialog
    const deleteButton = screen.getByRole('button', { name: /delete ontology/i });
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(screen.getByTestId('dialog-title')).toBeInTheDocument();
    });

    // Type confirmation text and confirm
    const input = screen.getByTestId('delete-confirm-input');
    await user.type(input, 'delete ontology');

    const dialogFooter = screen.getByTestId('dialog-footer');
    const confirmButton = dialogFooter.querySelector('button:last-child') as HTMLButtonElement;
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(engineApi.deleteOntology).toHaveBeenCalledWith('proj-1', 'ds-1');
    });

    // onError callback should be called
    await waitFor(() => {
      expect(mockProps.onError).toHaveBeenCalledWith('Network error');
    });
  });

  it('calls onStatusChange with false when no DAG exists', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValueOnce({
      data: null,
      success: true,
    });

    const onStatusChange = vi.fn();
    render(<OntologyDAG {...mockProps} onStatusChange={onStatusChange} />);

    await waitFor(() => {
      expect(onStatusChange).toHaveBeenCalledWith(false);
    });
  });

  it('calls onStatusChange with true when DAG exists', async () => {
    const onStatusChange = vi.fn();
    render(<OntologyDAG {...mockProps} onStatusChange={onStatusChange} />);

    await waitFor(() => {
      expect(onStatusChange).toHaveBeenCalledWith(true);
    });
  });

  it('calls onStatusChange when DAG is deleted', async () => {
    vi.mocked(engineApi.deleteOntology).mockResolvedValueOnce({
      data: { message: 'Deleted' },
      success: true,
    });

    const onStatusChange = vi.fn();
    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} onStatusChange={onStatusChange} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete ontology/i })).toBeInTheDocument();
    });

    // Initial call with true (DAG exists)
    expect(onStatusChange).toHaveBeenCalledWith(true);

    // Click delete button to open dialog
    const deleteButton = screen.getByRole('button', { name: /delete ontology/i });
    fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(screen.getByTestId('dialog-title')).toBeInTheDocument();
    });

    // Type confirmation text and confirm
    const input = screen.getByTestId('delete-confirm-input');
    await user.type(input, 'delete ontology');

    const dialogFooter = screen.getByTestId('dialog-footer');
    const confirmButton = dialogFooter.querySelector('button:last-child') as HTMLButtonElement;
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(engineApi.deleteOntology).toHaveBeenCalledWith('proj-1', 'ds-1');
    });

    // Should be called with false after deletion
    await waitFor(() => {
      expect(onStatusChange).toHaveBeenCalledWith(false);
    });
  });
});
