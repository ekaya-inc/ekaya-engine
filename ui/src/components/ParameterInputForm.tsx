/**
 * ParameterInputForm Component
 * Renders input fields for parameter values with type-appropriate controls
 */


import type { QueryParameter } from '../types/query';

import { Input } from './ui/Input';

interface ParameterInputFormProps {
  parameters: QueryParameter[];
  values: Record<string, unknown>;
  onChange: (values: Record<string, unknown>) => void;
}

/**
 * Render type-appropriate input for a parameter
 */
const ParameterInput = ({
  parameter,
  value,
  onChange,
}: {
  parameter: QueryParameter;
  value: unknown;
  onChange: (value: unknown) => void;
}) => {
  const stringValue = value?.toString() ?? parameter.default?.toString() ?? '';

  const inputId = `param-${parameter.name}`;

  switch (parameter.type) {
    case 'boolean':
      return (
        <div className="flex items-center gap-2">
          <input
            id={inputId}
            type="checkbox"
            checked={value === true || value === 'true'}
            onChange={(e) => onChange(e.target.checked)}
            className="rounded border-border-medium"
          />
          <span className="text-xs text-text-tertiary">
            {value === true || value === 'true' ? 'true' : 'false'}
          </span>
        </div>
      );

    case 'integer':
    case 'decimal':
      return (
        <Input
          id={inputId}
          type="number"
          value={stringValue}
          onChange={(e) => onChange(e.target.value)}
          placeholder={
            parameter.default
              ? `Default: ${parameter.default}`
              : parameter.required
                ? 'Required'
                : 'Optional'
          }
          step={parameter.type === 'decimal' ? '0.01' : '1'}
          className="h-9"
        />
      );

    case 'date':
      return (
        <Input
          id={inputId}
          type="date"
          value={stringValue}
          onChange={(e) => onChange(e.target.value)}
          placeholder={
            parameter.default
              ? `Default: ${parameter.default}`
              : parameter.required
                ? 'Required'
                : 'Optional'
          }
          className="h-9"
        />
      );

    case 'timestamp':
      return (
        <Input
          id={inputId}
          type="datetime-local"
          value={stringValue}
          onChange={(e) => onChange(e.target.value)}
          placeholder={
            parameter.default
              ? `Default: ${parameter.default}`
              : parameter.required
                ? 'Required'
                : 'Optional'
          }
          className="h-9"
        />
      );

    case 'string[]':
    case 'integer[]':
      return (
        <div className="space-y-1">
          <Input
            id={inputId}
            type="text"
            value={stringValue}
            onChange={(e) => onChange(e.target.value)}
            placeholder={
              parameter.type === 'string[]'
                ? 'Comma-separated values: a,b,c'
                : 'Comma-separated integers: 1,2,3'
            }
            className="h-9"
          />
          <p className="text-xs text-text-tertiary">
            Enter values separated by commas
          </p>
        </div>
      );

    case 'string':
    case 'uuid':
    default:
      return (
        <Input
          id={inputId}
          type="text"
          value={stringValue}
          onChange={(e) => onChange(e.target.value)}
          placeholder={
            parameter.default
              ? `Default: ${parameter.default}`
              : parameter.required
                ? 'Required'
                : 'Optional'
          }
          className="h-9"
        />
      );
  }
};

export const ParameterInputForm = ({
  parameters,
  values,
  onChange,
}: ParameterInputFormProps) => {
  if (parameters.length === 0) {
    return null;
  }

  const handleValueChange = (paramName: string, value: unknown) => {
    onChange({
      ...values,
      [paramName]: value,
    });
  };

  // Separate required and optional parameters
  const requiredParams = parameters.filter((p) => p.required);
  const optionalParams = parameters.filter((p) => !p.required);

  return (
    <div className="space-y-4">
      {requiredParams.length > 0 && (
        <div className="space-y-3">
          {requiredParams.map((param) => (
            <div key={param.name}>
              <label
                htmlFor={`param-${param.name}`}
                className="block text-sm font-medium text-text-primary mb-1"
              >
                {param.name}
                <span className="text-red-500 ml-1">*</span>
                <span className="text-xs text-text-tertiary font-normal ml-2">
                  ({param.type})
                </span>
              </label>
              {param.description && (
                <p className="text-xs text-text-secondary mb-2">
                  {param.description}
                </p>
              )}
              <ParameterInput
                parameter={param}
                value={values[param.name]}
                onChange={(value) => handleValueChange(param.name, value)}
              />
            </div>
          ))}
        </div>
      )}

      {optionalParams.length > 0 && (
        <div className="space-y-3">
          {optionalParams.map((param) => (
            <div key={param.name}>
              <label
                htmlFor={`param-${param.name}`}
                className="block text-sm font-medium text-text-primary mb-1"
              >
                {param.name}
                <span className="text-xs text-text-tertiary font-normal ml-2">
                  ({param.type})
                </span>
              </label>
              {param.description && (
                <p className="text-xs text-text-secondary mb-2">
                  {param.description}
                </p>
              )}
              <ParameterInput
                parameter={param}
                value={values[param.name]}
                onChange={(value) => handleValueChange(param.name, value)}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
