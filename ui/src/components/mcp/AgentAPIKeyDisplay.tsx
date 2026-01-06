import { Copy, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';

import { useToast } from '../../hooks/useToast';
import engineApi from '../../services/engineApi';
import { Button } from '../ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/Dialog';
import { Input } from '../ui/Input';

interface AgentAPIKeyDisplayProps {
  projectId: string;
  onKeyChange?: (key: string) => void;
}

export default function AgentAPIKeyDisplay({ projectId, onKeyChange }: AgentAPIKeyDisplayProps) {
  const { toast } = useToast();
  const [key, setKey] = useState<string>('');
  const [masked, setMasked] = useState(true);
  const [loading, setLoading] = useState(true);
  const [regenerating, setRegenerating] = useState(false);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);

  // Fetch initial key (masked)
  useEffect(() => {
    let cancelled = false;

    const fetchKey = async () => {
      try {
        setLoading(true);
        const response = await engineApi.getAgentAPIKey(projectId, false);
        if (!cancelled && response.success && response.data) {
          setKey(response.data.key);
          setMasked(response.data.masked);
        }
      } catch (error) {
        if (!cancelled) {
          console.error('Failed to fetch agent API key:', error);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    void fetchKey();
    return () => {
      cancelled = true;
    };
  }, [projectId]);

  // Reveal key on focus
  const handleFocus = async (e: React.FocusEvent<HTMLInputElement>) => {
    if (masked) {
      try {
        const response = await engineApi.getAgentAPIKey(projectId, true);
        if (response.success && response.data) {
          setKey(response.data.key);
          setMasked(false);
          // Auto-select text
          e.target.select();
        }
      } catch (error) {
        console.error('Failed to reveal agent API key:', error);
      }
    } else {
      // Already revealed, just select
      e.target.select();
    }
  };

  // Mask key on blur
  const handleBlur = () => {
    if (!masked) {
      setMasked(true);
    }
  };

  // Copy to clipboard
  const handleCopy = async () => {
    try {
      // Fetch unmasked key if needed
      let keyToCopy = key;
      if (masked) {
        const response = await engineApi.getAgentAPIKey(projectId, true);
        if (response.success && response.data) {
          keyToCopy = response.data.key;
        }
      }

      await navigator.clipboard.writeText(keyToCopy);
      toast({
        title: 'Copied',
        description: 'Agent API key copied to clipboard',
        variant: 'success',
      });
    } catch {
      toast({
        title: 'Error',
        description: 'Failed to copy API key',
        variant: 'destructive',
      });
    }
  };

  // Regenerate key
  const handleRegenerate = async () => {
    setConfirmDialogOpen(false);

    try {
      setRegenerating(true);
      const response = await engineApi.regenerateAgentAPIKey(projectId);
      if (response.success && response.data) {
        setKey(response.data.key);
        setMasked(false);
        onKeyChange?.(response.data.key);
        toast({
          title: 'Key Rotated',
          description: 'Agent API key has been rotated',
          variant: 'success',
        });
      }
    } catch {
      toast({
        title: 'Error',
        description: 'Failed to rotate API key',
        variant: 'destructive',
      });
    } finally {
      setRegenerating(false);
    }
  };

  // Generate a masked display value that matches the full key length
  const MASKED_KEY_LENGTH = 64; // Standard key length
  const maskedDisplay = '*'.repeat(MASKED_KEY_LENGTH);

  if (loading) {
    return <div className="text-sm text-text-secondary">Loading...</div>;
  }

  return (
    <div className="space-y-3">
      <div>
        <h4 className="text-sm font-medium text-text-primary">AI Agent API Key</h4>
        <p className="text-xs text-text-secondary mt-0.5">
          This API Key enables AI Agents to access Pre-Approved Queries.
        </p>
      </div>
      <div className="flex items-center gap-2">
        <Input
          type="text"
          value={masked ? maskedDisplay : key}
          onFocus={handleFocus}
          onBlur={handleBlur}
          readOnly
          className="flex-1 font-mono text-sm"
        />
        <Button
          size="icon"
          variant="outline"
          onClick={handleCopy}
          title="Copy to clipboard"
        >
          <Copy className="h-4 w-4" />
        </Button>
        <Button
          size="icon"
          variant="outline"
          onClick={() => setConfirmDialogOpen(true)}
          disabled={regenerating}
          title="Rotate key"
        >
          <RefreshCw className={`h-4 w-4 ${regenerating ? 'animate-spin' : ''}`} />
        </Button>
      </div>
      <p className="text-xs text-text-secondary">
        Click the key to reveal.
      </p>

      {/* Confirmation Dialog */}
      <Dialog open={confirmDialogOpen} onOpenChange={setConfirmDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rotate API Key?</DialogTitle>
            <DialogDescription>
              This will reset the API key. All previously configured Agents will fail to authenticate until updated with the new key.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDialogOpen(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleRegenerate}>
              Rotate Key
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
