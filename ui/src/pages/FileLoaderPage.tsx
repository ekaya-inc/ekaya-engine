import {
  FileSpreadsheet,
  FileText,
  Loader2,
  Trash2,
  Upload,
} from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import AppPageHeader from '../components/AppPageHeader';
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

interface LoadStatus {
  id: string;
  project_id: string;
  app_id: string;
  file_name: string;
  table_name: string;
  rows_attempted: number;
  rows_loaded: number;
  rows_skipped: number;
  errors: string[];
  started_at: string;
  completed_at: string | null;
  status: string;
}

interface LoadResult {
  table_name: string;
  rows_attempted: number;
  rows_loaded: number;
  rows_skipped: number;
  errors: string[];
}

const FileLoaderPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [loadHistory, setLoadHistory] = useState<LoadStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);

  // Uninstall state
  const [confirmText, setConfirmText] = useState('');
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);

  const acceptedExtensions = '.csv,.tsv,.txt,.xlsx,.xlsm,.xltx';

  const fetchHistory = useCallback(async () => {
    if (!pid) return;
    try {
      const response = await engineApi.etlGetLoadHistory<LoadStatus[]>(pid);
      setLoadHistory(response.data ?? []);
    } catch {
      // Ignore errors on initial load
    } finally {
      setLoading(false);
    }
  }, [pid]);

  useEffect(() => {
    fetchHistory();
  }, [fetchHistory]);

  const handleUpload = async (file: File) => {
    if (!pid) return;
    setUploading(true);

    try {
      const response = await engineApi.etlUploadFile<LoadResult>(pid, file);

      if (response.data) {
        toast({
          title: 'File loaded',
          description: `Loaded ${response.data.rows_loaded} rows into ${response.data.table_name}`,
        });
        await fetchHistory();
      }
    } catch (err) {
      toast({
        title: 'Upload failed',
        description: err instanceof Error ? err.message : 'Failed to upload file',
        variant: 'destructive',
      });
    } finally {
      setUploading(false);
    }
  };

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) handleUpload(file);
    // Reset the input so the same file can be re-selected
    if (fileInputRef.current) fileInputRef.current.value = '';
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer.files[0];
    if (file) handleUpload(file);
  };

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString();
  };

  const statusBadge = (status: string) => {
    const colors: Record<string, string> = {
      completed: 'bg-green-100 text-green-800',
      running: 'bg-blue-100 text-blue-800',
      failed: 'bg-red-100 text-red-800',
      pending: 'bg-yellow-100 text-yellow-800',
    };
    return (
      <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${colors[status] ?? 'bg-gray-100 text-gray-800'}`}>
        {status}
      </span>
    );
  };

  const handleUninstall = async () => {
    if (confirmText !== 'uninstall application' || !pid) return;

    setIsUninstalling(true);
    try {
      const response = await engineApi.uninstallApp(pid, 'file-loader');
      if (response.error) {
        toast({
          title: 'Error',
          description: response.error,
          variant: 'destructive',
        });
        return;
      }
      if (response.data?.redirectUrl) {
        window.location.href = response.data.redirectUrl;
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
      <AppPageHeader
        title="Spreadsheet Loader [BETA]"
        slug="file-loader"
        icon={<FileSpreadsheet className="h-8 w-8" />}
        description="Import CSV, TSV, and Excel files into your SQL database"
        showInfoLink={false}
      />

      {/* Upload dropzone */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Upload File</CardTitle>
          <CardDescription>
            Drag and drop a file or click to browse. The file will be parsed, schema inferred, and loaded into your database.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div
            className={`flex flex-col items-center justify-center rounded-lg border-2 border-dashed p-8 transition-colors ${
              dragOver
                ? 'border-blue-400 bg-blue-50 dark:bg-blue-950/20'
                : 'border-gray-300 hover:border-gray-400 dark:border-gray-700'
            }`}
            onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
            onDragLeave={() => setDragOver(false)}
            onDrop={handleDrop}
          >
            {uploading ? (
              <>
                <Loader2 className="h-8 w-8 animate-spin text-blue-500 mb-2" />
                <p className="text-sm text-text-secondary">Processing file...</p>
              </>
            ) : (
              <>
                <Upload className="h-8 w-8 text-gray-400 mb-2" />
                <p className="text-sm text-text-secondary mb-2">
                  Drop your file here, or{' '}
                  <button
                    className="text-blue-500 hover:underline"
                    onClick={() => fileInputRef.current?.click()}
                  >
                    browse
                  </button>
                </p>
                <p className="text-xs text-gray-400">
                  Accepted: {acceptedExtensions}
                </p>
              </>
            )}
            <input
              ref={fileInputRef}
              type="file"
              accept={acceptedExtensions}
              className="hidden"
              onChange={handleFileSelect}
            />
          </div>
        </CardContent>
      </Card>

      {/* Load history */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Load History</CardTitle>
          <CardDescription>Recent file imports</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="flex justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
            </div>
          ) : loadHistory.length === 0 ? (
            <div className="flex flex-col items-center py-8 text-center">
              <FileText className="h-8 w-8 text-gray-300 mb-2" />
              <p className="text-sm text-text-secondary">No files loaded yet</p>
              <p className="text-xs text-gray-400 mt-1">
                Upload a file above to get started
              </p>
            </div>
          ) : (
            <div className="divide-y">
              {loadHistory.map((entry) => (
                <div key={entry.id} className="flex items-center justify-between py-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">{entry.file_name}</span>
                      {statusBadge(entry.status)}
                    </div>
                    <div className="mt-0.5 text-xs text-text-secondary">
                      <span className="font-mono">{entry.table_name}</span>
                      {' · '}
                      {entry.rows_loaded} rows loaded
                      {entry.rows_skipped > 0 && `, ${entry.rows_skipped} skipped`}
                      {' · '}
                      {formatDate(entry.started_at)}
                    </div>
                    {entry.errors && entry.errors.length > 0 && (
                      <div className="mt-1 text-xs text-red-500 truncate">
                        {entry.errors[0]}
                        {entry.errors.length > 1 && ` (+${entry.errors.length - 1} more)`}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
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
              <CardDescription>Remove Spreadsheet Loader from this project</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-text-secondary mb-4">
            Uninstalling Spreadsheet Loader will remove the file import capability from this project.
            Previously loaded data will remain in your database.
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
      <Dialog
        open={showUninstallDialog}
        onOpenChange={(open) => {
          setShowUninstallDialog(open);
          if (!open) {
            setConfirmText('');
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Uninstall Spreadsheet Loader?</DialogTitle>
            <DialogDescription>
              This will remove the file import capability from this project.
              Previously loaded data will remain in your database.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <label className="text-sm font-medium text-text-primary">
              Type{' '}
              <span className="font-mono bg-gray-100 dark:bg-gray-800 px-1 rounded">
                uninstall application
              </span>{' '}
              to confirm
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

export default FileLoaderPage;
