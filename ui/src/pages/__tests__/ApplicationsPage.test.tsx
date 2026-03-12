import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ProjectProvider } from '../../contexts/ProjectContext';
import ApplicationsPage from '../ApplicationsPage';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

const mockRefetch = vi.fn().mockResolvedValue(undefined);
const mockInstall = vi.fn();
let mockInstalledApps: string[] = [];

vi.mock('../../hooks/useInstalledApps', () => ({
  useInstalledApps: () => ({
    apps: mockInstalledApps.map((id) => ({ app_id: id })),
    isLoading: false,
    error: null,
    refetch: mockRefetch,
    isInstalled: (appId: string) => mockInstalledApps.includes(appId),
  }),
  useInstallApp: () => ({
    install: mockInstall,
    isLoading: false,
    error: null,
  }),
}));

describe('ApplicationsPage', () => {
  const mockClick = vi.fn();
  let capturedHref = '';
  const originalCreateElement = document.createElement.bind(document);

  beforeEach(() => {
    vi.clearAllMocks();
    capturedHref = '';
    mockInstalledApps = [];

    vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
      if (tagName === 'a') {
        const mockAnchor = {
          href: '',
          click: () => {
            capturedHref = mockAnchor.href;
            mockClick();
          },
        };
        return mockAnchor as unknown as HTMLAnchorElement;
      }
      return originalCreateElement(tagName);
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const renderPage = () => {
    return render(
      <ProjectProvider>
        <MemoryRouter initialEntries={['/projects/proj-1/applications']}>
          <Routes>
            <Route path="/projects/:pid/applications" element={<ApplicationsPage />} />
          </Routes>
        </MemoryRouter>
      </ProjectProvider>
    );
  };

  it('renders page header with correct title', () => {
    renderPage();

    expect(screen.getByText('Applications')).toBeInTheDocument();
    expect(screen.getByText('Choose an application to add to your project')).toBeInTheDocument();
  });

  it('renders all application tiles', () => {
    renderPage();

    expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
    expect(screen.getByText(/governed access to glossary terms, approved queries, and collaboration workflows/i)).toBeInTheDocument();
    expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
    expect(screen.getByText('AI Agents')).toBeInTheDocument();
    expect(screen.getByText('MCP Tunnel')).toBeInTheDocument();
    expect(screen.getByText('Product Kit [COMING SOON]')).toBeInTheDocument();
    expect(screen.getByText('On-Premise Chat [COMING SOON]')).toBeInTheDocument();
    expect(screen.getByText('Your own Data Application')).toBeInTheDocument();
  });

  it('renders Contact Sales buttons for Product Kit and On-Premise Chat', () => {
    renderPage();

    const contactSalesButtons = screen.getAllByRole('button', { name: 'Contact Sales' });
    expect(contactSalesButtons).toHaveLength(2);
  });

  it('renders Install buttons for installable apps when not installed', () => {
    renderPage();

    const installButtons = screen.getAllByRole('button', { name: 'Install' });
    expect(installButtons).toHaveLength(5);

    const learnMoreButtons = screen.getAllByRole('button', { name: /Learn More/i });
    expect(learnMoreButtons).toHaveLength(4);
  });

  it('renders Installed badge, Learn More, and Configure button when AI Data Liaison is installed', () => {
    mockInstalledApps = ['ai-data-liaison'];
    renderPage();

    expect(screen.getByText('Installed')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Configure' })).toBeInTheDocument();

    const learnMoreButtons = screen.getAllByRole('button', { name: /Learn More/i });
    expect(learnMoreButtons.length).toBeGreaterThanOrEqual(3);

    const installButtons = screen.getAllByRole('button', { name: 'Install' });
    expect(installButtons).toHaveLength(4);
  });

  it('disables AI Data Liaison Install button when Ontology Forge is not installed', () => {
    renderPage();

    const installButtons = screen.getAllByRole('button', { name: 'Install' });
    expect(installButtons[1]).toBeDisabled();
    expect(installButtons[2]).toBeDisabled();

    const requiresNotes = screen.getAllByText(/Requires Ontology Forge/);
    expect(requiresNotes).toHaveLength(2);
  });

  it('enables AI Data Liaison Install button when Ontology Forge is installed', async () => {
    mockInstalledApps = ['ontology-forge'];
    mockInstall.mockResolvedValue({ id: 'test-id', app_id: 'ai-data-liaison' });
    renderPage();

    expect(screen.queryByText(/Requires Ontology Forge/)).not.toBeInTheDocument();

    const installButtons = screen.getAllByRole('button', { name: 'Install' });
    expect(installButtons[0]).not.toBeDisabled();
    fireEvent.click(installButtons[0] as HTMLElement);

    await waitFor(() => {
      expect(mockInstall).toHaveBeenCalledWith('ai-data-liaison');
    });
  });

  it('navigates directly to MCP Tunnel after installing it', async () => {
    mockInstall.mockResolvedValue({ id: 'test-id', app_id: 'mcp-tunnel' });
    renderPage();

    const tunnelCard = screen.getByTestId('app-card-mcp-tunnel');
    fireEvent.click(within(tunnelCard).getByRole('button', { name: 'Install' }));

    await waitFor(() => {
      expect(mockInstall).toHaveBeenCalledWith('mcp-tunnel');
    });
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1/mcp-tunnel');
    });
  });

  it('navigates to config page when clicking Configure on installed AI Data Liaison', () => {
    mockInstalledApps = ['ai-data-liaison'];
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: 'Configure' }));
    expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1/ai-data-liaison');
  });

  it('opens mailto link when clicking Contact Sales on Product Kit', () => {
    renderPage();

    const contactSalesButtons = screen.getAllByRole('button', { name: 'Contact Sales' });
    fireEvent.click(contactSalesButtons[0] as HTMLElement);

    expect(mockClick).toHaveBeenCalled();
    expect(capturedHref).toBe(
      'mailto:sales@ekaya.ai?subject=Interest%20in%20Product%20Kit%20%5BCOMING%20SOON%5D%20for%20my%20Ekaya%20project'
    );
  });

  it('opens mailto link when clicking Contact Sales on On-Premise Chat', () => {
    renderPage();

    const contactSalesButtons = screen.getAllByRole('button', { name: 'Contact Sales' });
    fireEvent.click(contactSalesButtons[1] as HTMLElement);

    expect(mockClick).toHaveBeenCalled();
    expect(capturedHref).toBe(
      'mailto:sales@ekaya.ai?subject=Interest%20in%20On-Premise%20Chat%20%5BCOMING%20SOON%5D%20for%20my%20Ekaya%20project'
    );
  });

  it('renders Contact Support button on Build Your Own tile', () => {
    renderPage();

    expect(screen.getByTestId('app-card-build-your-own')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Contact Support' })).toBeInTheDocument();
  });

  it('navigates back when clicking back button', () => {
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: 'Back to project dashboard' }));
    expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1');
  });

  it('opens mailto link when clicking Contact Support on Build Your Own tile', () => {
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: 'Contact Support' }));

    expect(mockClick).toHaveBeenCalled();
    expect(capturedHref).toBe(
      'mailto:support@ekaya.ai?subject=Interest%20in%20building%20a%20custom%20data%20application%20on%20Ekaya'
    );
  });

  it('opens Learn More link in new tab', () => {
    const mockOpen = vi.fn();
    vi.spyOn(window, 'open').mockImplementation(mockOpen);

    renderPage();

    const learnMoreButtons = screen.getAllByRole('button', { name: /Learn More/i });
    fireEvent.click(learnMoreButtons[0] as HTMLElement);
    expect(mockOpen).toHaveBeenCalledWith(
      'https://us.ekaya.ai/apps/ontology-forge',
      '_blank',
      'noopener,noreferrer'
    );

    fireEvent.click(learnMoreButtons[1] as HTMLElement);
    expect(mockOpen).toHaveBeenCalledWith(
      'https://us.ekaya.ai/apps/ai-data-liaison',
      '_blank',
      'noopener,noreferrer'
    );
  });
});
