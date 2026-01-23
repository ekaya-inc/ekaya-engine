import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

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
      <MemoryRouter initialEntries={['/projects/proj-1/applications']}>
        <Routes>
          <Route
            path="/projects/:pid/applications"
            element={<ApplicationsPage />}
          />
        </Routes>
      </MemoryRouter>
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
    expect(screen.getByText('Product Kit')).toBeInTheDocument();
    expect(screen.getByText('On-Premise Chat')).toBeInTheDocument();
    expect(screen.getByText('More Coming!')).toBeInTheDocument();
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

  it('renders Install button for AI Data Liaison when not installed', () => {
    renderPage();

    expect(screen.getByRole('button', { name: 'Install' })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /Learn More/i })
    ).toBeInTheDocument();
  });

  it('renders Installed badge and Configure button when AI Data Liaison is installed', () => {
    mockInstalledApps = ['ai-data-liaison'];
    renderPage();

    expect(screen.getByText('Installed')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Configure' })
    ).toBeInTheDocument();
    // Install button should not be present
    expect(screen.queryByRole('button', { name: 'Install' })).toBeNull();
  });

  it('calls install and navigates when clicking Install on AI Data Liaison', async () => {
    mockInstall.mockResolvedValue({ id: 'test-id', app_id: 'ai-data-liaison' });
    renderPage();

    const installButton = screen.getByRole('button', { name: 'Install' });
    fireEvent.click(installButton);

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
      'mailto:sales@ekaya.ai?subject=Interest%20in%20Product%20Kit%20for%20my%20Ekaya%20project'
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
      'mailto:sales@ekaya.ai?subject=Interest%20in%20On-Premise%20Chat%20for%20my%20Ekaya%20project'
    );
  });

  it('does not render Contact Sales button for disabled More Coming tile', () => {
    renderPage();

    const moreComingCard = screen.getByTestId('app-card-more-coming');
    const contactSalesButton = moreComingCard.querySelector(
      'button[name="Contact Sales"]'
    );
    expect(contactSalesButton).toBeNull();
  });

  it('navigates back when clicking back button', () => {
    renderPage();

    const backButton = screen.getByRole('button', {
      name: 'Back to project dashboard',
    });
    fireEvent.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith('/projects/proj-1');
  });

  it('displays Coming Soon text for disabled tiles', () => {
    renderPage();

    // The "More Coming!" tile should have "Coming Soon" footer text
    expect(screen.getByText('Coming Soon')).toBeInTheDocument();
  });

  it('opens Learn More link in new tab', () => {
    const mockOpen = vi.fn();
    vi.spyOn(window, 'open').mockImplementation(mockOpen);

    renderPage();

    const learnMoreButton = screen.getByRole('button', { name: /Learn More/i });
    fireEvent.click(learnMoreButton);

    expect(mockOpen).toHaveBeenCalledWith(
      'https://ekaya.ai/enterprise/',
      '_blank',
      'noopener,noreferrer'
    );
  });
});
