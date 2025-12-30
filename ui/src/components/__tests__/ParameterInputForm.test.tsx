import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { QueryParameter } from '../../types';
import { ParameterInputForm } from '../ParameterInputForm';

describe('ParameterInputForm', () => {
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
      name: 'start_date',
      type: 'date',
      description: 'Start of date range',
      required: false,
      default: '2024-01-01',
    },
    {
      name: 'limit',
      type: 'integer',
      description: 'Max rows to return',
      required: false,
      default: 100,
    },
    {
      name: 'is_active',
      type: 'boolean',
      description: 'Filter by active status',
      required: false,
      default: true,
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders nothing when no parameters', () => {
    const { container } = render(
      <ParameterInputForm parameters={[]} values={{}} onChange={mockOnChange} />
    );

    expect(container.firstChild).toBeNull();
  });

  it('renders parameter input fields', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    expect(screen.getByText('Query Parameters')).toBeInTheDocument();
    expect(screen.getByLabelText(/customer_id/)).toBeInTheDocument();
    expect(screen.getByLabelText(/start_date/)).toBeInTheDocument();
    expect(screen.getByLabelText(/limit/)).toBeInTheDocument();
    expect(screen.getByLabelText(/is_active/)).toBeInTheDocument();
  });

  it('shows required indicator for required parameters', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    // customer_id is required, should have red asterisk
    const customerIdLabel = screen.getByText('customer_id').closest('label');
    expect(customerIdLabel?.querySelector('.text-red-500')).toBeInTheDocument();
  });

  it('separates required and optional parameters', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    expect(screen.getByText('Required Parameters')).toBeInTheDocument();
    expect(screen.getByText('Optional Parameters')).toBeInTheDocument();
  });

  it('renders text input for string type', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    const input = screen.getByLabelText(/customer_id/) as HTMLInputElement;
    expect(input.type).toBe('text');
  });

  it('renders number input for integer type', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    const input = screen.getByLabelText(/limit/) as HTMLInputElement;
    expect(input.type).toBe('number');
    expect(input.step).toBe('1');
  });

  it('renders date input for date type', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    const input = screen.getByLabelText(/start_date/) as HTMLInputElement;
    expect(input.type).toBe('date');
  });

  it('renders checkbox for boolean type', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    const input = screen.getByLabelText(/is_active/) as HTMLInputElement;
    expect(input.type).toBe('checkbox');
  });

  it('calls onChange when string input value changes', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    const input = screen.getByLabelText(/customer_id/);
    fireEvent.change(input, { target: { value: 'cust-123' } });

    expect(mockOnChange).toHaveBeenCalledWith({
      customer_id: 'cust-123',
    });
  });

  it('calls onChange when number input value changes', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    const input = screen.getByLabelText(/limit/);
    fireEvent.change(input, { target: { value: '50' } });

    expect(mockOnChange).toHaveBeenCalledWith({
      limit: '50',
    });
  });

  it('calls onChange when date input value changes', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    const input = screen.getByLabelText(/start_date/);
    fireEvent.change(input, { target: { value: '2024-06-15' } });

    expect(mockOnChange).toHaveBeenCalledWith({
      start_date: '2024-06-15',
    });
  });

  it('calls onChange when checkbox value changes', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    const input = screen.getByLabelText(/is_active/);
    fireEvent.click(input);

    expect(mockOnChange).toHaveBeenCalledWith({
      is_active: true,
    });
  });

  it('displays parameter descriptions', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    expect(screen.getByText('Customer unique identifier')).toBeInTheDocument();
    expect(screen.getByText('Start of date range')).toBeInTheDocument();
    expect(screen.getByText('Max rows to return')).toBeInTheDocument();
  });

  it('displays parameter types in labels', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{}}
        onChange={mockOnChange}
      />
    );

    expect(screen.getByText('(string)')).toBeInTheDocument();
    expect(screen.getByText('(date)')).toBeInTheDocument();
    expect(screen.getByText('(integer)')).toBeInTheDocument();
    expect(screen.getByText('(boolean)')).toBeInTheDocument();
  });

  it('pre-fills values from props', () => {
    render(
      <ParameterInputForm
        parameters={sampleParameters}
        values={{
          customer_id: 'existing-customer',
          limit: 200,
        }}
        onChange={mockOnChange}
      />
    );

    expect(
      (screen.getByLabelText(/customer_id/) as HTMLInputElement).value
    ).toBe('existing-customer');
    expect((screen.getByLabelText(/limit/) as HTMLInputElement).value).toBe(
      '200'
    );
  });

  it('renders array input with comma-separated hint', () => {
    const arrayParam: QueryParameter = {
      name: 'tags',
      type: 'string[]',
      description: 'Filter by tags',
      required: false,
      default: null,
    };

    render(
      <ParameterInputForm
        parameters={[arrayParam]}
        values={{}}
        onChange={mockOnChange}
      />
    );

    expect(
      screen.getByPlaceholderText(/Comma-separated values: a,b,c/)
    ).toBeInTheDocument();
    expect(screen.getByText(/Enter values separated by commas/)).toBeInTheDocument();
  });
});
