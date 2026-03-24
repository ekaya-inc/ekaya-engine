import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as authToken from '../../../lib/auth-token';
import engineApi from '../../../services/engineApi';
import type { DAGStatusResponse } from '../../../types';
import { OntologyDAG } from '../OntologyDAG';

// Mock the engineApi module
vi.mock('../../../services/engineApi', () => ({
  default: {
    getOntologyDAGStatus: vi.fn(),
    getOntologyStatus: vi.fn().mockResolvedValue({ success: true, data: { has_ontology: false, schema_changed_since_build: false } }),
    getProjectOverview: vi.fn(),
    startOntologyExtraction: vi.fn(),
    cancelOntologyDAG: vi.fn(),
    deleteOntology: vi.fn(),
    exportOntologyBundle: vi.fn(),
    importOntologyBundle: vi.fn(),
  },
}));

vi.mock('../../../lib/auth-token', () => ({
  getUserRoles: vi.fn(() => []),
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
      data-testid={id ? `${id}-input` : 'input'}
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
  is_incremental: false,
  nodes: [
    {
      name: 'KnowledgeSeeding',
      status: 'completed',
      progress: { current: 1, total: 1, message: 'Knowledge seeded' },
    },
    {
      name: 'ColumnEnrichment',
      status: 'completed',
      progress: { current: 38, total: 38, message: 'Enriched 38 columns' },
    },
  ],
  started_at: '2024-01-20T10:00:00Z',
  completed_at: '2024-01-20T10:15:00Z',
};

const mockFailedDAG: DAGStatusResponse = {
  dag_id: 'dag-2',
  status: 'failed',
  is_incremental: false,
  current_node: 'KnowledgeSeeding',
  nodes: [
    {
      name: 'KnowledgeSeeding',
      status: 'failed',
      error: 'Connection failed',
    },
  ],
  started_at: '2024-01-20T10:00:00Z',
  completed_at: '2024-01-20T10:01:00Z',
};

describe('OntologyDAG - Retry Failed Extraction', () => {
  const mockProps = {
    projectId: 'proj-1',
    datasourceId: 'ds-1',
    onComplete: vi.fn(),
    onError: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(authToken.getUserRoles).mockReturnValue([]);
    vi.mocked(engineApi.getProjectOverview).mockResolvedValue({
      success: true,
      data: { overview: null },
    });
  });

  it('shows retry button and starts extraction without confirmation dialog', async () => {
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

    await waitFor(() => {
      expect(engineApi.startOntologyExtraction).toHaveBeenCalledWith('proj-1', 'ds-1', undefined);
    });

    expect(screen.queryByTestId('dialog-title')).not.toBeInTheDocument();
  });

  it('does not show retry button for completed state', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /delete ontology/i })).toBeInTheDocument();
    });

    expect(screen.queryByRole('button', { name: /retry extraction/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /re-extract/i })).not.toBeInTheDocument();
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
    is_incremental: false,
    current_node: 'ColumnEnrichment',
    nodes: [
      {
        name: 'ColumnEnrichment',
        status: 'running',
        progress: { current: 10, total: 38, message: 'Processing...' },
      },
    ],
    started_at: '2024-01-20T10:00:00Z',
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(authToken.getUserRoles).mockReturnValue([]);
    // Default mock for getProjectOverview - returns no overview
    vi.mocked(engineApi.getProjectOverview).mockResolvedValue({
      success: true,
      data: { overview: null },
    });
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
    vi.mocked(engineApi.getOntologyStatus).mockResolvedValueOnce({
      success: true,
      data: { has_ontology: true, schema_changed_since_build: false, completion_provenance: 'extracted' },
    });

    const onStatusChange = vi.fn();
    render(<OntologyDAG {...mockProps} onStatusChange={onStatusChange} />);

    await waitFor(() => {
      expect(onStatusChange).toHaveBeenCalledWith(true);
    });
  });

  it('calls onStatusChange when DAG is deleted', async () => {
    vi.mocked(engineApi.getOntologyStatus).mockResolvedValueOnce({
      success: true,
      data: { has_ontology: true, schema_changed_since_build: false, completion_provenance: 'extracted' },
    });
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

describe('OntologyDAG - Import Ontology', () => {
  const mockProps = {
    projectId: 'proj-1',
    datasourceId: 'ds-1',
    onComplete: vi.fn(),
    onError: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(authToken.getUserRoles).mockReturnValue(['admin']);
    vi.mocked(engineApi.getProjectOverview).mockResolvedValue({
      success: true,
      data: { overview: null },
    });
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: null,
    });
    vi.mocked(engineApi.getOntologyStatus).mockResolvedValue({
      success: true,
      data: { has_ontology: false, schema_changed_since_build: false },
    });
  });

  it('shows Import Ontology on the ready-to-extract empty state for admins', async () => {
    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText('Ready to Extract Ontology')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /import ontology/i })).toBeInTheDocument();
  });

  it('does not show Import Ontology for non-admin users', async () => {
    vi.mocked(authToken.getUserRoles).mockReturnValue(['user']);

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByText('Ready to Extract Ontology')).toBeInTheDocument();
    });

    expect(screen.queryByRole('button', { name: /import ontology/i })).not.toBeInTheDocument();
  });

  it('imports a bundle and switches to the import-complete state', async () => {
    vi.mocked(engineApi.importOntologyBundle).mockResolvedValue({
      success: true,
      data: {
        imported_at: '2026-03-24T10:00:00Z',
        completion_provenance: 'imported',
      },
    });

    const onStatusChange = vi.fn();
    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} onStatusChange={onStatusChange} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /import ontology/i })).toBeInTheDocument();
    });

    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(['{"format":"ekaya-ontology-export"}'], 'bundle.json', {
      type: 'application/json',
    });

    await user.upload(fileInput, file);

    await waitFor(() => {
      expect(engineApi.importOntologyBundle).toHaveBeenCalledWith('proj-1', 'ds-1', file);
    });

    await waitFor(() => {
      expect(screen.getByText('Ontology Import Complete')).toBeInTheDocument();
      expect(
        screen.queryByText(
          'This datasource is using imported ontology state instead of an extracted DAG.',
        ),
      ).not.toBeInTheDocument();
      expect(onStatusChange).toHaveBeenCalledWith(true);
    });
  });

  it('shows a client-side size error and skips the API call for oversized bundles', async () => {
    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /import ontology/i })).toBeInTheDocument();
    });

    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(['{}'], 'bundle.json', { type: 'application/json' });
    Object.defineProperty(file, 'size', { value: 5 * 1024 * 1024 + 1 });

    await user.upload(fileInput, file);

    await waitFor(() => {
      expect(screen.getByText('Ontology bundle exceeds the 5 MB maximum size')).toBeInTheDocument();
    });
    expect(engineApi.importOntologyBundle).not.toHaveBeenCalled();
  });

  it('renders a structured validation report and dismisses it', async () => {
    vi.mocked(engineApi.importOntologyBundle).mockRejectedValue({
      message: 'Ontology bundle does not match the selected datasource schema',
      report: {
        missing_required_apps: ['ai_data_liaison'],
        missing_tables: [{ schema_name: 'public', table_name: 'orders' }],
        missing_columns: [
          {
            table: { schema_name: 'public', table_name: 'orders' },
            column_name: 'status',
          },
        ],
      },
    });

    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /import ontology/i })).toBeInTheDocument();
    });

    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
    const file = new File(['{"format":"ekaya-ontology-export"}'], 'bundle.json', {
      type: 'application/json',
    });

    await user.upload(fileInput, file);

    await waitFor(() => {
      expect(screen.getByText('Ontology bundle does not match the selected datasource schema')).toBeInTheDocument();
      expect(screen.getByText('Missing required apps')).toBeInTheDocument();
      expect(screen.getByText('ai_data_liaison')).toBeInTheDocument();
      expect(screen.getByText('public.orders')).toBeInTheDocument();
      expect(screen.getByText('public.orders.status')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /dismiss/i }));

    await waitFor(() => {
      expect(screen.queryByText('Missing required apps')).not.toBeInTheDocument();
    });
  });
});

