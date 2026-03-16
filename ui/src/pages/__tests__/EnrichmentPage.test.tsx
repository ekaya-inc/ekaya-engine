import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import ontologyApi from '../../services/ontologyApi';
import type { EnrichmentResponse } from '../../types';
import EnrichmentPage from '../EnrichmentPage';

const mockNavigate = vi.fn();

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock('../../services/ontologyApi', () => ({
  default: {
    getEnrichment: vi.fn(),
  },
}));

describe('EnrichmentPage', () => {
  const mockEnrichment: EnrichmentResponse = {
    entity_summaries: [],
    column_details: [
      {
        table_name: 'products',
        columns: [
          {
            name: 'id',
            description: 'Unique identifier for the product.',
            synonyms: [],
            semantic_type: 'identifier',
            role: 'primary_key',
            nullable: false,
            is_primary_key: true,
            is_foreign_key: false,
          },
          {
            name: 'product_distribution_center_id',
            description: 'Identifies the distribution center where the product is located.',
            synonyms: [],
            semantic_type: 'identifier',
            role: 'foreign_key',
            nullable: false,
            is_primary_key: false,
            is_foreign_key: true,
            foreign_table: 'distribution_centers',
          },
        ],
      },
    ],
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(ontologyApi.getEnrichment).mockResolvedValue(mockEnrichment);
  });

  const renderPage = () =>
    render(
      <MemoryRouter initialEntries={['/projects/proj-1/enrichment']}>
        <Routes>
          <Route path="/projects/:pid/enrichment" element={<EnrichmentPage />} />
        </Routes>
      </MemoryRouter>
    );

  it('does not duplicate structural key badges when role mirrors pk/fk status', async () => {
    const user = userEvent.setup();
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('products')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: /products/i }));

    await waitFor(() => {
      expect(screen.getByText('product_distribution_center_id')).toBeInTheDocument();
    });

    expect(screen.getAllByText('identifier')).toHaveLength(2);
    expect(screen.getByText('Foreign Key')).toBeInTheDocument();
    expect(screen.getByText('Primary Key')).toBeInTheDocument();
    expect(screen.queryByText('foreign_key')).not.toBeInTheDocument();
    expect(screen.queryByText('primary_key')).not.toBeInTheDocument();
  });
});
