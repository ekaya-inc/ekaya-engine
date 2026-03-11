import {
  ArrowLeft,
  FileText,
  Loader2,
  Upload,
} from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { Button } from '../components/ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/Card';
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

interface ETLAppPageProps {
  appId: string;
  title: string;
  description: string;
  acceptedExtensions: string;
}

const ETLAppPage = ({ appId, title, description, acceptedExtensions }: ETLAppPageProps) => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { toast } = useToast();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [loadHistory, setLoadHistory] = useState<LoadStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);

  const fetchHistory = useCallback(async () => {
    if (!pid) return;
    try {
      const response = await engineApi.etlGetLoadHistory<LoadStatus[]>(pid, appId);
      setLoadHistory(response.data ?? []);
    } catch {
      // Ignore errors on initial load
    } finally {
      setLoading(false);
    }
  }, [pid, appId]);

  useEffect(() => {
    fetchHistory();
  }, [fetchHistory]);

  const handleUpload = async (file: File) => {
    if (!pid) return;
    setUploading(true);

    try {
      const response = await engineApi.etlUploadFile<LoadResult>(pid, appId, file);

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

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          aria-label="Back to applications"
          onClick={() => navigate(`/projects/${pid}/applications`)}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">{title}</h1>
          <p className="text-text-secondary text-sm">{description}</p>
        </div>
      </div>

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
    </div>
  );
};

// Wrapper components for each applet
export const ETLCSVPage = () => (
  <ETLAppPage
    appId="etl-csv"
    title="CSV/TSV Loader"
    description="Import CSV and TSV files into your SQL database"
    acceptedExtensions=".csv,.tsv,.txt"
  />
);

export const ETLExcelPage = () => (
  <ETLAppPage
    appId="etl-excel"
    title="Excel Loader"
    description="Import XLSX spreadsheets into your SQL database"
    acceptedExtensions=".xlsx,.xlsm,.xltx"
  />
);

export default ETLAppPage;
