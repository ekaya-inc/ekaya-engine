/**
 * Rejection Reason Dialog
 * Modal to enter rejection reason before confirming query rejection
 */

import { AlertCircle, Loader2, XCircle } from 'lucide-react';
import { useState } from 'react';

import { Button } from './ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog';

interface RejectionReasonDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  queryName: string;
  onReject: (reason: string) => Promise<void>;
}

/**
 * Dialog for entering a rejection reason when rejecting a pending query.
 */
export const RejectionReasonDialog = ({
  open,
  onOpenChange,
  queryName,
  onReject,
}: RejectionReasonDialogProps) => {
  const [reason, setReason] = useState('');
  const [isRejecting, setIsRejecting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleReject = async (): Promise<void> => {
    if (!reason.trim()) {
      setError('Please provide a reason for rejection');
      return;
    }

    setIsRejecting(true);
    setError(null);

    try {
      await onReject(reason.trim());
      setReason('');
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to reject query');
    } finally {
      setIsRejecting(false);
    }
  };

  const handleOpenChange = (newOpen: boolean): void => {
    if (!isRejecting) {
      if (!newOpen) {
        setReason('');
        setError(null);
      }
      onOpenChange(newOpen);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-red-500" />
            Reject Query Suggestion
          </DialogTitle>
          <DialogDescription>
            Provide a reason for rejecting this query suggestion. The reason
            will be recorded for reference.
          </DialogDescription>
        </DialogHeader>

        <div className="py-4">
          <div className="rounded-md bg-surface-secondary p-3 text-sm mb-4">
            <div className="font-medium text-text-primary line-clamp-2">
              {queryName}
            </div>
          </div>

          <div>
            <label
              htmlFor="rejection-reason"
              className="block text-sm font-medium text-text-primary mb-2"
            >
              Reason for rejection <span className="text-red-500">*</span>
            </label>
            <textarea
              id="rejection-reason"
              value={reason}
              onChange={(e) => {
                setReason(e.target.value);
                if (error) setError(null);
              }}
              placeholder="Explain why this query is being rejected..."
              className="w-full h-24 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
              disabled={isRejecting}
            />
          </div>

          {error && (
            <div className="mt-4 flex items-center gap-2 text-sm text-red-600 dark:text-red-400">
              <AlertCircle className="h-4 w-4" />
              {error}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={isRejecting}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleReject}
            disabled={isRejecting || !reason.trim()}
          >
            {isRejecting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Rejecting...
              </>
            ) : (
              'Reject Query'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
