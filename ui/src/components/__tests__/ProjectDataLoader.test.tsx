import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ProjectDataLoader from '../ProjectDataLoader';

const mockLoadDataSources = vi.fn();
const mockSetProjectInfo = vi.fn();
const mockClearProjectInfo = vi.fn();
const mockGetProject = vi.fn();
const mockProvisionProject = vi.fn();

vi.mock('../../contexts/DatasourceConnectionContext', () => ({
  useDatasourceConnection: () => ({
    loadDataSources: mockLoadDataSources,
  }),
}));

vi.mock('../../contexts/ProjectContext', () => ({
  useProject: () => ({
    setProjectInfo: mockSetProjectInfo,
    clearProjectInfo: mockClearProjectInfo,
  }),
}));

vi.mock('../../services/provision', () => ({
  getProject: (...args: unknown[]) => mockGetProject(...args),
  provisionProject: (...args: unknown[]) => mockProvisionProject(...args),
}));

const renderLoader = () =>
  render(
    <MemoryRouter initialEntries={['/projects/proj-1']}>
      <Routes>
        <Route
          path="/projects/:pid"
          element={
            <ProjectDataLoader>
              <div>Project ready</div>
            </ProjectDataLoader>
          }
        />
      </Routes>
    </MemoryRouter>
  );

describe('ProjectDataLoader', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('marks the project as just provisioned when POST /projects creates it', async () => {
    mockGetProject.mockRejectedValue(new Error('Project not found'));
    mockProvisionProject.mockResolvedValue({
      status: 'success',
      pid: 'proj-1',
      name: 'Provisioned Project',
      created: true,
      applications: ['mcp-server', 'ai-data-liaison'],
    });

    renderLoader();

    await waitFor(() => {
      expect(mockSetProjectInfo).toHaveBeenCalledWith(
        'proj-1',
        'Provisioned Project',
        {},
        {
          justProvisioned: true,
          assignedAppIds: ['mcp-server', 'ai-data-liaison'],
        }
      );
    });

    expect(mockLoadDataSources).toHaveBeenCalledWith('proj-1');
    expect(screen.getByText('Project ready')).toBeInTheDocument();
  });

  it('does not mark existing projects as just provisioned on GET /projects', async () => {
    mockGetProject.mockResolvedValue({
      status: 'success',
      pid: 'proj-1',
      name: 'Existing Project',
      applications: ['mcp-server'],
    });

    renderLoader();

    await waitFor(() => {
      expect(mockSetProjectInfo).toHaveBeenCalledWith(
        'proj-1',
        'Existing Project',
        {},
        {
          justProvisioned: false,
          assignedAppIds: ['mcp-server'],
        }
      );
    });

    expect(mockProvisionProject).not.toHaveBeenCalled();
  });
});
