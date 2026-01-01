import { AlertTriangle } from 'lucide-react';
import type { ReactNode } from 'react';

import type { SubOptionInfo } from '../../types';
import { Card, CardContent } from '../ui/Card';
import { Switch } from '../ui/Switch';

interface MCPToolGroupProps {
  name: string;
  description: ReactNode;
  warning?: string;
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
  disabled?: boolean;
  subOptions?: Record<string, SubOptionInfo>;
  onSubOptionToggle?: (subOptionName: string, enabled: boolean) => void;
}

export default function MCPToolGroup({
  name,
  description,
  warning,
  enabled,
  onToggle,
  disabled = false,
  subOptions,
  onSubOptionToggle,
}: MCPToolGroupProps) {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="flex items-start justify-between gap-4">
          <div className="flex-1 space-y-2">
            <div className="flex items-center gap-3">
              <Switch
                checked={enabled}
                onCheckedChange={onToggle}
                disabled={disabled}
              />
              <span className="text-lg font-medium text-text-primary">{name}</span>
            </div>
            <p className="text-sm text-text-secondary">{description}</p>
            {warning && (
              <div className="flex items-start gap-2 rounded-md bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-950 dark:text-amber-200">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                <span>{warning}</span>
              </div>
            )}
            {enabled && subOptions && Object.entries(subOptions).map(([subName, subOption]) => (
              <div key={subName} className="mt-4 border-t border-border-light pt-4">
                <div className="flex items-center gap-3">
                  <Switch
                    checked={subOption.enabled}
                    onCheckedChange={(checked) => onSubOptionToggle?.(subName, checked)}
                    disabled={disabled}
                  />
                  <span className="text-sm font-medium text-text-primary">{subOption.name}</span>
                </div>
                {subOption.description && (
                  <p className="mt-1 text-sm text-text-secondary">{subOption.description}</p>
                )}
                {subOption.warning && (
                  <div className="mt-2 flex items-start gap-2 rounded-md bg-red-50 p-3 text-sm text-red-800 dark:bg-red-950 dark:text-red-200">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                    <span>{subOption.warning}</span>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
