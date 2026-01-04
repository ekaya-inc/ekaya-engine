import { fireEvent, render, screen } from '@testing-library/react';
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

describe('ApplicationsPage', () => {
  const mockClick = vi.fn();
  let capturedHref = '';

  // Save original createElement before any tests run
  const originalCreateElement = document.createElement.bind(document);

  beforeEach(() => {
    vi.clearAllMocks();
    capturedHref = '';

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

    expect(screen.getByText('Install Application')).toBeInTheDocument();
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

  it('renders Contact Sales buttons for available applications', () => {
    renderPage();

    const contactSalesButtons = screen.getAllByRole('button', {
      name: 'Contact Sales',
    });
    // Should have 3 Contact Sales buttons (one for each available app)
    expect(contactSalesButtons).toHaveLength(3);
  });

  it('opens mailto link when clicking Contact Sales on AI Data Liaison', () => {
    renderPage();

    const contactSalesButtons = screen.getAllByRole('button', {
      name: 'Contact Sales',
    });
    fireEvent.click(contactSalesButtons[0] as HTMLElement);

    expect(mockClick).toHaveBeenCalled();
    expect(capturedHref).toBe(
      'mailto:sales@ekaya.ai?subject=Interest%20in%20AI%20Data%20Liaison%20for%20my%20Ekaya%20project'
    );
  });

  it('opens mailto link when clicking Contact Sales on Product Kit', () => {
    renderPage();

    const contactSalesButtons = screen.getAllByRole('button', {
      name: 'Contact Sales',
    });
    fireEvent.click(contactSalesButtons[1] as HTMLElement);

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
    fireEvent.click(contactSalesButtons[2] as HTMLElement);

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
});
