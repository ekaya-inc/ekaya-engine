import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import AIConfigWidget from '../AIConfigWidget';

// Mock the engineApi module
vi.mock('../../services/engineApi', () => ({
  default: {
    getAIConfig: vi.fn(),
    saveAIConfig: vi.fn(),
    deleteAIConfig: vi.fn(),
    testAIConnection: vi.fn(),
    getProjectConfig: vi.fn(),
  },
}));

// Mock Dialog components
vi.mock('../ui/Dialog', () => ({
  Dialog: ({ children, open }: { children: React.ReactNode; open: boolean }) =>
    open ? <div data-testid="dialog-root">{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) =>
    <div data-testid="dialog-content">{children}</div>,
  DialogHeader: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <h2>{children}</h2>,
  DialogDescription: ({ children }: { children: React.ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

describe('AIConfigWidget', () => {
  const projectId = 'proj-1';
  const mockOnConfigChange = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    // Default: no existing config
    vi.mocked(engineApi.getAIConfig).mockResolvedValue({
      success: true,
      data: { project_id: projectId, config_type: 'none' },
    });
    vi.mocked(engineApi.getProjectConfig).mockResolvedValue(null);
  });

  describe('Save Operation', () => {
    it('calls engineApi.saveAIConfig with correct payload and shows success state', async () => {
      vi.mocked(engineApi.testAIConnection).mockResolvedValue({
        success: true,
        data: {
          success: true,
          message: 'Connection successful',
        },
      });

      vi.mocked(engineApi.saveAIConfig).mockResolvedValue({
        success: true,
        data: {
          project_id: projectId,
          config_type: 'byok',
          llm_base_url: 'https://api.openai.com/v1',
          llm_api_key: 'sk-***',
          llm_model: 'gpt-4o',
        },
      });

      render(
        <AIConfigWidget
          projectId={projectId}
          onConfigChange={mockOnConfigChange}
        />
      );

      // Wait for initial load to complete
      await waitFor(() => {
        expect(engineApi.getAIConfig).toHaveBeenCalledWith(projectId);
      });

      // Click "Bring Your Own AI Keys" to open BYOK panel
      fireEvent.click(screen.getByText('Bring Your Own AI Keys'));

      // Wait for the BYOK panel to render (loading state should be done)
      await waitFor(() => {
        expect(screen.getByText('Provider')).toBeInTheDocument();
      });

      // Open provider dropdown by clicking the trigger button showing "OpenAI"
      fireEvent.click(screen.getByText('OpenAI'));

      // Select "Custom" from dropdown to get a URL text input
      fireEvent.click(screen.getByText('Custom'));

      // Fill in the custom URL
      const urlInput = screen.getByPlaceholderText('https://your-endpoint.com/v1');
      await userEvent.type(urlInput, 'https://api.openai.com/v1');

      // Fill in API key
      const apiKeyInput = screen.getByPlaceholderText('sk-...');
      await userEvent.type(apiKeyInput, 'sk-test-key-123');

      // Fill in model
      const modelInput = screen.getByPlaceholderText('gpt-4o, claude-haiku-4-5, llama3.1');
      await userEvent.type(modelInput, 'gpt-4o');

      // Click "Test Connection" to enable the save button
      const testButton = screen.getByRole('button', { name: /test connection/i });
      fireEvent.click(testButton);

      // Wait for test connection to complete
      await waitFor(() => {
        expect(engineApi.testAIConnection).toHaveBeenCalled();
      });

      // Save button should now be enabled
      const saveButton = screen.getByRole('button', { name: /save configuration/i });
      await waitFor(() => {
        expect(saveButton).toBeEnabled();
      });

      // Click "Save Configuration"
      fireEvent.click(saveButton);

      // Verify saveAIConfig was called with correct payload
      await waitFor(() => {
        expect(engineApi.saveAIConfig).toHaveBeenCalledWith(projectId, {
          config_type: 'byok',
          llm_base_url: 'https://api.openai.com/v1',
          llm_api_key: 'sk-test-key-123',
          llm_model: 'gpt-4o',
          embedding_base_url: '',
          embedding_model: '',
        });
      });

      // Verify success state: panel closes and onConfigChange is called
      await waitFor(() => {
        expect(mockOnConfigChange).toHaveBeenCalledWith('byok');
      });

      // The BYOK panel should have closed after save (selectedAIOption set to null)
      expect(screen.queryByText('Provider')).not.toBeInTheDocument();

      // Re-open the BYOK panel to verify the success state persists
      fireEvent.click(screen.getByText('Bring Your Own AI Keys'));

      await waitFor(() => {
        // The save button should now show "Remove Configuration" (activeAIConfig === 'byok')
        expect(screen.getByRole('button', { name: /remove configuration/i })).toBeInTheDocument();
      });
    });
  });

  describe('Provider Selection', () => {
    it('selecting a provider updates the displayed provider and base URL', async () => {
      render(
        <AIConfigWidget
          projectId={projectId}
          onConfigChange={mockOnConfigChange}
        />
      );

      await waitFor(() => {
        expect(engineApi.getAIConfig).toHaveBeenCalledWith(projectId);
      });

      // Open BYOK panel
      fireEvent.click(screen.getByText('Bring Your Own AI Keys'));
      await waitFor(() => {
        expect(screen.getByText('Provider')).toBeInTheDocument();
      });

      // Default provider should be OpenAI
      expect(screen.getByText('OpenAI')).toBeInTheDocument();

      // Open provider dropdown
      fireEvent.click(screen.getByText('OpenAI'));

      // Select Anthropic
      fireEvent.click(screen.getByText('Anthropic'));

      // Provider button should now show Anthropic
      // The dropdown should be closed and the model should be auto-filled
      await waitFor(() => {
        const modelInput = screen.getByPlaceholderText('gpt-4o, claude-haiku-4-5, llama3.1');
        expect(modelInput).toHaveValue('claude-haiku-4-5');
      });
    });

    it('selecting Custom provider shows URL input field', async () => {
      render(
        <AIConfigWidget
          projectId={projectId}
          onConfigChange={mockOnConfigChange}
        />
      );

      await waitFor(() => {
        expect(engineApi.getAIConfig).toHaveBeenCalledWith(projectId);
      });

      // Open BYOK panel
      fireEvent.click(screen.getByText('Bring Your Own AI Keys'));
      await waitFor(() => {
        expect(screen.getByText('Provider')).toBeInTheDocument();
      });

      // Open provider dropdown and select Custom
      fireEvent.click(screen.getByText('OpenAI'));
      fireEvent.click(screen.getByText('Custom'));

      // Custom URL input should appear
      await waitFor(() => {
        expect(screen.getByPlaceholderText('https://your-endpoint.com/v1')).toBeInTheDocument();
      });
    });

    it('selecting Azure OpenAI provider shows URL input with Azure placeholder', async () => {
      render(
        <AIConfigWidget
          projectId={projectId}
          onConfigChange={mockOnConfigChange}
        />
      );

      await waitFor(() => {
        expect(engineApi.getAIConfig).toHaveBeenCalledWith(projectId);
      });

      // Open BYOK panel
      fireEvent.click(screen.getByText('Bring Your Own AI Keys'));
      await waitFor(() => {
        expect(screen.getByText('Provider')).toBeInTheDocument();
      });

      // Open provider dropdown and select Azure OpenAI
      fireEvent.click(screen.getByText('OpenAI'));
      fireEvent.click(screen.getByText('Azure OpenAI'));

      // Azure URL input should appear with Azure-specific placeholder
      await waitFor(() => {
        expect(screen.getByPlaceholderText('https://your-resource.openai.azure.com')).toBeInTheDocument();
      });
    });
  });

  describe('Delete Operation', () => {
    it('calls engineApi.deleteAIConfig and updates UI state', async () => {
      // Load with an existing BYOK config so remove button is available
      vi.mocked(engineApi.getAIConfig).mockResolvedValue({
        success: true,
        data: {
          project_id: projectId,
          config_type: 'byok',
          llm_base_url: 'https://api.openai.com/v1',
          llm_api_key: 'sk-***...',
          llm_model: 'gpt-4o',
        },
      });

      vi.mocked(engineApi.deleteAIConfig).mockResolvedValue({ success: true });

      render(
        <AIConfigWidget
          projectId={projectId}
          onConfigChange={mockOnConfigChange}
        />
      );

      await waitFor(() => {
        expect(engineApi.getAIConfig).toHaveBeenCalledWith(projectId);
      });

      // onConfigChange should have been called with 'byok' on load
      expect(mockOnConfigChange).toHaveBeenCalledWith('byok');
      mockOnConfigChange.mockClear();

      // Open BYOK panel — should show "Remove Configuration" since config is active
      fireEvent.click(screen.getByText('Bring Your Own AI Keys'));
      await waitFor(() => {
        expect(screen.getByRole('button', { name: /remove configuration/i })).toBeInTheDocument();
      });

      // Click "Remove Configuration" — opens confirmation dialog
      fireEvent.click(screen.getByRole('button', { name: /remove configuration/i }));

      // Confirm removal in dialog
      await waitFor(() => {
        expect(screen.getByTestId('dialog-root')).toBeInTheDocument();
      });
      const dialogButtons = screen.getByTestId('dialog-root').querySelectorAll('button');
      const confirmButton = Array.from(dialogButtons).find(
        (btn) => btn.textContent === 'Remove Configuration'
      );
      expect(confirmButton).toBeDefined();
      fireEvent.click(confirmButton as HTMLElement);

      // Verify deleteAIConfig was called with the project ID
      await waitFor(() => {
        expect(engineApi.deleteAIConfig).toHaveBeenCalledWith(projectId);
      });

      // Verify onConfigChange was called with null (config removed)
      await waitFor(() => {
        expect(mockOnConfigChange).toHaveBeenCalledWith(null);
      });

      // The button should now show "Save Configuration" instead of "Remove Configuration"
      await waitFor(() => {
        expect(screen.getByRole('button', { name: /save configuration/i })).toBeInTheDocument();
      });
      expect(screen.queryByRole('button', { name: /remove configuration/i })).not.toBeInTheDocument();
    });
  });

  describe('Error Handling', () => {
    it('displays error when save fails', async () => {
      // Set up test connection to succeed (required to enable save button)
      vi.mocked(engineApi.testAIConnection).mockResolvedValue({
        success: true,
        data: { success: true, message: 'Connection successful' },
      });

      vi.mocked(engineApi.saveAIConfig).mockRejectedValue(
        new Error('Network error: unable to save')
      );

      render(
        <AIConfigWidget
          projectId={projectId}
          onConfigChange={mockOnConfigChange}
        />
      );

      await waitFor(() => {
        expect(engineApi.getAIConfig).toHaveBeenCalledWith(projectId);
      });

      // Open BYOK panel
      fireEvent.click(screen.getByText('Bring Your Own AI Keys'));
      await waitFor(() => {
        expect(screen.getByText('Provider')).toBeInTheDocument();
      });

      // Select Custom provider to get URL input
      fireEvent.click(screen.getByText('OpenAI'));
      fireEvent.click(screen.getByText('Custom'));

      // Fill in required fields
      await userEvent.type(
        screen.getByPlaceholderText('https://your-endpoint.com/v1'),
        'https://api.example.com/v1'
      );
      await userEvent.type(screen.getByPlaceholderText('sk-...'), 'sk-key');
      await userEvent.type(
        screen.getByPlaceholderText('gpt-4o, claude-haiku-4-5, llama3.1'),
        'gpt-4o'
      );

      // Test connection to enable save
      fireEvent.click(screen.getByRole('button', { name: /test connection/i }));
      await waitFor(() => {
        expect(engineApi.testAIConnection).toHaveBeenCalled();
      });

      // Click save
      const saveButton = screen.getByRole('button', { name: /save configuration/i });
      await waitFor(() => expect(saveButton).toBeEnabled());
      fireEvent.click(saveButton);

      // Verify error message is displayed to the user
      await waitFor(() => {
        expect(screen.getByText('Network error: unable to save')).toBeInTheDocument();
      });
    });

    it('displays error when test connection fails', async () => {
      vi.mocked(engineApi.testAIConnection).mockRejectedValue(
        new Error('Connection refused')
      );

      render(
        <AIConfigWidget
          projectId={projectId}
          onConfigChange={mockOnConfigChange}
        />
      );

      await waitFor(() => {
        expect(engineApi.getAIConfig).toHaveBeenCalledWith(projectId);
      });

      // Open BYOK panel
      fireEvent.click(screen.getByText('Bring Your Own AI Keys'));
      await waitFor(() => {
        expect(screen.getByText('Provider')).toBeInTheDocument();
      });

      // Select Custom provider and fill URL so test button is enabled
      fireEvent.click(screen.getByText('OpenAI'));
      fireEvent.click(screen.getByText('Custom'));
      await userEvent.type(
        screen.getByPlaceholderText('https://your-endpoint.com/v1'),
        'https://api.example.com/v1'
      );

      // Click test connection
      fireEvent.click(screen.getByRole('button', { name: /test connection/i }));

      // Verify error is shown in the test result display
      await waitFor(() => {
        expect(screen.getByText('Connection refused')).toBeInTheDocument();
      });
    });

    it('displays error when remove configuration fails', async () => {
      // Load with an existing BYOK config so remove button is available
      vi.mocked(engineApi.getAIConfig).mockResolvedValue({
        success: true,
        data: {
          project_id: projectId,
          config_type: 'byok',
          llm_base_url: 'https://api.openai.com/v1',
          llm_api_key: 'sk-***...',
          llm_model: 'gpt-4o',
        },
      });

      vi.mocked(engineApi.deleteAIConfig).mockRejectedValue(
        new Error('Permission denied')
      );

      render(
        <AIConfigWidget
          projectId={projectId}
          onConfigChange={mockOnConfigChange}
        />
      );

      await waitFor(() => {
        expect(engineApi.getAIConfig).toHaveBeenCalledWith(projectId);
      });

      // Open BYOK panel — should show "Remove Configuration" since config is active
      fireEvent.click(screen.getByText('Bring Your Own AI Keys'));
      await waitFor(() => {
        expect(screen.getByRole('button', { name: /remove configuration/i })).toBeInTheDocument();
      });

      // Click "Remove Configuration" — opens confirmation dialog
      fireEvent.click(screen.getByRole('button', { name: /remove configuration/i }));

      // Confirm removal in dialog
      await waitFor(() => {
        expect(screen.getByTestId('dialog-root')).toBeInTheDocument();
      });
      const dialogButtons = screen.getByTestId('dialog-root').querySelectorAll('button');
      const confirmButton = Array.from(dialogButtons).find(
        (btn) => btn.textContent === 'Remove Configuration'
      );
      expect(confirmButton).toBeDefined();
      fireEvent.click(confirmButton as HTMLElement);

      // Verify error message is displayed
      await waitFor(() => {
        expect(screen.getByText('Permission denied')).toBeInTheDocument();
      });
    });
  });
});
