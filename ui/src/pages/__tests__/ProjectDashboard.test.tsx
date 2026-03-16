import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useEffect } from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ProjectDashboard from '../ProjectDashboard';

const mockNavigate = vi.fn();
let mockInstalledApps: string[] = [];

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock('../../contexts/DatasourceConnectionContext', () => ({
  useDatasourceConnection: () => ({
    isConnected: true,
    hasSelectedTables: true,
  }),
}));

vi.mock('../../hooks/useInstalledApps', () => ({
  useInstalledApps: () => ({
    apps: mockInstalledApps.map((appId) => ({ app_id: appId })),
  }),
}));

vi.mock('../../services/ontologyService', () => ({
  ontologyService: {
    subscribe: vi.fn(() => () => {}),
  },
}));

vi.mock('../../components/AIConfigWidget', () => ({
  default: function AIConfigWidgetMock(
    { onConfigChange }: { onConfigChange: (option: 'byok') => void }
  ) {
    useEffect(() => {
      onConfigChange('byok');
    }, [onConfigChange]);

    return <div data-testid="ai-config-widget" />;
  },
}));

describe('ProjectDashboard', () => {
  const renderPage = () => render(
    <MemoryRouter initialEntries={['/projects/proj-1']}>
      <Routes>
        <Route path="/projects/:pid" element={<ProjectDashboard />} />
      </Routes>
    </MemoryRouter>,
  );

  beforeEach(() => {
    vi.clearAllMocks();
    mockInstalledApps = ['ontology-forge'];
  });

  it('does not show the Glossary tile when AI Data Liaison is not installed', async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Ontology Extraction')).toBeInTheDocument();
    });

    expect(screen.queryByText('Glossary')).not.toBeInTheDocument();
  });

  it('shows the Glossary tile when AI Data Liaison is installed', async () => {
    mockInstalledApps = ['ontology-forge', 'ai-data-liaison'];
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Glossary')).toBeInTheDocument();
    });
  });

  it('navigates to the glossary page from the AI Data Liaison tile', async () => {
    mockInstalledApps = ['ontology-forge', 'ai-data-liaison'];
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Glossary')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Glossary'));

    expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1/glossary');
  });
});
