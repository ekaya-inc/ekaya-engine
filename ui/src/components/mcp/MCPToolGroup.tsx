import { AlertTriangle } from 'lucide-react';

import { Card, CardContent } from '../ui/Card';
import { Switch } from '../ui/Switch';

interface MCPToolGroupProps {
  name: string;
  description: string;
  warning?: string;
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
  disabled?: boolean;
}

export default function MCPToolGroup({
  name,
  description,
  warning,
  enabled,
  onToggle,
  disabled = false,
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
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
