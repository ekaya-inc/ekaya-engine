import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import AIConfigPage from '../AIConfigPage';

vi.mock('../../components/AIConfigWidget', () => ({
  default: ({ projectId }: { projectId: string }) => (
    <div data-testid="ai-config-widget">{projectId}</div>
  ),
}));

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock('../../contexts/ProjectContext', () => ({
  useProject: () => ({
    urls: { projectsPageUrl: 'https://us.ekaya.ai/projects' },
  }),
}));

function renderPage() {
  render(
    <MemoryRouter initialEntries={['/projects/proj-1/ai-config']}>
      <Routes>
        <Route path="/projects/:pid/ai-config" element={<AIConfigPage />} />
      </Routes>
    </MemoryRouter>
  );
}

describe('AIConfigPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders the shared page header and widget', () => {
    renderPage();

    expect(screen.getByText('AI Configuration')).toBeInTheDocument();
    expect(screen.getByText(/enable AI-powered features like ontology extraction, semantic search, and natural language querying/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /back to dashboard/i })).toBeInTheDocument();
    expect(screen.getByTestId('ai-config-widget')).toHaveTextContent('proj-1');
  });

  it('navigates back to the project dashboard', () => {
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: /back to dashboard/i }));
    expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1');
  });

  it('does not render the external info link', () => {
    renderPage();

    expect(screen.queryByTitle(/AI Configuration documentation/i)).not.toBeInTheDocument();
  });
});
