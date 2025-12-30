import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { QueryParameter } from '../../types';
import { ParameterEditor } from '../ParameterEditor';

describe('ParameterEditor', () => {
  const mockOnChange = vi.fn();

  const sampleParameters: QueryParameter[] = [
    {
      name: 'customer_id',
      type: 'string',
      description: 'Customer unique identifier',
      required: true,
      default: null,
    },
    {
      name: 'limit',
      type: 'integer',
      description: 'Max rows to return',
      required: false,
      default: 100,
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders empty state when no parameters', () => {
    render(
      <ParameterEditor parameters={[]} onChange={mockOnChange} sqlQuery="" />
    );

    expect(
      screen.getByText(/No parameters defined. Click "Add Parameter" to create one./)
    ).toBeInTheDocument();
  });

  it('renders parameter list when parameters exist', () => {
    render(
      <ParameterEditor
        parameters={sampleParameters}
        onChange={mockOnChange}
        sqlQuery="SELECT * FROM customers WHERE id = {{customer_id}} LIMIT {{limit}}"
      />
    );

    expect(screen.getByDisplayValue('customer_id')).toBeInTheDocument();
    expect(screen.getByDisplayValue('limit')).toBeInTheDocument();
  });

  it('shows warning for undefined parameters in SQL', () => {
    render(
      <ParameterEditor
        parameters={sampleParameters}
        onChange={mockOnChange}
        sqlQuery="SELECT * FROM customers WHERE id = {{customer_id}} AND region = {{region_id}}"
      />
    );

    expect(screen.getByText(/Undefined parameters in SQL/)).toBeInTheDocument();
    expect(screen.getByText(/{{region_id}}/)).toBeInTheDocument();
  });

  it('shows warning for unused parameters', () => {
    render(
      <ParameterEditor
        parameters={sampleParameters}
        onChange={mockOnChange}
        sqlQuery="SELECT * FROM customers WHERE id = {{customer_id}}"
      />
    );

    expect(screen.getByText(/Unused parameters/)).toBeInTheDocument();
    expect(screen.getByText(/limit/)).toBeInTheDocument();
  });

  it('calls onChange when add parameter button is clicked', () => {
    render(
      <ParameterEditor parameters={[]} onChange={mockOnChange} sqlQuery="" />
    );

    const addButton = screen.getByRole('button', { name: /Add Parameter/i });
    fireEvent.click(addButton);

    expect(mockOnChange).toHaveBeenCalledWith([
      {
        name: '',
        type: 'string',
        description: '',
        required: true,
        default: null,
      },
    ]);
  });

  it('calls onChange when parameter name is updated', () => {
    render(
      <ParameterEditor
        parameters={sampleParameters}
        onChange={mockOnChange}
        sqlQuery=""
      />
    );

    const nameInput = screen.getByDisplayValue('customer_id');
    fireEvent.change(nameInput, { target: { value: 'user_id' } });

    expect(mockOnChange).toHaveBeenCalledWith([
      {
        ...sampleParameters[0],
        name: 'user_id',
      },
      sampleParameters[1],
    ]);
  });

  it('calls onChange when parameter type is updated', () => {
    render(
      <ParameterEditor
        parameters={sampleParameters}
        onChange={mockOnChange}
        sqlQuery=""
      />
    );

    const typeSelects = screen.getAllByRole('combobox');
    const firstSelect = typeSelects[0];
    expect(firstSelect).toBeDefined();
    if (firstSelect) {
      fireEvent.change(firstSelect, { target: { value: 'uuid' } });
    }

    expect(mockOnChange).toHaveBeenCalledWith([
      {
        ...sampleParameters[0],
        type: 'uuid',
      },
      sampleParameters[1],
    ]);
  });

  it('calls onChange when parameter is removed', () => {
    render(
      <ParameterEditor
        parameters={sampleParameters}
        onChange={mockOnChange}
        sqlQuery=""
      />
    );

    const deleteButtons = screen.getAllByRole('button');
    // Find the button that contains a trash icon (look for svg with lucide-trash-2 class)
    const firstDeleteButton = deleteButtons.find((btn) => {
      const svg = btn.querySelector('svg');
      return svg?.classList.contains('lucide-trash-2');
    });

    if (firstDeleteButton) {
      fireEvent.click(firstDeleteButton);
      expect(mockOnChange).toHaveBeenCalledWith([sampleParameters[1]]);
    } else {
      // Alternative: click the first small icon button after the input fields
      const iconButtons = deleteButtons.filter((btn) =>
        btn.className.includes('h-8 w-8')
      );
      const firstIconButton = iconButtons[0];
      if (firstIconButton) {
        fireEvent.click(firstIconButton);
        expect(mockOnChange).toHaveBeenCalledWith([sampleParameters[1]]);
      }
    }
  });

  it('adds parameter with suggested name when quick-add button is clicked', () => {
    render(
      <ParameterEditor
        parameters={[]}
        onChange={mockOnChange}
        sqlQuery="SELECT * FROM orders WHERE region = {{region_id}}"
      />
    );

    const addRegionButton = screen.getByRole('button', {
      name: /Add region_id/i,
    });
    fireEvent.click(addRegionButton);

    expect(mockOnChange).toHaveBeenCalledWith([
      {
        name: 'region_id',
        type: 'string',
        description: '',
        required: true,
        default: null,
      },
    ]);
  });

  it('expands parameter details when row is clicked', () => {
    render(
      <ParameterEditor
        parameters={sampleParameters}
        onChange={mockOnChange}
        sqlQuery=""
      />
    );

    // Initially expanded row should show description input
    expect(
      screen.getByDisplayValue('Customer unique identifier')
    ).toBeInTheDocument();
  });
});
