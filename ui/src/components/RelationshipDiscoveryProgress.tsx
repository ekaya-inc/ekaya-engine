import { X, Loader2, CheckCircle2, AlertCircle } from "lucide-react";
import { useState, useEffect, useCallback } from "react";

import sdapApi from "../services/sdapApi";
import type { DiscoveryResults } from "../types";

import { Button } from "./ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "./ui/Card";

interface RelationshipDiscoveryProgressProps {
  projectId: string;
  isOpen: boolean;
  onClose: () => void;
  onComplete: () => void;
}

/**
 * RelationshipDiscoveryProgress - Shows a loading modal during relationship discovery
 * Discovery runs synchronously - no polling needed
 */
export const RelationshipDiscoveryProgress = ({
  projectId,
  isOpen,
  onClose,
  onComplete,
}: RelationshipDiscoveryProgressProps): React.ReactElement | null => {
  const [isLoading, setIsLoading] = useState(false);
  const [results, setResults] = useState<DiscoveryResults | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Run discovery
  const runDiscovery = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);
      setResults(null);

      const response = await sdapApi.discoverRelationships(projectId);

      if (response.data) {
        setResults(response.data);
        // Notify parent that discovery is complete (refreshes schema)
        onComplete();
      } else if (response.error) {
        setError(response.error);
      }
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : "Failed to discover relationships";
      console.error("Discovery failed:", errorMessage);
      setError(errorMessage);
    } finally {
      setIsLoading(false);
    }
  }, [projectId, onComplete]);

  // Start discovery when modal opens
  useEffect(() => {
    if (isOpen && !isLoading && results === null && error === null) {
      runDiscovery();
    }
  }, [isOpen, isLoading, results, error, runDiscovery]);

  // Reset state when modal closes
  useEffect(() => {
    if (!isOpen) {
      setResults(null);
      setError(null);
      setIsLoading(false);
    }
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <Card className="w-full max-w-md mx-4">
        <CardHeader className="flex flex-row items-start justify-between space-y-0">
          <div>
            <CardTitle className="flex items-center gap-2">
              {isLoading && (
                <Loader2 className="h-5 w-5 animate-spin text-amber-500" />
              )}
              {results !== null && (
                <CheckCircle2 className="h-5 w-5 text-green-500" />
              )}
              {error !== null && (
                <AlertCircle className="h-5 w-5 text-red-500" />
              )}
              Finding Relationships
            </CardTitle>
            <CardDescription>
              {isLoading && "Analyzing schema and discovering relationships..."}
              {results !== null && "Discovery complete!"}
              {error !== null && "Discovery failed"}
            </CardDescription>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="h-8 w-8 p-0"
            onClick={onClose}
            disabled={isLoading}
          >
            <X className="h-4 w-4" />
            <span className="sr-only">Close</span>
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Loading state */}
          {isLoading && (
            <div className="flex flex-col items-center justify-center py-8 space-y-4">
              <Loader2 className="h-12 w-12 animate-spin text-amber-500" />
              <p className="text-sm text-muted-foreground text-center">
                This may take a few seconds depending on the size of your schema...
              </p>
            </div>
          )}

          {/* Success results */}
          {results !== null && (
            <div className="rounded-lg bg-green-50 dark:bg-green-950/20 p-4">
              <div className="flex items-center gap-2 text-green-700 dark:text-green-300 mb-2">
                <CheckCircle2 className="h-4 w-4" />
                <span className="font-medium">Discovery completed successfully</span>
              </div>
              <div className="grid grid-cols-2 gap-2 text-sm text-green-600 dark:text-green-400">
                <div>Relationships created: {results.relationships_created}</div>
                <div>Tables analyzed: {results.tables_analyzed}</div>
                <div>Columns analyzed: {results.columns_analyzed}</div>
                <div>Tables without relationships: {results.tables_without_relationships}</div>
              </div>
            </div>
          )}

          {/* Error state */}
          {error !== null && (
            <div className="rounded-lg bg-red-50 dark:bg-red-950/20 p-4">
              <div className="flex items-center gap-2 text-red-700 dark:text-red-300 mb-2">
                <AlertCircle className="h-4 w-4" />
                <span className="font-medium">Discovery failed</span>
              </div>
              <p className="text-sm text-red-600 dark:text-red-400">
                {error}
              </p>
            </div>
          )}

          {/* Actions */}
          {!isLoading && (
            <div className="flex justify-end gap-2 pt-2">
              {error !== null && (
                <Button variant="outline" onClick={runDiscovery}>
                  Retry
                </Button>
              )}
              <Button variant="default" onClick={onClose}>
                Close
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
};
