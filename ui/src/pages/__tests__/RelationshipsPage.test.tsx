import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import engineApi from '../../services/engineApi';
import type { RelationshipDetail, DatasourceSchema } from '../../types';
import RelationshipsPage from '../RelationshipsPage';

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

// Mock the engineApi
vi.mock('../../services/engineApi', () => ({
  default: {
    getRelationships: vi.fn(),
    getSchema: vi.fn(),
  },
}));

describe('RelationshipsPage - Description Rendering', () => {
  const mockSchema: DatasourceSchema = {
    tables: [
      {
        table_name: 'users',
        columns: [
          { column_name: 'id', data_type: 'integer' },
          { column_name: 'name', data_type: 'varchar' },
        ],
      },
      {
        table_name: 'orders',
        columns: [
          { column_name: 'id', data_type: 'integer' },
          { column_name: 'user_id', data_type: 'integer' },
        ],
      },
    ],
    total_tables: 2,
    relationships: [],
  };

  const mockRelationshipWithDescription: RelationshipDetail = {
    id: 'rel-1',
    source_table_name: 'users',
    source_column_name: 'id',
    source_column_type: 'integer',
    target_table_name: 'orders',
    target_column_name: 'user_id',
    target_column_type: 'integer',
    relationship_type: 'fk',
    cardinality: '1:N',
    confidence: 1.0,
    inference_method: null,
    is_validated: true,
    is_approved: true,
    description: 'Orders belong to users. Each order is placed by exactly one user.',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  };

  const mockRelationshipWithoutDescription: RelationshipDetail = {
    id: 'rel-2',
    source_table_name: 'products',
    source_column_name: 'id',
    source_column_type: 'integer',
    target_table_name: 'order_items',
    target_column_name: 'product_id',
    target_column_type: 'integer',
    relationship_type: 'fk',
    cardinality: '1:N',
    confidence: 1.0,
    inference_method: null,
    is_validated: true,
    is_approved: true,
    // description intentionally omitted to test absence case
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'Test DB',
      },
    });

    vi.mocked(engineApi.getSchema).mockResolvedValue({
      success: true,
      data: mockSchema,
    });
  });

  const renderPage = () => {
    return render(
      <MemoryRouter initialEntries={['/projects/proj-1/relationships']}>
        <Routes>
          <Route path="/projects/:pid/relationships" element={<RelationshipsPage />} />
        </Routes>
      </MemoryRouter>
    );
  };

  it('renders description when present', async () => {
    vi.mocked(engineApi.getRelationships).mockResolvedValue({
      success: true,
      data: {
        relationships: [mockRelationshipWithDescription],
        total_count: 1,
        empty_tables: [],
        orphan_tables: [],
      },
    });

    renderPage();

    await waitFor(() => {
      expect(screen.getByText('Orders belong to users. Each order is placed by exactly one user.')).toBeInTheDocument();
    });
  });

  it('does not render description when absent', async () => {
    vi.mocked(engineApi.getRelationships).mockResolvedValue({
      success: true,
      data: {
        relationships: [mockRelationshipWithoutDescription],
        total_count: 1,
        empty_tables: [],
        orphan_tables: [],
      },
    });

    renderPage();

    // Wait for page to finish loading
    await waitFor(() => {
      expect(screen.queryByText('Failed to Load Relationships')).not.toBeInTheDocument();
    });

    // Description should not be present for relationship without description
    expect(screen.queryByText(/belong to/i)).not.toBeInTheDocument();
  });

  it('renders multiple relationships with mixed descriptions', async () => {
    vi.mocked(engineApi.getRelationships).mockResolvedValue({
      success: true,
      data: {
        relationships: [mockRelationshipWithDescription, mockRelationshipWithoutDescription],
        total_count: 2,
        empty_tables: [],
        orphan_tables: [],
      },
    });

    renderPage();

    await waitFor(() => {
      // First relationship should have description
      expect(screen.getByText('Orders belong to users. Each order is placed by exactly one user.')).toBeInTheDocument();
    });

    // Verify page shows 2 relationships in summary
    expect(screen.getByText(/2.*total relationships/)).toBeInTheDocument();
  });

  it('escapes HTML in description (XSS protection)', async () => {
    const maliciousRelationship: RelationshipDetail = {
      ...mockRelationshipWithDescription,
      description: '<script>alert("xss")</script>This is a description',
    };

    vi.mocked(engineApi.getRelationships).mockResolvedValue({
      success: true,
      data: {
        relationships: [maliciousRelationship],
        total_count: 1,
        empty_tables: [],
        orphan_tables: [],
      },
    });

    renderPage();

    await waitFor(() => {
      const descriptionElement = screen.getByText(/<script>alert\("xss"\)<\/script>This is a description/);
      expect(descriptionElement).toBeInTheDocument();
      // Verify the text is rendered as plain text, not executed
      expect(descriptionElement.innerHTML).not.toContain('<script>');
      expect(descriptionElement.textContent).toContain('<script>');
    });
  });
});
