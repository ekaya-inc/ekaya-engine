import { render, screen } from '@testing-library/react';
import { useEffect } from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';

import { ProjectProvider, useProject } from '../../contexts/ProjectContext';
import ProjectRouteGate from '../ProjectRouteGate';

function ProjectContextHarness({
  justProvisioned,
}: {
  justProvisioned: boolean;
}) {
  const { setProjectInfo } = useProject();

  useEffect(() => {
    setProjectInfo(
      'proj-1',
      'Wizard Project',
      {},
      {
        justProvisioned,
        assignedAppIds: ['mcp-server'],
      }
    );
  }, [justProvisioned, setProjectInfo]);

  return null;
}

const renderRouteGate = ({
  initialPath,
  justProvisioned,
}: {
  initialPath: string;
  justProvisioned: boolean;
}) =>
  render(
    <MemoryRouter initialEntries={[initialPath]}>
      <ProjectProvider>
        <ProjectContextHarness justProvisioned={justProvisioned} />
        <Routes>
          <Route path="/projects/:pid" element={<ProjectRouteGate />}>
            <Route index element={<div>Project home</div>} />
            <Route path="setup" element={<div>Project setup</div>} />
          </Route>
        </Routes>
      </ProjectProvider>
    </MemoryRouter>
  );

describe('ProjectRouteGate', () => {
  it('redirects newly provisioned projects into setup', async () => {
    renderRouteGate({
      initialPath: '/projects/proj-1',
      justProvisioned: true,
    });

    expect(await screen.findByText('Project setup')).toBeInTheDocument();
  });

  it('keeps setup directly accessible after provisioning flow is no longer active', async () => {
    renderRouteGate({
      initialPath: '/projects/proj-1/setup',
      justProvisioned: false,
    });

    expect(await screen.findByText('Project setup')).toBeInTheDocument();
  });
});
