import { render, screen, waitFor } from '@testing-library/react';
import React from 'react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import OntologyDAG from '../../components/ontology/OntologyDAG';
import OntologyPage from '../OntologyPage';

// Mock the OntologyDAG component
vi.mock('../../components/ontology/OntologyDAG', () => ({
  default: vi.fn(),
}));

// Mock the DatasourceConnectionContext
const mockUseDatasourceConnection = vi.fn();
vi.mock('../../contexts/DatasourceConnectionContext', () => ({
  useDatasourceConnection: () => mockUseDatasourceConnection(),
}));

// Mock react-router-dom hooks
const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

// Import the mocked OntologyDAG

describe('OntologyPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'Test DB',
      },
    });

    // Default mock implementation
    vi.mocked(OntologyDAG).mockImplementation(({ onStatusChange }) => {
      React.useEffect(() => {
        onStatusChange?.(true);
      }, [onStatusChange]);
      return <div data-testid="ontology-dag">OntologyDAG Component</div>;
    });
  });

  const renderPage = () => {
    return render(
      <MemoryRouter initialEntries={['/projects/proj-1/ontology']}>
        <Routes>
          <Route path="/projects/:pid/ontology" element={<OntologyPage />} />
        </Routes>
      </MemoryRouter>
    );
  };

  it('hides "How it works" banner when ontology exists', async () => {
    // Mock OntologyDAG to report hasOntology=true (already set in beforeEach)
    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId('ontology-dag')).toBeInTheDocument();
    });

    // Banner should NOT be present
    expect(screen.queryByText(/How it works:/i)).not.toBeInTheDocument();
  });

  it('shows "How it works" banner when no ontology exists', async () => {
    // Mock OntologyDAG to report hasOntology=false
    vi.mocked(OntologyDAG).mockImplementation(({ onStatusChange }) => {
      React.useEffect(() => {
        onStatusChange?.(false);
      }, [onStatusChange]);
      return <div data-testid="ontology-dag">OntologyDAG Component</div>;
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId('ontology-dag')).toBeInTheDocument();
    });

    // Banner SHOULD be present
    await waitFor(() => {
      expect(screen.getByText(/How it works:/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/The extraction process runs automatically through 7 steps/i)).toBeInTheDocument();
  });

  it('shows "No Datasource Selected" when no datasource is selected', () => {
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: null,
    });

    renderPage();

    expect(screen.getByText(/No Datasource Selected/i)).toBeInTheDocument();
    expect(screen.queryByTestId('ontology-dag')).not.toBeInTheDocument();
  });

  it('renders page header with correct title', () => {
    renderPage();

    expect(screen.getByText('Ontology Extraction')).toBeInTheDocument();
    expect(screen.getByText(/Extract business knowledge from your database schema/i)).toBeInTheDocument();
  });
});
