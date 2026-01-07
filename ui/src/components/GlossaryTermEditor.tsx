/**
 * GlossaryTermEditor Component
 * Modal form for creating and editing glossary terms
 */

import { AlertCircle, CheckCircle2, Loader2, X } from 'lucide-react';
import { useState, useEffect } from 'react';

import engineApi from '../services/engineApi';
import type { GlossaryTerm, TestSQLResult, OutputColumn } from '../types';

import { SqlEditor } from './SqlEditor';
import { Button } from './ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog';
import { Input } from './ui/Input';

interface GlossaryTermEditorProps {
  projectId: string;
  term?: GlossaryTerm | null;
  isOpen: boolean;
  onClose: () => void;
  onSave: () => void;
}

export function GlossaryTermEditor({
  projectId,
  term,
  isOpen,
  onClose,
  onSave,
}: GlossaryTermEditorProps) {
  const isEditing = !!term;

  // Form state
  const [termName, setTermName] = useState('');
  const [definition, setDefinition] = useState('');
  const [definingSql, setDefiningSql] = useState('');
  const [baseTable, setBaseTable] = useState('');
  const [aliases, setAliases] = useState<string[]>([]);
  const [aliasInput, setAliasInput] = useState('');

  // Test SQL state
  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestSQLResult | null>(null);
  const [sqlTested, setSqlTested] = useState(false);

  // Save state
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  // Initialize form when term changes or dialog opens
  useEffect(() => {
    if (isOpen) {
      if (term) {
        setTermName(term.term);
        setDefinition(term.definition);
        setDefiningSql(term.defining_sql);
        setBaseTable(term.base_table || '');
        setAliases(term.aliases || []);
        setSqlTested(true); // Existing terms already have tested SQL
        setTestResult({
          valid: true,
          output_columns: term.output_columns || [],
        });
      } else {
        // Reset for new term
        setTermName('');
        setDefinition('');
        setDefiningSql('');
        setBaseTable('');
        setAliases([]);
        setAliasInput('');
        setSqlTested(false);
        setTestResult(null);
      }
      setSaveError(null);
    }
  }, [isOpen, term]);

  // Reset SQL tested flag when SQL changes
  // Only reset if we have actual SQL to compare (skip on initial mount)
  useEffect(() => {
    if (term && definingSql && definingSql !== term.defining_sql) {
      setSqlTested(false);
      setTestResult(null);
    } else if (!term && definingSql) {
      // For new terms, reset when SQL changes
      setSqlTested(false);
      setTestResult(null);
    }
  }, [definingSql, term]);

  const handleTestSQL = async () => {
    if (!definingSql.trim()) {
      setTestResult({
        valid: false,
        error: 'SQL is required',
      });
      return;
    }

    setIsTesting(true);
    setSaveError(null);

    try {
      const response = await engineApi.testGlossarySQL(projectId, definingSql);

      if (response.success && response.data) {
        setTestResult(response.data);
        setSqlTested(response.data.valid);
      } else {
        setTestResult({
          valid: false,
          error: response.error || 'Failed to test SQL',
        });
        setSqlTested(false);
      }
    } catch (err) {
      setTestResult({
        valid: false,
        error: err instanceof Error ? err.message : 'Failed to test SQL',
      });
      setSqlTested(false);
    } finally {
      setIsTesting(false);
    }
  };

  const handleAddAlias = () => {
    const trimmed = aliasInput.trim();
    if (trimmed && !aliases.includes(trimmed)) {
      setAliases([...aliases, trimmed]);
      setAliasInput('');
    }
  };

  const handleRemoveAlias = (alias: string) => {
    setAliases(aliases.filter((a) => a !== alias));
  };

  const handleAliasKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleAddAlias();
    }
  };

  const handleSave = async () => {
    // Validation
    if (!termName.trim()) {
      setSaveError('Term name is required');
      return;
    }

    if (!definition.trim()) {
      setSaveError('Definition is required');
      return;
    }

    if (!definingSql.trim()) {
      setSaveError('SQL is required');
      return;
    }

    if (!sqlTested || !testResult?.valid) {
      setSaveError('Please test the SQL and ensure it is valid before saving');
      return;
    }

    setIsSaving(true);
    setSaveError(null);

    try {
      if (isEditing) {
        const updateRequest: {
          term: string;
          definition: string;
          defining_sql: string;
          base_table?: string;
          aliases?: string[];
        } = {
          term: termName,
          definition,
          defining_sql: definingSql,
        };

        if (baseTable) {
          updateRequest.base_table = baseTable;
        }

        if (aliases.length > 0) {
          updateRequest.aliases = aliases;
        }

        const response = await engineApi.updateGlossaryTerm(projectId, term.id, updateRequest);

        if (!response.success) {
          setSaveError(response.error || 'Failed to update term');
          return;
        }
      } else {
        const createRequest: {
          term: string;
          definition: string;
          defining_sql: string;
          base_table?: string;
          aliases?: string[];
        } = {
          term: termName,
          definition,
          defining_sql: definingSql,
        };

        if (baseTable) {
          createRequest.base_table = baseTable;
        }

        if (aliases.length > 0) {
          createRequest.aliases = aliases;
        }

        const response = await engineApi.createGlossaryTerm(projectId, createRequest);

        if (!response.success) {
          setSaveError(response.error || 'Failed to create term');
          return;
        }
      }

      // Success - notify parent and close
      onSave();
      onClose();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save term');
    } finally {
      setIsSaving(false);
    }
  };

  const canSave =
    termName.trim() &&
    definition.trim() &&
    definingSql.trim() &&
    sqlTested &&
    testResult?.valid;

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-4xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {isEditing ? 'Edit Glossary Term' : 'Add Glossary Term'}
          </DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Update the business term definition and SQL query.'
              : 'Define a new business term with executable SQL.'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {/* Term Name */}
          <div>
            <label
              htmlFor="term-name"
              className="block text-sm font-medium text-text-primary mb-1"
            >
              Term Name
            </label>
            <Input
              id="term-name"
              value={termName}
              onChange={(e) => setTermName(e.target.value)}
              placeholder="e.g., Active Users, Monthly Revenue"
              disabled={isSaving}
            />
          </div>

          {/* Definition */}
          <div>
            <label
              htmlFor="definition"
              className="block text-sm font-medium text-text-primary mb-1"
            >
              Definition
            </label>
            <textarea
              id="definition"
              value={definition}
              onChange={(e) => setDefinition(e.target.value)}
              placeholder="Describe what this term means in business context..."
              rows={3}
              disabled={isSaving}
              className="flex w-full rounded-md border border-border-medium bg-surface-primary px-3 py-2 text-sm text-text-primary ring-offset-surface-primary placeholder:text-text-tertiary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 resize-none"
            />
          </div>

          {/* Defining SQL */}
          <div>
            <label
              htmlFor="defining-sql"
              className="block text-sm font-medium text-text-primary mb-1"
            >
              Defining SQL
            </label>
            <SqlEditor
              value={definingSql}
              onChange={setDefiningSql}
              dialect="PostgreSQL"
              placeholder="SELECT COUNT(DISTINCT user_id) AS active_users FROM users WHERE ..."
              minHeight="200px"
              readOnly={isSaving}
            />
            <div className="mt-2 flex items-center gap-2">
              <Button
                onClick={handleTestSQL}
                disabled={!definingSql.trim() || isTesting || isSaving}
                variant="outline"
                size="sm"
              >
                {isTesting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Testing...
                  </>
                ) : (
                  'Test SQL'
                )}
              </Button>

              {testResult && (
                <div className="flex items-center gap-2">
                  {testResult.valid ? (
                    <>
                      <CheckCircle2 className="h-4 w-4 text-green-500" />
                      <span className="text-sm text-green-600 dark:text-green-400">
                        SQL is valid
                      </span>
                    </>
                  ) : (
                    <>
                      <AlertCircle className="h-4 w-4 text-red-500" />
                      <span className="text-sm text-red-600 dark:text-red-400">
                        {testResult.error || 'SQL is invalid'}
                      </span>
                    </>
                  )}
                </div>
              )}
            </div>
          </div>

          {/* Output Columns (shown after successful test) */}
          {testResult?.valid && testResult.output_columns && testResult.output_columns.length > 0 && (
            <div>
              <label className="block text-sm font-medium text-text-primary mb-2">
                Output Columns
              </label>
              <div className="rounded-md border border-border-light bg-surface-secondary/30 p-3 space-y-1">
                {testResult.output_columns.map((col: OutputColumn, idx: number) => (
                  <div key={idx} className="text-sm">
                    <span className="font-mono text-text-primary">{col.name}</span>
                    <span className="text-text-tertiary"> ({col.type})</span>
                    {col.description && (
                      <span className="text-text-secondary"> - {col.description}</span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Base Table (optional) */}
          <div>
            <label
              htmlFor="base-table"
              className="block text-sm font-medium text-text-primary mb-1"
            >
              Base Table <span className="text-text-tertiary">(optional)</span>
            </label>
            <Input
              id="base-table"
              value={baseTable}
              onChange={(e) => setBaseTable(e.target.value)}
              placeholder="e.g., users, transactions"
              disabled={isSaving}
            />
            <p className="mt-1 text-xs text-text-tertiary">
              Primary table used in the SQL query
            </p>
          </div>

          {/* Aliases */}
          <div>
            <label
              htmlFor="aliases"
              className="block text-sm font-medium text-text-primary mb-1"
            >
              Aliases <span className="text-text-tertiary">(optional)</span>
            </label>
            <div className="space-y-2">
              <div className="flex gap-2">
                <Input
                  id="aliases"
                  value={aliasInput}
                  onChange={(e) => setAliasInput(e.target.value)}
                  onKeyDown={handleAliasKeyDown}
                  placeholder="e.g., MAU, Monthly Active Users"
                  disabled={isSaving}
                />
                <Button
                  onClick={handleAddAlias}
                  disabled={!aliasInput.trim() || isSaving}
                  variant="outline"
                  size="sm"
                >
                  Add
                </Button>
              </div>

              {aliases.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {aliases.map((alias) => (
                    <span
                      key={alias}
                      className="inline-flex items-center gap-1 px-2 py-1 rounded text-sm bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300"
                    >
                      {alias}
                      <button
                        onClick={() => handleRemoveAlias(alias)}
                        disabled={isSaving}
                        className="hover:text-purple-900 dark:hover:text-purple-100"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Error Message */}
          {saveError && (
            <div className="rounded-md bg-red-50 dark:bg-red-900/20 p-3 flex items-start gap-2">
              <AlertCircle className="h-5 w-5 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />
              <p className="text-sm text-red-600 dark:text-red-400">{saveError}</p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            onClick={onClose}
            disabled={isSaving}
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            onClick={handleSave}
            disabled={!canSave || isSaving}
          >
            {isSaving ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              isEditing ? 'Update Term' : 'Create Term'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
