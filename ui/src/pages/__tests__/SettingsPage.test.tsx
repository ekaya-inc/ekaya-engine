import { render, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import SettingsPage from '../SettingsPage';

vi.mock('../../services/engineApi', () => ({
  default: {
    completeDeleteCallback: vi.fn(),
    deleteProject: vi.fn(),
  },
}));

const mockToast = vi.fn();
vi.mock('../../hooks/useToast', () => ({
  useToast: () => ({
    toast: mockToast,
  }),
}));

vi.mock('../../components/ThemeProvider', () => ({
  useTheme: () => ({
    theme: 'light',
    setTheme: vi.fn(),
  }),
}));

vi.mock('../../contexts/ProjectContext', () => ({
  useProject: () => ({
    projectName: 'Test Project',
    urls: { projectsPageUrl: 'https://us.ekaya.ai/projects' },
  }),
}));

describe('SettingsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(engineApi.completeDeleteCallback).mockResolvedValue({
      success: true,
      data: { action: 'delete', status: 'cancelled' },
    });
  });

  it('completes cancelled delete callbacks and stays on the page', async () => {
    const initialHref = window.location.href;

    render(
      <MemoryRouter
        initialEntries={[
          '/projects/proj-1/settings?callback_action=delete&callback_state=test-state&callback_status=cancelled',
        ]}
      >
        <Routes>
          <Route path="/projects/:pid/settings" element={<SettingsPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(engineApi.completeDeleteCallback).toHaveBeenCalledWith(
        'proj-1',
        'delete',
        'cancelled',
        'test-state',
      );
    });

    expect(mockToast).toHaveBeenCalledWith({
      title: 'Deletion cancelled',
      description: 'Project was not deleted.',
    });
    expect(window.location.href).toBe(initialHref);
  });
});
