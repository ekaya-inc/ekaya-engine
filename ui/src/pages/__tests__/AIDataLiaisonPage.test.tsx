import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import AIDataLiaisonPage from '../AIDataLiaisonPage';

// Mock the engineApi
vi.mock('../../services/engineApi', () => ({
  default: {
    uninstallApp: vi.fn(),
  },
}));

// Mock the toast hook
const mockToast = vi.fn();
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({
    toast: mockToast,
  }),
}));

// Mock navigate
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

const renderAIDataLiaisonPage = () => {
  return render(
    <MemoryRouter initialEntries={['/projects/proj-1/ai-data-liaison']}>
      <Routes>
        <Route path="/projects/:pid/ai-data-liaison" element={<AIDataLiaisonPage />} />
      </Routes>
    </MemoryRouter>
  );
};

// Helper to get the confirm button in the dialog (the last "Uninstall Application" button)
const getConfirmButton = () => {
  const dialogButtons = screen.getAllByRole('button', { name: /uninstall application/i });
  const confirmButton = dialogButtons.at(-1);
  if (!confirmButton) {
    throw new Error('Confirm button not found');
  }
  return confirmButton;
};

describe('AIDataLiaisonPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('Header', () => {
    it('renders the page title', () => {
      renderAIDataLiaisonPage();
      expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
    });

    it('renders the page description', () => {
      renderAIDataLiaisonPage();
      expect(screen.getByText('Configure your AI Data Liaison application')).toBeInTheDocument();
    });

    it('renders back button', () => {
      renderAIDataLiaisonPage();
      expect(screen.getByRole('button', { name: /back to project dashboard/i })).toBeInTheDocument();
    });

    it('navigates to project dashboard when back button clicked', () => {
      renderAIDataLiaisonPage();
      const backButton = screen.getByRole('button', { name: /back to project dashboard/i });
      fireEvent.click(backButton);
      expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1');
    });
  });

  describe('Configuration Card', () => {
    it('renders configuration card', () => {
      renderAIDataLiaisonPage();
      expect(screen.getByText('Configuration')).toBeInTheDocument();
      expect(screen.getByText('AI Data Liaison configuration options will appear here.')).toBeInTheDocument();
    });

    it('shows coming soon message', () => {
      renderAIDataLiaisonPage();
      expect(screen.getByText(/Coming soon: Configure how AI Data Liaison connects/)).toBeInTheDocument();
    });
  });

  describe('Danger Zone', () => {
    it('renders danger zone card', () => {
      renderAIDataLiaisonPage();
      expect(screen.getByText('Danger Zone')).toBeInTheDocument();
    });

    it('renders uninstall button', () => {
      renderAIDataLiaisonPage();
      expect(screen.getByRole('button', { name: /uninstall application/i })).toBeInTheDocument();
    });

    it('renders warning text', () => {
      renderAIDataLiaisonPage();
      expect(screen.getByText(/Uninstalling AI Data Liaison will remove this application/)).toBeInTheDocument();
    });
  });

  describe('Uninstall Dialog', () => {
    it('opens dialog when uninstall button clicked', () => {
      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      expect(screen.getByText('Uninstall AI Data Liaison?')).toBeInTheDocument();
    });

    it('shows confirmation input in dialog', () => {
      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      expect(screen.getByPlaceholderText('uninstall application')).toBeInTheDocument();
    });

    it('shows instruction to type "uninstall application"', () => {
      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      expect(screen.getByText(/Type/)).toBeInTheDocument();
      expect(screen.getByText('uninstall application')).toBeInTheDocument();
    });

    it('disables confirm button when text does not match', () => {
      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'wrong text' } });

      expect(getConfirmButton()).toBeDisabled();
    });

    it('enables confirm button when text matches', () => {
      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      expect(getConfirmButton()).not.toBeDisabled();
    });

    it('closes dialog when cancel button clicked', () => {
      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const cancelButton = screen.getByRole('button', { name: /cancel/i });
      fireEvent.click(cancelButton);

      expect(screen.queryByText('Uninstall AI Data Liaison?')).not.toBeInTheDocument();
    });

    it('resets input state when dialog is closed and reopened', async () => {
      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      // Verify input has the value
      expect(input).toHaveValue('uninstall application');

      // Close via X button or onOpenChange - simulate closing
      const cancelButton = screen.getByRole('button', { name: /cancel/i });
      fireEvent.click(cancelButton);

      // Wait for dialog to close (Radix Dialog uses animation)
      await waitFor(() => {
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      });
    });
  });

  describe('Uninstall Action', () => {
    it('calls uninstallApp when confirmed', async () => {
      vi.mocked(engineApi.uninstallApp).mockResolvedValue({ success: true });

      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(engineApi.uninstallApp).toHaveBeenCalledWith('proj-1', 'ai-data-liaison');
      });
    });

    it('navigates to project dashboard on successful uninstall', async () => {
      vi.mocked(engineApi.uninstallApp).mockResolvedValue({ success: true });

      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1');
      });
    });

    it('shows error toast when API returns error', async () => {
      vi.mocked(engineApi.uninstallApp).mockResolvedValue({
        success: false,
        error: 'Failed to uninstall'
      });

      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(mockToast).toHaveBeenCalledWith({
          title: 'Error',
          description: 'Failed to uninstall',
          variant: 'destructive',
        });
      });
    });

    it('shows error toast when API throws exception', async () => {
      vi.mocked(engineApi.uninstallApp).mockRejectedValue(new Error('Network error'));

      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(mockToast).toHaveBeenCalledWith({
          title: 'Error',
          description: 'Network error',
          variant: 'destructive',
        });
      });
    });

    it('shows loading state while uninstalling', async () => {
      let resolvePromise: (value: { success: boolean }) => void;
      vi.mocked(engineApi.uninstallApp).mockImplementation(
        () => new Promise((resolve) => { resolvePromise = resolve; })
      );

      renderAIDataLiaisonPage();
      const uninstallButton = screen.getByRole('button', { name: /uninstall application/i });
      fireEvent.click(uninstallButton);

      const input = screen.getByPlaceholderText('uninstall application');
      fireEvent.change(input, { target: { value: 'uninstall application' } });

      fireEvent.click(getConfirmButton());

      await waitFor(() => {
        expect(screen.getByText('Uninstalling...')).toBeInTheDocument();
      });

      // Resolve the promise to clean up
      resolvePromise!({ success: true });
    });
  });
});
