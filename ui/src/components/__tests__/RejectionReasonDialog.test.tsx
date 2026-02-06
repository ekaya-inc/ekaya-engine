import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RejectionReasonDialog } from '../RejectionReasonDialog';

// Mock the Dialog components to render without portal
vi.mock('../ui/Dialog', () => ({
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

describe('RejectionReasonDialog', () => {
  const mockOnOpenChange = vi.fn();
  const mockOnReject = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders dialog when open', () => {
    render(
      <RejectionReasonDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        queryName="Get user orders"
        onReject={mockOnReject}
      />
    );

    expect(screen.getByTestId('dialog-title')).toHaveTextContent('Reject Query Suggestion');
    expect(screen.getByText('Get user orders')).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/explain why/i)).toBeInTheDocument();
  });

  it('does not render when closed', () => {
    const { container } = render(
      <RejectionReasonDialog
        open={false}
        onOpenChange={mockOnOpenChange}
        queryName="Get user orders"
        onReject={mockOnReject}
      />
    );

    expect(container.firstChild).toBeNull();
  });

  it('has required field indicator for reason', () => {
    render(
      <RejectionReasonDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        queryName="Get user orders"
        onReject={mockOnReject}
      />
    );

    // Check that the required indicator is shown
    expect(screen.getByText('*')).toBeInTheDocument();
    expect(screen.getByText(/reason for rejection/i)).toBeInTheDocument();
  });

  it('reject button is disabled when reason is empty', () => {
    render(
      <RejectionReasonDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        queryName="Get user orders"
        onReject={mockOnReject}
      />
    );

    const rejectButton = screen.getByRole('button', { name: /reject query/i });
    expect(rejectButton).toBeDisabled();
  });

  it('calls onReject with reason when submitted', async () => {
    mockOnReject.mockResolvedValue(undefined);

    render(
      <RejectionReasonDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        queryName="Get user orders"
        onReject={mockOnReject}
      />
    );

    const textarea = screen.getByPlaceholderText(/explain why/i);
    fireEvent.change(textarea, { target: { value: 'SQL is not optimized' } });

    const rejectButton = screen.getByRole('button', { name: /reject query/i });
    expect(rejectButton).not.toBeDisabled();
    fireEvent.click(rejectButton);

    await waitFor(() => {
      expect(mockOnReject).toHaveBeenCalledWith('SQL is not optimized');
      expect(mockOnOpenChange).toHaveBeenCalledWith(false);
    });
  });

  it('displays error message on rejection failure', async () => {
    mockOnReject.mockRejectedValue(new Error('Network error'));

    render(
      <RejectionReasonDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        queryName="Get user orders"
        onReject={mockOnReject}
      />
    );

    const textarea = screen.getByPlaceholderText(/explain why/i);
    fireEvent.change(textarea, { target: { value: 'Invalid SQL' } });

    const rejectButton = screen.getByRole('button', { name: /reject query/i });
    fireEvent.click(rejectButton);

    await waitFor(() => {
      expect(screen.getByText('Network error')).toBeInTheDocument();
    });

    expect(mockOnOpenChange).not.toHaveBeenCalled();
  });

  it('shows loading state while rejecting', async () => {
    let resolveReject: () => void = () => { /* no-op */ };
    const rejectPromise = new Promise<void>((resolve) => {
      resolveReject = resolve;
    });

    mockOnReject.mockReturnValue(rejectPromise);

    render(
      <RejectionReasonDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        queryName="Get user orders"
        onReject={mockOnReject}
      />
    );

    const textarea = screen.getByPlaceholderText(/explain why/i);
    fireEvent.change(textarea, { target: { value: 'Security concern' } });

    const rejectButton = screen.getByRole('button', { name: /reject query/i });
    fireEvent.click(rejectButton);

    await waitFor(() => {
      expect(screen.getByText('Rejecting...')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /rejecting/i })).toBeDisabled();
    expect(screen.getByRole('button', { name: /cancel/i })).toBeDisabled();

    resolveReject();
  });

  it('calls onOpenChange when cancel is clicked', () => {
    render(
      <RejectionReasonDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        queryName="Get user orders"
        onReject={mockOnReject}
      />
    );

    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    fireEvent.click(cancelButton);

    expect(mockOnOpenChange).toHaveBeenCalledWith(false);
  });

  it('enables and disables reject button based on reason content', () => {
    render(
      <RejectionReasonDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        queryName="Get user orders"
        onReject={mockOnReject}
      />
    );

    const textarea = screen.getByPlaceholderText(/explain why/i);
    const rejectButton = screen.getByRole('button', { name: /reject query/i });

    // Initially disabled
    expect(rejectButton).toBeDisabled();

    // Type something - should enable
    fireEvent.change(textarea, { target: { value: 'Some reason' } });
    expect(rejectButton).not.toBeDisabled();

    // Clear - should disable again
    fireEvent.change(textarea, { target: { value: '' } });
    expect(rejectButton).toBeDisabled();

    // Type whitespace only - should still be disabled
    fireEvent.change(textarea, { target: { value: '   ' } });
    expect(rejectButton).toBeDisabled();

    // Type valid reason - should enable
    fireEvent.change(textarea, { target: { value: 'Valid reason' } });
    expect(rejectButton).not.toBeDisabled();
  });
});
