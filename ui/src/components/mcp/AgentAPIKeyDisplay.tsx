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
}

export default function AgentAPIKeyDisplay({ projectId }: AgentAPIKeyDisplayProps) {
  const { toast } = useToast();
  const [key, setKey] = useState<string>('');
  const [masked, setMasked] = useState(true);
  const [loading, setLoading] = useState(true);
  const [regenerating, setRegenerating] = useState(false);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);

  // Fetch initial key (masked)
  useEffect(() => {
    const fetchKey = async () => {
      try {
        setLoading(true);
        const response = await engineApi.getAgentAPIKey(projectId, false);
        if (response.success && response.data) {
          setKey(response.data.key);
          setMasked(response.data.masked);
        }
      } catch (error) {
        console.error('Failed to fetch agent API key:', error);
      } finally {
        setLoading(false);
      }
    };

    void fetchKey();
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
    } catch (error) {
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
        toast({
          title: 'Key Regenerated',
          description: 'Agent API key has been regenerated',
          variant: 'success',
        });
      }
    } catch (error) {
      toast({
        title: 'Error',
        description: 'Failed to regenerate API key',
        variant: 'destructive',
      });
    } finally {
      setRegenerating(false);
    }
  };

  if (loading) {
    return <div className="text-sm text-text-secondary">Loading...</div>;
  }

  return (
    <div className="space-y-2">
      <label className="text-sm font-medium text-text-primary">Agent API Key</label>
      <div className="flex items-center gap-2">
        <Input
          type="text"
          value={key}
          onFocus={handleFocus}
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
          title="Regenerate key"
        >
          <RefreshCw className={`h-4 w-4 ${regenerating ? 'animate-spin' : ''}`} />
        </Button>
      </div>
      <p className="text-xs text-text-secondary">
        Click the key to reveal. Use this key for agent authentication.
      </p>

      {/* Confirmation Dialog */}
      <Dialog open={confirmDialogOpen} onOpenChange={setConfirmDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Regenerate API Key?</DialogTitle>
            <DialogDescription>
              This will reset the API key. All previously configured Agents will fail to authenticate until updated with the new key.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDialogOpen(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleRegenerate}>
              Regenerate Key
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
