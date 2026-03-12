import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ProjectProvider } from '../../contexts/ProjectContext';
import ApplicationsPage from '../ApplicationsPage';

// Mock react-router-dom hooks
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

// Mock the useInstalledApps hook
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

  // Save original createElement before any tests run
  const originalCreateElement = document.createElement.bind(document);

  beforeEach(() => {
    vi.clearAllMocks();
    capturedHref = '';
    mockInstalledApps = [];

    // Mock document.createElement for anchor elements only
    vi.spyOn(document, 'createElement').mockImplementation(
      (tagName: string) => {
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
      }
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const renderPage = () => {
    return render(
      <ProjectProvider>
        <MemoryRouter initialEntries={['/projects/proj-1/applications']}>
          <Routes>
            <Route
              path="/projects/:pid/applications"
              element={<ApplicationsPage />}
            />
          </Routes>
        </MemoryRouter>
      </ProjectProvider>
    );
  };

  it('renders page header with correct title', () => {
    renderPage();

    expect(screen.getByText('Applications')).toBeInTheDocument();
    expect(
      screen.getByText('Choose an application to add to your project')
    ).toBeInTheDocument();
  });

  it('renders all application tiles', () => {
    renderPage();

    expect(screen.getByText('AI Data Liaison')).toBeInTheDocument();
    expect(screen.getByText('Ontology Forge')).toBeInTheDocument();
    expect(screen.getByText('MCP Tunnel')).toBeInTheDocument();
    // AI Agents tile is temporarily hidden
    expect(screen.queryByText('AI Agents and Automation')).not.toBeInTheDocument();
    expect(screen.getByText('Product Kit [COMING SOON]')).toBeInTheDocument();
    expect(screen.getByText('On-Premise Chat [COMING SOON]')).toBeInTheDocument();
    expect(screen.getByText('Your own Data Application')).toBeInTheDocument();
  });

  it('renders Contact Sales buttons for Product Kit and On-Premise Chat', () => {
    renderPage();

    const contactSalesButtons = screen.getAllByRole('button', {
      name: 'Contact Sales',
    });
    // Should have 2 Contact Sales buttons (Product Kit and On-Premise Chat)
    // AI Data Liaison now has Install button instead
    expect(contactSalesButtons).toHaveLength(2);
  });

  it('renders Install buttons for installable apps when not installed', () => {
    renderPage();

    // Ontology Forge + AI Data Liaison + Spreadsheet Loader + MCP Tunnel have Install buttons (AI Agents tile is temporarily hidden)
    const installButtons = screen.getAllByRole('button', { name: 'Install' });
    expect(installButtons).toHaveLength(4);
    // Ontology Forge + AI Data Liaison + MCP Tunnel have Learn More buttons
    const learnMoreButtons = screen.getAllByRole('button', { name: /Learn More/i });
    expect(learnMoreButtons).toHaveLength(3);
  });

  it('renders Installed badge, Learn More, and Configure button when AI Data Liaison is installed', () => {
    mockInstalledApps = ['ai-data-liaison'];
    renderPage();

    expect(screen.getByText('Installed')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Configure' })
    ).toBeInTheDocument();
    // Learn More should still be visible for installed apps that have a learnMoreUrl
    const learnMoreButtons = screen.getAllByRole('button', { name: /Learn More/i });
    expect(learnMoreButtons.length).toBeGreaterThanOrEqual(2);
    // Ontology Forge + Spreadsheet Loader + MCP Tunnel still have Install buttons
    const installButtons = screen.getAllByRole('button', { name: 'Install' });
    expect(installButtons).toHaveLength(3);
  });

  it('disables AI Data Liaison Install button when Ontology Forge is not installed', () => {
    renderPage();

    // AI Data Liaison Install button (second) should be disabled
    const installButtons = screen.getAllByRole('button', { name: 'Install' });
    expect(installButtons[1]).toBeDisabled();
    // "Requires" note should be visible
    expect(screen.getByText(/Requires Ontology Forge/)).toBeInTheDocument();
  });

  it('enables AI Data Liaison Install button when Ontology Forge is installed', async () => {
    mockInstalledApps = ['ontology-forge'];
    mockInstall.mockResolvedValue({ id: 'test-id', app_id: 'ai-data-liaison' });
    renderPage();

    // "Requires" note should be hidden
    expect(screen.queryByText(/Requires Ontology Forge/)).not.toBeInTheDocument();

    // AI Data Liaison Install button (first non-installed) should be enabled
    const installButtons = screen.getAllByRole('button', { name: 'Install' });
    expect(installButtons[0]).not.toBeDisabled();
    fireEvent.click(installButtons[0] as HTMLElement);

    await waitFor(() => {
      expect(mockInstall).toHaveBeenCalledWith('ai-data-liaison');
    });
  });

  it('navigates to config page when clicking Configure on installed AI Data Liaison', () => {
    mockInstalledApps = ['ai-data-liaison'];
    renderPage();

    const configureButton = screen.getByRole('button', { name: 'Configure' });
    fireEvent.click(configureButton);

    expect(mockNavigate).toHaveBeenCalledWith(
      '/projects/proj-1/ai-data-liaison'
    );
  });

  it('opens mailto link when clicking Contact Sales on Product Kit', () => {
    renderPage();

    const contactSalesButtons = screen.getAllByRole('button', {
      name: 'Contact Sales',
    });
    // First Contact Sales button is Product Kit (AI Data Liaison has Install button now)
    fireEvent.click(contactSalesButtons[0] as HTMLElement);

    expect(mockClick).toHaveBeenCalled();
    expect(capturedHref).toBe(
      'mailto:sales@ekaya.ai?subject=Interest%20in%20Product%20Kit%20%5BCOMING%20SOON%5D%20for%20my%20Ekaya%20project'
    );
  });

  it('opens mailto link when clicking Contact Sales on On-Premise Chat', () => {
    renderPage();

    const contactSalesButtons = screen.getAllByRole('button', {
      name: 'Contact Sales',
    });
    // Second Contact Sales button is On-Premise Chat
    fireEvent.click(contactSalesButtons[1] as HTMLElement);

    expect(mockClick).toHaveBeenCalled();
    expect(capturedHref).toBe(
      'mailto:sales@ekaya.ai?subject=Interest%20in%20On-Premise%20Chat%20%5BCOMING%20SOON%5D%20for%20my%20Ekaya%20project'
    );
  });

  it('renders Contact Support button on Build Your Own tile', () => {
    renderPage();

    const buildYourOwnCard = screen.getByTestId('app-card-build-your-own');
    expect(buildYourOwnCard).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Contact Support' })).toBeInTheDocument();
  });

  it('navigates back when clicking back button', () => {
    renderPage();

    const backButton = screen.getByRole('button', {
      name: 'Back to project dashboard',
    });
    fireEvent.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1');
  });

  it('opens mailto link when clicking Contact Support on Build Your Own tile', () => {
    renderPage();

    const contactSupportButton = screen.getByRole('button', { name: 'Contact Support' });
    fireEvent.click(contactSupportButton);

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
    // First Learn More is Ontology Forge (central), second is AI Data Liaison (central)
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
