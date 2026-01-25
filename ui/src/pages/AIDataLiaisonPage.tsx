import { ArrowLeft, Loader2, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { Button } from '../components/ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/Card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../components/ui/Dialog';
import { Input } from '../components/ui/Input';
import { useToast } from '../hooks/useToast';
import engineApi from '../services/engineApi';

const AIDataLiaisonPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();
  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  const handleUninstall = async () => {
    if (confirmText !== 'uninstall application' || !pid) return;

    setIsUninstalling(true);
    try {
      const response = await engineApi.uninstallApp(pid, 'ai-data-liaison');
      if (response.error) {
        toast({
          title: 'Error',
          description: response.error,
          variant: 'destructive',
        });
        return;
      }
      navigate(`/projects/${pid}`);
    } catch (error) {
      toast({
        title: 'Error',
        description: error instanceof Error ? error.message : 'Failed to uninstall application',
        variant: 'destructive',
      });
    } finally {
      setIsUninstalling(false);
    }
  };

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header with back button */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          aria-label="Back to project dashboard"
          onClick={() => navigate(`/projects/${pid}`)}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">AI Data Liaison</h1>
          <p className="text-text-secondary">
            Configure your AI Data Liaison application
          </p>
        </div>
      </div>

      {/* Configuration placeholder card */}
      <Card>
        <CardHeader>
          <CardTitle>Configuration</CardTitle>
          <CardDescription>
            AI Data Liaison configuration options will appear here.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-text-secondary text-sm">
            Coming soon: Configure how AI Data Liaison connects to your data and serves your business users.
          </p>
        </CardContent>
      </Card>

      {/* Danger Zone */}
      <Card className="border-red-200 dark:border-red-900">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-red-500/10">
              <Trash2 className="h-5 w-5 text-red-500" />
            </div>
            <div>
              <CardTitle className="text-red-600 dark:text-red-400">Danger Zone</CardTitle>
              <CardDescription>
                Remove AI Data Liaison from this project
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-text-secondary mb-4">
            Uninstalling AI Data Liaison will remove this application from your project and clear all its settings. This action cannot be undone.
          </p>
          <Button
            variant="outline"
            onClick={() => setShowUninstallDialog(true)}
            className="text-red-600 hover:text-red-700 hover:bg-red-50 border-red-300"
          >
            <Trash2 className="mr-2 h-4 w-4" />
            Uninstall Application
          </Button>
        </CardContent>
      </Card>

      {/* Uninstall Confirmation Dialog */}
      <Dialog open={showUninstallDialog} onOpenChange={(open) => {
        setShowUninstallDialog(open);
        if (!open) {
          setConfirmText('');
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Uninstall AI Data Liaison?</DialogTitle>
            <DialogDescription>
              This will remove the AI Data Liaison application from your project and clear all its settings. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <label className="text-sm font-medium text-text-primary">
              Type <span className="font-mono bg-gray-100 dark:bg-gray-800 px-1 rounded">uninstall application</span> to confirm
            </label>
            <Input
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder="uninstall application"
              className="mt-2"
              disabled={isUninstalling}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowUninstallDialog(false)}
              disabled={isUninstalling}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleUninstall}
              disabled={confirmText !== 'uninstall application' || isUninstalling}
            >
              {isUninstalling ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Uninstalling...
                </>
              ) : (
                'Uninstall Application'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default AIDataLiaisonPage;
