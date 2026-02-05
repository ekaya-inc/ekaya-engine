/**
 * OntologyPage - DAG-based Ontology Extraction UI
 *
 * This page displays the unified ontology extraction workflow using a DAG (Directed Acyclic Graph)
 * visualization. Users can start, monitor, and cancel the extraction process from here.
 */

import { AlertTriangle, ArrowLeft, Brain, RefreshCw } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import OntologyDAG from '../components/ontology/OntologyDAG';
import { Button } from '../components/ui/Button';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';

const OntologyPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { selectedDatasource } = useDatasourceConnection();

  const [error, setError] = useState<string | null>(null);
  const [hasOntology, setHasOntology] = useState(false);

  // Handler for DAG completion
  const handleComplete = useCallback(() => {
    // Could trigger a refresh of other UI components or show a success message
    console.log('Ontology extraction complete');
  }, []);

  // Handler for DAG errors
  const handleError = useCallback((errorMessage: string) => {
    setError(errorMessage);
  }, []);

  // Handler for retry after error
  const handleRetry = useCallback(() => {
    setError(null);
  }, []);

  // Handler for ontology status changes
  const handleStatusChange = useCallback((hasOntologyData: boolean) => {
    setHasOntology(hasOntologyData);
  }, []);

  // No datasource selected
  if (!selectedDatasource?.datasourceId) {
    return (
      <div className="mx-auto max-w-7xl">
        <div className="mb-6">
          <Button
            variant="ghost"
            onClick={() => navigate(`/projects/${pid}`)}
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>

        <div className="rounded-lg border border-amber-200 bg-amber-50 shadow-sm p-12">
          <AlertTriangle className="h-16 w-16 text-amber-400 mx-auto mb-4" />
          <h2 className="text-xl font-semibold text-text-primary mb-2 text-center">
            No Datasource Selected
          </h2>
          <p className="text-text-secondary max-w-3xl mx-auto mb-6 text-center">
            Please select a datasource from the project dashboard before starting ontology extraction.
          </p>
          <div className="text-center">
            <Button
              variant="outline"
              onClick={() => navigate(`/projects/${pid}`)}
            >
              Go to Dashboard
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl">
      {/* Header */}
      <div className="mb-6">
        <div className="mb-4">
          <Button
            variant="ghost"
            onClick={() => navigate(`/projects/${pid}`)}
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>

        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-text-primary flex items-center gap-2">
              <Brain className="h-8 w-8 text-purple-500" />
              Ontology Extraction
            </h1>
            <p className="mt-2 text-text-secondary">
              Extract business knowledge from your database schema
            </p>
          </div>
        </div>

        {/* Error banner */}
        {error && (
          <div className="mt-4 rounded-lg border border-red-200 bg-red-50 p-4 flex items-center justify-between">
            <p className="text-red-800 text-sm">{error}</p>
            <Button
              variant="ghost"
              size="sm"
              onClick={handleRetry}
              className="text-red-600 hover:text-red-700"
            >
              <RefreshCw className="h-4 w-4 mr-1" />
              Dismiss
            </Button>
          </div>
        )}
      </div>

      {/* Main content - DAG visualization */}
      {pid && selectedDatasource.datasourceId && (
        <OntologyDAG
          projectId={pid}
          datasourceId={selectedDatasource.datasourceId}
          onComplete={handleComplete}
          onError={handleError}
          onStatusChange={handleStatusChange}
        />
      )}

      {/* Info panel - only show when no ontology exists */}
      {!hasOntology && (
        <div className="mt-6 rounded-lg border border-purple-200 bg-purple-50 p-4">
          <p className="text-purple-800 text-sm">
            <strong>How it works:</strong> The extraction process runs automatically through multiple
            steps—discovering relationships, enriching columns with semantic meaning,
            and building a complete domain model. You can leave this page and return later to check progress.
          </p>
        </div>
      )}
    </div>
  );
};

export default OntologyPage;
