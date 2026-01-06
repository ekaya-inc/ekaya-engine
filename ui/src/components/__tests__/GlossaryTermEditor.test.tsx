import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import type { GlossaryTerm, TestSQLResult } from '../../types';
import { GlossaryTermEditor } from '../GlossaryTermEditor';

// Mock the engineApi module
vi.mock('../../services/engineApi', () => ({
  default: {
    testGlossarySQL: vi.fn(),
    createGlossaryTerm: vi.fn(),
    updateGlossaryTerm: vi.fn(),
  },
}));

// Mock the SqlEditor component to render a simple textarea
vi.mock('../SqlEditor', () => ({
  SqlEditor: ({
    value,
    onChange,
    placeholder,
    readOnly,
  }: {
    value: string;
    onChange: (value: string) => void;
    placeholder?: string;
    readOnly?: boolean;
  }) => (
    <textarea
      data-testid="sql-editor"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      readOnly={readOnly}
    />
  ),
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

const mockTerm: GlossaryTerm = {
  id: 'term-1',
  project_id: 'proj-1',
  term: 'Active Users',
  definition: 'Users who have logged in within the last 30 days',
  defining_sql: 'SELECT COUNT(DISTINCT user_id) AS active_users FROM users WHERE last_login > NOW() - INTERVAL \'30 days\'',
  base_table: 'users',
  output_columns: [
    { name: 'active_users', type: 'integer', description: 'Count of active users' },
  ],
  aliases: ['MAU', 'Monthly Active Users'],
  source: 'manual',
  created_at: '2024-01-15T00:00:00Z',
  updated_at: '2024-01-15T00:00:00Z',
};

describe('GlossaryTermEditor', () => {
  const mockOnClose = vi.fn();
  const mockOnSave = vi.fn();
  const projectId = 'proj-1';

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('Create Mode', () => {
    it('renders with empty form in create mode', () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.getByText('Add Glossary Term')).toBeInTheDocument();
      expect(screen.getByLabelText(/term name/i)).toHaveValue('');
      expect(screen.getByLabelText(/definition/i)).toHaveValue('');
      expect(screen.getByTestId('sql-editor')).toHaveValue('');
    });

    it('shows correct dialog description in create mode', () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.getByText(/define a new business term/i)).toBeInTheDocument();
    });

    it('shows Create Term button in create mode', () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.getByRole('button', { name: /create term/i })).toBeInTheDocument();
    });
  });

  describe('Edit Mode', () => {
    it('renders with pre-filled form in edit mode', () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          term={mockTerm}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.getByText('Edit Glossary Term')).toBeInTheDocument();
      expect(screen.getByLabelText(/term name/i)).toHaveValue('Active Users');
      expect(screen.getByLabelText(/definition/i)).toHaveValue('Users who have logged in within the last 30 days');
      expect(screen.getByTestId('sql-editor')).toHaveValue(mockTerm.defining_sql);
      expect(screen.getByLabelText(/base table/i)).toHaveValue('users');
    });

    it('shows existing aliases in edit mode', () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          term={mockTerm}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.getByText('MAU')).toBeInTheDocument();
      expect(screen.getByText('Monthly Active Users')).toBeInTheDocument();
    });

    it('shows Update Term button in edit mode', () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          term={mockTerm}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.getByRole('button', { name: /update term/i })).toBeInTheDocument();
    });

    it('enables save button for existing terms with valid SQL', async () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          term={mockTerm}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      // Wait for form to initialize
      await waitFor(() => {
        expect(screen.getByDisplayValue('Active Users')).toBeInTheDocument();
      });

      const saveButton = screen.getByRole('button', { name: /update term/i });
      // Existing terms should have their save button enabled (SQL already tested)
      expect(saveButton).toBeEnabled();
    });
  });

  describe('Form Validation', () => {
    it('disables save button when term name is empty', () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const saveButton = screen.getByRole('button', { name: /create term/i });
      expect(saveButton).toBeDisabled();
    });

    it('disables save button when SQL is not tested', async () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const termInput = screen.getByLabelText(/term name/i);
      const definitionInput = screen.getByLabelText(/definition/i);
      const sqlEditor = screen.getByTestId('sql-editor');

      await userEvent.type(termInput, 'Test Term');
      await userEvent.type(definitionInput, 'Test definition');
      await userEvent.type(sqlEditor, 'SELECT * FROM users');

      const saveButton = screen.getByRole('button', { name: /create term/i });
      expect(saveButton).toBeDisabled();
    });

    it('enables save button after successful SQL test', async () => {
      const testResult: TestSQLResult = {
        valid: true,
        output_columns: [{ name: 'user_id', type: 'integer', description: '' }],
      };

      vi.mocked(engineApi.testGlossarySQL).mockResolvedValue({
        success: true,
        data: testResult,
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const termInput = screen.getByLabelText(/term name/i);
      const definitionInput = screen.getByLabelText(/definition/i);
      const sqlEditor = screen.getByTestId('sql-editor');

      await userEvent.type(termInput, 'Test Term');
      await userEvent.type(definitionInput, 'Test definition');
      await userEvent.type(sqlEditor, 'SELECT * FROM users');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        const saveButton = screen.getByRole('button', { name: /create term/i });
        expect(saveButton).toBeEnabled();
      });
    });
  });

  describe('SQL Testing', () => {
    it('disables test button when SQL is empty', () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const testButton = screen.getByRole('button', { name: /test sql/i });
      expect(testButton).toBeDisabled();
    });

    it('calls testGlossarySQL API when Test SQL button clicked', async () => {
      const testResult: TestSQLResult = {
        valid: true,
        output_columns: [{ name: 'user_id', type: 'integer', description: '' }],
      };

      vi.mocked(engineApi.testGlossarySQL).mockResolvedValue({
        success: true,
        data: testResult,
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const sqlEditor = screen.getByTestId('sql-editor');
      await userEvent.type(sqlEditor, 'SELECT * FROM users');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        expect(engineApi.testGlossarySQL).toHaveBeenCalledWith(projectId, 'SELECT * FROM users');
      });
    });

    it('shows loading state while testing SQL', async () => {
      let resolveTest: (value: unknown) => void;
      const testPromise = new Promise((resolve) => {
        resolveTest = resolve;
      });

      vi.mocked(engineApi.testGlossarySQL).mockReturnValue(testPromise as never);

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const sqlEditor = screen.getByTestId('sql-editor');
      await userEvent.type(sqlEditor, 'SELECT * FROM users');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        expect(screen.getByText('Testing...')).toBeInTheDocument();
      });

      resolveTest!({
        success: true,
        data: { valid: true, output_columns: [] },
      });
    });

    it('displays success message when SQL is valid', async () => {
      const testResult: TestSQLResult = {
        valid: true,
        output_columns: [{ name: 'user_id', type: 'integer', description: '' }],
      };

      vi.mocked(engineApi.testGlossarySQL).mockResolvedValue({
        success: true,
        data: testResult,
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const sqlEditor = screen.getByTestId('sql-editor');
      await userEvent.type(sqlEditor, 'SELECT * FROM users');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        expect(screen.getByText(/sql is valid/i)).toBeInTheDocument();
      });
    });

    it('displays error message when SQL is invalid', async () => {
      const testResult: TestSQLResult = {
        valid: false,
        error: 'Syntax error near SELECT',
      };

      vi.mocked(engineApi.testGlossarySQL).mockResolvedValue({
        success: true,
        data: testResult,
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const sqlEditor = screen.getByTestId('sql-editor');
      await userEvent.type(sqlEditor, 'INVALID SQL');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        expect(screen.getByText(/syntax error near select/i)).toBeInTheDocument();
      });
    });

    it('displays output columns after successful test', async () => {
      const testResult: TestSQLResult = {
        valid: true,
        output_columns: [
          { name: 'user_id', type: 'integer', description: '' },
          { name: 'email', type: 'varchar', description: '' },
        ],
      };

      vi.mocked(engineApi.testGlossarySQL).mockResolvedValue({
        success: true,
        data: testResult,
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const sqlEditor = screen.getByTestId('sql-editor');
      await userEvent.type(sqlEditor, 'SELECT user_id, email FROM users');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        expect(screen.getByText('Output Columns')).toBeInTheDocument();
        expect(screen.getByText('user_id')).toBeInTheDocument();
        expect(screen.getByText('(integer)')).toBeInTheDocument();
        expect(screen.getByText('email')).toBeInTheDocument();
        expect(screen.getByText('(varchar)')).toBeInTheDocument();
      });
    });

    it('resets test state when SQL is modified', async () => {
      const testResult: TestSQLResult = {
        valid: true,
        output_columns: [{ name: 'user_id', type: 'integer', description: '' }],
      };

      vi.mocked(engineApi.testGlossarySQL).mockResolvedValue({
        success: true,
        data: testResult,
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const sqlEditor = screen.getByTestId('sql-editor');
      await userEvent.type(sqlEditor, 'SELECT * FROM users');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        expect(screen.getByText(/sql is valid/i)).toBeInTheDocument();
      });

      // Modify SQL
      await userEvent.clear(sqlEditor);
      await userEvent.type(sqlEditor, 'SELECT * FROM orders');

      // Valid indicator should disappear
      await waitFor(() => {
        expect(screen.queryByText(/sql is valid/i)).not.toBeInTheDocument();
      });
    });
  });

  describe('Alias Management', () => {
    it('adds alias when Add button clicked', async () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const aliasInput = screen.getByLabelText(/aliases/i);
      await userEvent.type(aliasInput, 'MAU');

      const addButton = screen.getByRole('button', { name: /add/i });
      fireEvent.click(addButton);

      expect(screen.getByText('MAU')).toBeInTheDocument();
    });

    it('adds alias when Enter key pressed', async () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const aliasInput = screen.getByLabelText(/aliases/i);
      await userEvent.type(aliasInput, 'MAU{Enter}');

      expect(screen.getByText('MAU')).toBeInTheDocument();
    });

    it('removes alias when X button clicked', async () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          term={mockTerm}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(screen.getByText('MAU')).toBeInTheDocument();

      const removeButtons = screen.getAllByRole('button').filter(
        (btn) => btn.querySelector('svg')?.classList.contains('lucide-x')
      );

      if (removeButtons.length > 0) {
        fireEvent.click(removeButtons[0]!);
      }

      await waitFor(() => {
        expect(screen.queryByText('MAU')).not.toBeInTheDocument();
      });
    });

    it('prevents duplicate aliases', async () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const aliasInput = screen.getByLabelText(/aliases/i);
      await userEvent.type(aliasInput, 'MAU');

      const addButton = screen.getByRole('button', { name: /add/i });
      fireEvent.click(addButton);

      await userEvent.type(aliasInput, 'MAU');
      fireEvent.click(addButton);

      const mauElements = screen.getAllByText('MAU');
      expect(mauElements).toHaveLength(1);
    });

    it('clears alias input after adding', async () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const aliasInput = screen.getByLabelText(/aliases/i) as HTMLInputElement;
      await userEvent.type(aliasInput, 'MAU');

      const addButton = screen.getByRole('button', { name: /add/i });
      fireEvent.click(addButton);

      expect(aliasInput.value).toBe('');
    });
  });

  describe('Create Term', () => {
    it('calls createGlossaryTerm API on save', async () => {
      const testResult: TestSQLResult = {
        valid: true,
        output_columns: [{ name: 'user_id', type: 'integer', description: '' }],
      };

      vi.mocked(engineApi.testGlossarySQL).mockResolvedValue({
        success: true,
        data: testResult,
      });

      vi.mocked(engineApi.createGlossaryTerm).mockResolvedValue({
        success: true,
        data: { ...mockTerm, id: 'new-term-1' },
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const termInput = screen.getByLabelText(/term name/i);
      const definitionInput = screen.getByLabelText(/definition/i);
      const sqlEditor = screen.getByTestId('sql-editor');

      await userEvent.type(termInput, 'New Term');
      await userEvent.type(definitionInput, 'New definition');
      await userEvent.type(sqlEditor, 'SELECT * FROM users');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        expect(screen.getByText(/sql is valid/i)).toBeInTheDocument();
      });

      const saveButton = screen.getByRole('button', { name: /create term/i });
      fireEvent.click(saveButton);

      await waitFor(() => {
        expect(engineApi.createGlossaryTerm).toHaveBeenCalledWith(projectId, {
          term: 'New Term',
          definition: 'New definition',
          defining_sql: 'SELECT * FROM users',
        });
      });
    });

    it('calls onSave and onClose after successful create', async () => {
      const testResult: TestSQLResult = {
        valid: true,
        output_columns: [],
      };

      vi.mocked(engineApi.testGlossarySQL).mockResolvedValue({
        success: true,
        data: testResult,
      });

      vi.mocked(engineApi.createGlossaryTerm).mockResolvedValue({
        success: true,
        data: mockTerm,
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const termInput = screen.getByLabelText(/term name/i);
      const definitionInput = screen.getByLabelText(/definition/i);
      const sqlEditor = screen.getByTestId('sql-editor');

      await userEvent.type(termInput, 'New Term');
      await userEvent.type(definitionInput, 'New definition');
      await userEvent.type(sqlEditor, 'SELECT * FROM users');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        expect(screen.getByText(/sql is valid/i)).toBeInTheDocument();
      });

      const saveButton = screen.getByRole('button', { name: /create term/i });
      fireEvent.click(saveButton);

      await waitFor(() => {
        expect(mockOnSave).toHaveBeenCalled();
        expect(mockOnClose).toHaveBeenCalled();
      });
    });

    it('displays error message on create failure', async () => {
      const testResult: TestSQLResult = {
        valid: true,
        output_columns: [],
      };

      vi.mocked(engineApi.testGlossarySQL).mockResolvedValue({
        success: true,
        data: testResult,
      });

      vi.mocked(engineApi.createGlossaryTerm).mockResolvedValue({
        success: false,
        error: 'Duplicate term name',
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const termInput = screen.getByLabelText(/term name/i);
      const definitionInput = screen.getByLabelText(/definition/i);
      const sqlEditor = screen.getByTestId('sql-editor');

      await userEvent.type(termInput, 'Duplicate Term');
      await userEvent.type(definitionInput, 'Test definition');
      await userEvent.type(sqlEditor, 'SELECT * FROM users');

      const testButton = screen.getByRole('button', { name: /test sql/i });
      fireEvent.click(testButton);

      await waitFor(() => {
        expect(screen.getByText(/sql is valid/i)).toBeInTheDocument();
      });

      const saveButton = screen.getByRole('button', { name: /create term/i });
      fireEvent.click(saveButton);

      await waitFor(() => {
        expect(screen.getByText(/duplicate term name/i)).toBeInTheDocument();
      });

      expect(mockOnSave).not.toHaveBeenCalled();
      expect(mockOnClose).not.toHaveBeenCalled();
    });
  });

  describe('Update Term', () => {
    it('calls updateGlossaryTerm API on save', async () => {
      vi.mocked(engineApi.updateGlossaryTerm).mockResolvedValue({
        success: true,
        data: mockTerm,
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          term={mockTerm}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      // Wait for form to initialize (term name should be populated)
      await waitFor(() => {
        expect(screen.getByDisplayValue('Active Users')).toBeInTheDocument();
      });

      const definitionInput = screen.getByLabelText(/definition/i);
      await userEvent.clear(definitionInput);
      await userEvent.type(definitionInput, 'Updated definition');

      const saveButton = screen.getByRole('button', { name: /update term/i });
      fireEvent.click(saveButton);

      await waitFor(() => {
        expect(engineApi.updateGlossaryTerm).toHaveBeenCalledWith(
          projectId,
          mockTerm.id,
          expect.objectContaining({
            definition: 'Updated definition',
          })
        );
      });
    });

    it('calls onSave and onClose after successful update', async () => {
      vi.mocked(engineApi.updateGlossaryTerm).mockResolvedValue({
        success: true,
        data: mockTerm,
      });

      render(
        <GlossaryTermEditor
          projectId={projectId}
          term={mockTerm}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      // Wait for form to initialize (term name should be populated)
      await waitFor(() => {
        expect(screen.getByDisplayValue('Active Users')).toBeInTheDocument();
      });

      const saveButton = screen.getByRole('button', { name: /update term/i });
      fireEvent.click(saveButton);

      await waitFor(() => {
        expect(mockOnSave).toHaveBeenCalled();
        expect(mockOnClose).toHaveBeenCalled();
      });
    });
  });

  describe('Dialog Controls', () => {
    it('calls onClose when Cancel button clicked', () => {
      render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      const cancelButton = screen.getByRole('button', { name: /cancel/i });
      fireEvent.click(cancelButton);

      expect(mockOnClose).toHaveBeenCalled();
    });

    it('does not render when isOpen is false', () => {
      const { container } = render(
        <GlossaryTermEditor
          projectId={projectId}
          isOpen={false}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      expect(container.firstChild).toBeNull();
    });

    it('disables form controls while saving', async () => {
      let resolveSave: (value: unknown) => void;
      const savePromise = new Promise((resolve) => {
        resolveSave = resolve;
      });

      vi.mocked(engineApi.updateGlossaryTerm).mockReturnValue(savePromise as never);

      render(
        <GlossaryTermEditor
          projectId={projectId}
          term={mockTerm}
          isOpen={true}
          onClose={mockOnClose}
          onSave={mockOnSave}
        />
      );

      // Wait for form to initialize (term name should be populated)
      await waitFor(() => {
        expect(screen.getByDisplayValue('Active Users')).toBeInTheDocument();
      });

      const saveButton = screen.getByRole('button', { name: /update term/i });
      fireEvent.click(saveButton);

      await waitFor(() => {
        expect(screen.getByText('Saving...')).toBeInTheDocument();
        expect(screen.getByLabelText(/term name/i)).toBeDisabled();
        expect(screen.getByLabelText(/definition/i)).toBeDisabled();
      });

      resolveSave!({ success: true, data: mockTerm });
    });
  });
});
