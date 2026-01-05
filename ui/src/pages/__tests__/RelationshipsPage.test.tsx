import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import RelationshipsPage from '../RelationshipsPage';
import type { RelationshipDetail, DatasourceSchema } from '../../types';

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

import engineApi from '../../services/engineApi';

describe('RelationshipsPage - Description Rendering', () => {
  const mockSchema: DatasourceSchema = {
    tables: [
      {
        name: 'users',
        columns: [
          { name: 'id', type: 'integer', nullable: false },
          { name: 'name', type: 'varchar', nullable: false },
        ],
      },
      {
        name: 'orders',
        columns: [
          { name: 'id', type: 'integer', nullable: false },
          { name: 'user_id', type: 'integer', nullable: false },
        ],
      },
    ],
  };

  const mockRelationshipWithDescription: RelationshipDetail = {
    source_entity_id: 'entity-1',
    source_entity: 'User',
    source_table_name: 'users',
    source_column_name: 'id',
    source_column_type: 'integer',
    target_entity_id: 'entity-2',
    target_entity: 'Order',
    target_table_name: 'orders',
    target_column_name: 'user_id',
    target_column_type: 'integer',
    relationship_type: 'foreign_key',
    cardinality: 'one_to_many',
    description: 'Orders belong to users. Each order is placed by exactly one user.',
  };

  const mockRelationshipWithoutDescription: RelationshipDetail = {
    source_entity_id: 'entity-3',
    source_entity: 'Product',
    source_table_name: 'products',
    source_column_name: 'id',
    source_column_type: 'integer',
    target_entity_id: 'entity-4',
    target_entity: 'OrderItem',
    target_table_name: 'order_items',
    target_column_name: 'product_id',
    target_column_type: 'integer',
    relationship_type: 'foreign_key',
    cardinality: 'one_to_many',
    description: undefined,
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockUseDatasourceConnection.mockReturnValue({
      selectedDatasource: {
        datasourceId: 'ds-1',
        name: 'Test DB',
      },
    });

    vi.mocked(engineApi.getSchema).mockResolvedValue({ data: mockSchema });
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
      data: {
        relationships: [mockRelationshipWithDescription],
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
      data: {
        relationships: [mockRelationshipWithoutDescription],
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
      data: {
        relationships: [mockRelationshipWithDescription, mockRelationshipWithoutDescription],
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
      data: {
        relationships: [maliciousRelationship],
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