describe('OntologyDAG - Export Ontology', () => {
  const mockProps = {
    projectId: 'proj-1',
    datasourceId: 'ds-1',
    onComplete: vi.fn(),
    onError: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(authToken.getUserRoles).mockReturnValue([]);
    vi.mocked(engineApi.getProjectOverview).mockResolvedValue({
      success: true,
      data: { overview: null },
    });
  });

  it('shows Export Ontology only when the DAG is complete', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /export ontology/i })).toBeInTheDocument();
    });
  });

  it('does not show Export Ontology when the DAG is not complete', async () => {
    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockFailedDAG,
    });

    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /export ontology/i })).not.toBeInTheDocument();
    });
  });

  it('downloads the bundle directly from the export button', async () => {
    const blob = new Blob(['{"format":"ekaya-ontology-export"}'], { type: 'application/json' });
    const originalCreateElement = document.createElement.bind(document);
    const link = originalCreateElement('a');
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:export');
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    const click = vi.spyOn(link, 'click').mockImplementation(() => {});
    const remove = vi.spyOn(link, 'remove').mockImplementation(() => {});
    const appendChild = vi.spyOn(document.body, 'appendChild');
    const createElement = vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
      if (tagName.toLowerCase() === 'a') {
        return link;
      }
      return originalCreateElement(tagName);
    });

    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });
    vi.mocked(engineApi.exportOntologyBundle).mockResolvedValue(blob);

    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /export ontology/i })).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /export ontology/i }));

    await waitFor(() => {
      expect(engineApi.exportOntologyBundle).toHaveBeenCalledWith('proj-1', 'ds-1');
      expect(createObjectURL).toHaveBeenCalledWith(blob);
      expect(click).toHaveBeenCalled();
      expect(remove).toHaveBeenCalled();
      expect(revokeObjectURL).toHaveBeenCalledWith('blob:export');
      expect(appendChild).toHaveBeenCalled();
    });

    createObjectURL.mockRestore();
    revokeObjectURL.mockRestore();
    appendChild.mockRestore();
    createElement.mockRestore();
    click.mockRestore();
    remove.mockRestore();
  });

  it('downloads directly even if File System Access API exists', async () => {
    const blob = new Blob(['{"format":"ekaya-ontology-export"}'], { type: 'application/json' });
    const originalCreateElement = document.createElement.bind(document);
    const link = originalCreateElement('a');
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:export');
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    const click = vi.spyOn(link, 'click').mockImplementation(() => {});
    const remove = vi.spyOn(link, 'remove').mockImplementation(() => {});
    const appendChild = vi.spyOn(document.body, 'appendChild');
    const createElement = vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
      if (tagName.toLowerCase() === 'a') {
        return link;
      }
      return originalCreateElement(tagName);
    });

    vi.mocked(engineApi.getOntologyDAGStatus).mockResolvedValue({
      success: true,
      data: mockCompletedDAG,
    });
    vi.mocked(engineApi.exportOntologyBundle).mockResolvedValue(blob);
    Object.assign(window, { showSaveFilePicker: vi.fn() });

    const user = userEvent.setup();
    render(<OntologyDAG {...mockProps} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /export ontology/i })).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /export ontology/i }));

    await waitFor(() => {
      expect(engineApi.exportOntologyBundle).toHaveBeenCalledWith('proj-1', 'ds-1');
      expect(createObjectURL).toHaveBeenCalledWith(blob);
      expect(click).toHaveBeenCalled();
      expect(remove).toHaveBeenCalled();
      expect(revokeObjectURL).toHaveBeenCalledWith('blob:export');
      expect(appendChild).toHaveBeenCalled();
    });

    createObjectURL.mockRestore();
    revokeObjectURL.mockRestore();
    appendChild.mockRestore();
    createElement.mockRestore();
    click.mockRestore();
    remove.mockRestore();
  });
});
