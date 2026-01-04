import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ToastProviderComponent } from '../../hooks/useToast';
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
  beforeEach(() => {
    vi.clearAllMocks();
  });

  const renderPage = () => {
    return render(
      <ToastProviderComponent>
        <MemoryRouter initialEntries={['/projects/proj-1/applications']}>
          <Routes>
            <Route
              path="/projects/:pid/applications"
              element={<ApplicationsPage />}
            />
          </Routes>
        </MemoryRouter>
      </ToastProviderComponent>
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

  it('shows coming soon toast when clicking on available application', () => {
    renderPage();

    const aiDataLiaisonCard = screen.getByTestId('app-card-ai-data-liaison');
    fireEvent.click(aiDataLiaisonCard);

    // Toast should appear with the application name
    expect(
      screen.getByText('AI Data Liaison installation coming soon!')
    ).toBeInTheDocument();
  });

  it('shows coming soon toast for Product Kit', () => {
    renderPage();

    const productKitCard = screen.getByTestId('app-card-product-kit');
    fireEvent.click(productKitCard);

    expect(
      screen.getByText('Product Kit installation coming soon!')
    ).toBeInTheDocument();
  });

  it('shows coming soon toast for On-Premise Chat', () => {
    renderPage();

    const onPremiseChatCard = screen.getByTestId('app-card-on-premise-chat');
    fireEvent.click(onPremiseChatCard);

    expect(
      screen.getByText('On-Premise Chat installation coming soon!')
    ).toBeInTheDocument();
  });

  it('does not show toast for disabled More Coming tile', () => {
    renderPage();

    const moreComingCard = screen.getByTestId('app-card-more-coming');
    fireEvent.click(moreComingCard);

    // No toast should appear
    expect(
      screen.queryByText(/installation coming soon!/i)
    ).not.toBeInTheDocument();
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
