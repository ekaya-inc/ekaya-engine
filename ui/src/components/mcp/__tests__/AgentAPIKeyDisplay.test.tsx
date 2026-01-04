import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../../services/engineApi';
import AgentAPIKeyDisplay from '../AgentAPIKeyDisplay';

// Mock the engineApi module
vi.mock('../../../services/engineApi', () => ({
  default: {
    getAgentAPIKey: vi.fn(),
    regenerateAgentAPIKey: vi.fn(),
  },
}));

// Mock useToast hook
const mockToast = vi.fn();
vi.mock('../../../hooks/useToast', () => ({
  useToast: () => ({
    toast: mockToast,
  }),
}));

// Mock the Dialog components to render without portal
vi.mock('../../ui/Dialog', () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <div data-testid="dialog-root">{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="dialog-content">{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="dialog-header">{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) =>
    <h2 data-testid="dialog-title">{children}</h2>,
  DialogDescription: ({ children }: { children: React.ReactNode }) =>
    <p data-testid="dialog-description">{children}</p>,
  DialogFooter: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="dialog-footer">{children}</div>,
}));

// Mock clipboard API
const mockClipboard = {
  writeText: vi.fn(),
};

Object.defineProperty(navigator, 'clipboard', {
  value: mockClipboard,
  writable: true,
  configurable: true,
});

