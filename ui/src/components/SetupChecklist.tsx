import { CheckCircle2, Circle, Loader2, XCircle } from 'lucide-react';
import { Link } from 'react-router-dom';

import { Button } from './ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/Card';

export interface ChecklistItem {
  id: string;
  title: string;
  description: string;
  status: 'pending' | 'complete' | 'error' | 'loading';
  link?: string;
  linkText?: string;
  onAction?: () => void;
  actionText?: string;
  actionDisabled?: boolean;
  disabled?: boolean;
  optional?: boolean;
}

interface SetupChecklistProps {
  items: ChecklistItem[];
  title?: string;
  description?: string;
  completeDescription?: string;
}

const SetupChecklist = ({
  items,
  title = 'Setup Checklist',
  description = 'Complete these steps to get started',
  completeDescription,
}: SetupChecklistProps) => {
  const allComplete = items.filter((item) => !item.optional).every((item) => item.status === 'complete');

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {allComplete ? (
            <CheckCircle2 className="h-5 w-5 text-green-500" />
          ) : (
            <Circle className="h-5 w-5 text-text-secondary" />
          )}
          {title}
        </CardTitle>
        <CardDescription>
          {allComplete ? (completeDescription ?? description) : description}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {items.map((item, index) => (
            <div
              key={item.id}
              className={`flex items-start gap-3 rounded-lg border border-border-light p-3${item.disabled ? ' opacity-50' : ''}`}
            >
              <div className="mt-0.5">
                {item.status === 'loading' ? (
                  <Loader2 className="h-5 w-5 animate-spin text-text-secondary" />
                ) : item.status === 'complete' ? (
                  <CheckCircle2 className="h-5 w-5 text-green-500" />
                ) : item.status === 'error' ? (
                  <XCircle className="h-5 w-5 text-red-500" />
                ) : (
                  <Circle className="h-5 w-5 text-text-secondary" />
                )}
              </div>
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-text-primary">
                    {index + 1}. {item.title}
                  </span>
                </div>
                <p className="text-sm text-text-secondary">{item.description}</p>
              </div>
              {item.onAction && item.status !== 'complete' && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={item.onAction}
                  disabled={item.actionDisabled}
                >
                  {item.actionText ?? 'Action'}
                </Button>
              )}
              {item.link && item.status !== 'complete' && !item.onAction && (
                <Link to={item.link}>
                  <Button variant="outline" size="sm">
                    {item.linkText}
                  </Button>
                </Link>
              )}
              {item.link && item.status === 'complete' && (
                <Link to={item.link}>
                  <Button variant="ghost" size="sm" className="text-text-secondary">
                    {item.linkText}
                  </Button>
                </Link>
              )}
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
};

export default SetupChecklist;