describe('AgentAPIKeyDisplay', () => {
  const projectId = 'test-project-id';
  const maskedKey = '************************************************************';
  const displayMask = '*'.repeat(64); // Component uses fixed 64-char display mask
  const revealedKey = 'abcd1234efgh5678ijkl9012mnop3456qrst7890uvwx1234yzab5678';

  beforeEach(() => {
    vi.clearAllMocks();
    mockClipboard.writeText.mockResolvedValue(undefined);
  });

  it('shows loading state initially', async () => {
    // Create a promise that won't resolve immediately
    vi.mocked(engineApi.getAgentAPIKey).mockReturnValue(new Promise(() => {}));

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    expect(screen.getByText('Loading...')).toBeInTheDocument();
  });

  it('displays masked key after loading', async () => {
    vi.mocked(engineApi.getAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: maskedKey, masked: true },
    });

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    const input = screen.getByRole('textbox');
    expect(input).toHaveValue(displayMask);
    expect(input).toHaveClass('font-mono');
  });

  it('reveals key on input focus', async () => {
    vi.mocked(engineApi.getAgentAPIKey)
      .mockResolvedValueOnce({
        success: true,
        data: { key: maskedKey, masked: true },
      })
      .mockResolvedValueOnce({
        success: true,
        data: { key: revealedKey, masked: false },
      });

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    const input = screen.getByRole('textbox');
    fireEvent.focus(input);

    await waitFor(() => {
      expect(input).toHaveValue(revealedKey);
    });

    // Should have called with reveal=true
    expect(engineApi.getAgentAPIKey).toHaveBeenCalledWith(projectId, true);
  });

  it('copies masked key by fetching revealed key first', async () => {
    vi.mocked(engineApi.getAgentAPIKey)
      .mockResolvedValueOnce({
        success: true,
        data: { key: maskedKey, masked: true },
      })
      .mockResolvedValueOnce({
        success: true,
        data: { key: revealedKey, masked: false },
      });

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    const copyButton = screen.getByTitle('Copy to clipboard');
    fireEvent.click(copyButton);

    await waitFor(() => {
      expect(mockClipboard.writeText).toHaveBeenCalledWith(revealedKey);
    });

    expect(mockToast).toHaveBeenCalledWith({
      title: 'Copied',
      description: 'Agent API key copied to clipboard',
      variant: 'success',
    });
  });

  it('copies already revealed key without additional fetch', async () => {
    vi.mocked(engineApi.getAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: revealedKey, masked: false },
    });

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    const copyButton = screen.getByTitle('Copy to clipboard');
    fireEvent.click(copyButton);

    await waitFor(() => {
      expect(mockClipboard.writeText).toHaveBeenCalledWith(revealedKey);
    });

    // Should only have called once (on mount)
    expect(engineApi.getAgentAPIKey).toHaveBeenCalledTimes(1);
  });

  it('shows error toast when copy fails', async () => {
    vi.mocked(engineApi.getAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: revealedKey, masked: false },
    });
    mockClipboard.writeText.mockRejectedValueOnce(new Error('Copy failed'));

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    const copyButton = screen.getByTitle('Copy to clipboard');
    fireEvent.click(copyButton);

    await waitFor(() => {
      expect(mockToast).toHaveBeenCalledWith({
        title: 'Error',
        description: 'Failed to copy API key',
        variant: 'destructive',
      });
    });
  });

  it('opens confirmation dialog when regenerate button is clicked', async () => {
    vi.mocked(engineApi.getAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: maskedKey, masked: true },
    });

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    // Dialog should not be visible initially
    expect(screen.queryByTestId('dialog-root')).not.toBeInTheDocument();

    const regenerateButton = screen.getByTitle('Rotate key');
    fireEvent.click(regenerateButton);

    // Dialog should now be visible
    expect(screen.getByTestId('dialog-root')).toBeInTheDocument();
    expect(screen.getByText('Rotate API Key?')).toBeInTheDocument();
    expect(screen.getByText(/This will reset the API key/)).toBeInTheDocument();
  });

  it('closes confirmation dialog when cancel is clicked', async () => {
    vi.mocked(engineApi.getAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: maskedKey, masked: true },
    });

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    // Open dialog
    const regenerateButton = screen.getByTitle('Rotate key');
    fireEvent.click(regenerateButton);

    expect(screen.getByTestId('dialog-root')).toBeInTheDocument();

    // Click cancel
    const cancelButton = screen.getByRole('button', { name: /cancel/i });
    fireEvent.click(cancelButton);

    await waitFor(() => {
      expect(screen.queryByTestId('dialog-root')).not.toBeInTheDocument();
    });
  });

  it('regenerates key when confirmed', async () => {
    const newKey = 'newkey123456789newkey123456789newkey123456789newkey12345';

    vi.mocked(engineApi.getAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: maskedKey, masked: true },
    });
    vi.mocked(engineApi.regenerateAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: newKey },
    });

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    // Open dialog
    const regenerateButton = screen.getByTitle('Rotate key');
    fireEvent.click(regenerateButton);

    // Confirm regeneration - find button within dialog
    const dialogFooter = screen.getByTestId('dialog-footer');
    const confirmButton = within(dialogFooter).getByRole('button', { name: /rotate key/i });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(engineApi.regenerateAgentAPIKey).toHaveBeenCalledWith(projectId);
    });

    // Key should be updated
    const input = screen.getByRole('textbox');
    await waitFor(() => {
      expect(input).toHaveValue(newKey);
    });

    expect(mockToast).toHaveBeenCalledWith({
      title: 'Key Rotated',
      description: 'Agent API key has been rotated',
      variant: 'success',
    });
  });

  it('shows error toast when regeneration fails', async () => {
    vi.mocked(engineApi.getAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: maskedKey, masked: true },
    });
    vi.mocked(engineApi.regenerateAgentAPIKey).mockRejectedValue(new Error('Regeneration failed'));

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    // Open dialog
    const regenerateButton = screen.getByTitle('Rotate key');
    fireEvent.click(regenerateButton);

    // Confirm regeneration - find button within dialog
    const dialogFooter = screen.getByTestId('dialog-footer');
    const confirmButton = within(dialogFooter).getByRole('button', { name: /rotate key/i });
    fireEvent.click(confirmButton);

    await waitFor(() => {
      expect(mockToast).toHaveBeenCalledWith({
        title: 'Error',
        description: 'Failed to rotate API key',
        variant: 'destructive',
      });
    });
  });

  it('shows spinning icon while regenerating', async () => {
    // Create a promise that we can control
    let resolveRegenerate: (value: unknown) => void;
    const regeneratePromise = new Promise((resolve) => {
      resolveRegenerate = resolve;
    });

    vi.mocked(engineApi.getAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: maskedKey, masked: true },
    });
    vi.mocked(engineApi.regenerateAgentAPIKey).mockReturnValue(regeneratePromise as never);

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    // Open dialog and confirm
    const regenerateButton = screen.getByTitle('Rotate key');
    fireEvent.click(regenerateButton);

    // Find button within dialog
    const dialogFooter = screen.getByTestId('dialog-footer');
    const confirmButton = within(dialogFooter).getByRole('button', { name: /rotate key/i });
    fireEvent.click(confirmButton);

    // Should show spinning animation
    await waitFor(() => {
      const refreshIcon = regenerateButton.querySelector('svg');
      expect(refreshIcon).toHaveClass('animate-spin');
    });

    // Button should be disabled
    expect(regenerateButton).toBeDisabled();

    // Resolve the promise
    resolveRegenerate!({ success: true, data: { key: 'newkey' } });
  });

  it('displays helper text', async () => {
    vi.mocked(engineApi.getAgentAPIKey).mockResolvedValue({
      success: true,
      data: { key: maskedKey, masked: true },
    });

    render(<AgentAPIKeyDisplay projectId={projectId} />);

    await waitFor(() => {
      expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Click the key to reveal.')).toBeInTheDocument();
  });
});
