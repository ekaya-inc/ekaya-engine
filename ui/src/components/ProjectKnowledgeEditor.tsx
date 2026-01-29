/**
 * ProjectKnowledgeEditor Component
 * Modal form for creating and editing project knowledge facts
 * For creating: simple text input that gets parsed by LLM
 * For editing: full form with all fields
 */

import { AlertCircle, Loader2 } from 'lucide-react';
import { useState, useEffect } from 'react';

import engineApi from '../services/engineApi';
import type { ProjectKnowledge } from '../types';

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

/**
 * Common fact types for the dropdown (used in edit mode)
 */
const COMMON_FACT_TYPES = [
  { value: 'business_rule', label: 'Business Rule' },
  { value: 'convention', label: 'Convention' },
  { value: 'domain_term', label: 'Domain Term' },
  { value: 'relationship', label: 'Relationship' },
];

interface ProjectKnowledgeEditorProps {
  projectId: string;
  fact?: ProjectKnowledge | null;
  isOpen: boolean;
  onClose: () => void;
  onSave: () => void;
  onProcessing?: () => void; // Called when async processing starts (for toast)
}

export function ProjectKnowledgeEditor({
  projectId,
  fact,
  isOpen,
  onClose,
  onSave,
  onProcessing,
}: ProjectKnowledgeEditorProps) {
  const isEditing = !!fact;

  // Form state for CREATE mode (simple text input)
  const [freeFormText, setFreeFormText] = useState('');

  // Form state for EDIT mode (full form)
  const [factType, setFactType] = useState('business_rule');
  const [customFactType, setCustomFactType] = useState('');
  const [useCustomType, setUseCustomType] = useState(false);
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [context, setContext] = useState('');

  // Save state
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  // Initialize form when fact changes or dialog opens
  useEffect(() => {
    if (isOpen) {
      if (fact) {
        // Edit mode: populate full form
        const isCommonType = COMMON_FACT_TYPES.some(t => t.value === fact.fact_type);
        if (isCommonType) {
          setFactType(fact.fact_type);
          setUseCustomType(false);
          setCustomFactType('');
        } else {
          setFactType('');
          setUseCustomType(true);
          setCustomFactType(fact.fact_type);
        }
        setKey(fact.key);
        setValue(fact.value);
        setContext(fact.context ?? '');
      } else {
        // Create mode: reset to simple form
        setFreeFormText('');
        setFactType('business_rule');
        setUseCustomType(false);
        setCustomFactType('');
        setKey('');
        setValue('');
        setContext('');
      }
      setSaveError(null);
    }
  }, [isOpen, fact]);

  const handleCreate = async () => {
    const text = freeFormText.trim();
    if (!text) {
      setSaveError('Please enter a fact');
      return;
    }

    setIsSaving(true);
    setSaveError(null);

    // Close dialog immediately and notify parent that processing started
    onClose();
    onProcessing?.();

    try {
      const response = await engineApi.parseProjectKnowledge(projectId, text);

      if (!response.success) {
        // Re-open dialog with error
        setSaveError(response.error ?? 'Failed to parse fact');
        return;
      }

      // Success - notify parent to refresh list
      onSave();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to parse fact');
    } finally {
      setIsSaving(false);
    }
  };

  const handleEdit = async () => {
    const effectiveFactType = useCustomType ? customFactType.trim() : factType;

    // Validation
    if (!effectiveFactType) {
      setSaveError('Fact type is required');
      return;
    }

    if (!key.trim()) {
      setSaveError('Key is required');
      return;
    }

    if (!value.trim()) {
      setSaveError('Value is required');
      return;
    }

    setIsSaving(true);
    setSaveError(null);

    try {
      const trimmedContext = context.trim();
      const requestData = trimmedContext
        ? {
            fact_type: effectiveFactType,
            key: key.trim(),
            value: value.trim(),
            context: trimmedContext,
          }
        : {
            fact_type: effectiveFactType,
            key: key.trim(),
            value: value.trim(),
          };

      const response = await engineApi.updateProjectKnowledge(projectId, fact!.id, requestData);

      if (!response.success) {
        setSaveError(response.error ?? 'Failed to update fact');
        return;
      }

      // Success - notify parent and close
      onSave();
      onClose();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save fact');
    } finally {
      setIsSaving(false);
    }
  };

  const handleSave = () => {
    if (isEditing) {
      void handleEdit();
    } else {
      void handleCreate();
    }
  };

  const effectiveFactType = useCustomType ? customFactType.trim() : factType;
  const canSaveEdit = effectiveFactType && key.trim() && value.trim();
  const canSaveCreate = freeFormText.trim();

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {isEditing ? 'Edit Project Knowledge' : 'Add Project Knowledge'}
          </DialogTitle>
          <DialogDescription>
            {isEditing
              ? 'Update the domain fact or business rule.'
              : 'Enter a fact in plain language. It will be automatically categorized.'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {isEditing ? (
            // EDIT MODE: Full form
            <>
              {/* Fact Type */}
              <div>
                <label
                  htmlFor="fact-type"
                  className="block text-sm font-medium text-text-primary mb-1"
                >
                  Fact Type
                </label>
                <div className="space-y-2">
                  <select
                    id="fact-type"
                    value={useCustomType ? '__custom__' : factType}
                    onChange={(e) => {
                      if (e.target.value === '__custom__') {
                        setUseCustomType(true);
                        setFactType('');
                      } else {
                        setUseCustomType(false);
                        setFactType(e.target.value);
                      }
                    }}
                    disabled={isSaving}
                    className="flex h-10 w-full rounded-md border border-border-medium bg-surface-primary px-3 py-2 text-sm text-text-primary ring-offset-surface-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {COMMON_FACT_TYPES.map((type) => (
                      <option key={type.value} value={type.value}>
                        {type.label}
                      </option>
                    ))}
                    <option value="__custom__">Custom...</option>
                  </select>

                  {useCustomType && (
                    <Input
                      value={customFactType}
                      onChange={(e) => setCustomFactType(e.target.value)}
                      placeholder="Enter custom fact type"
                      disabled={isSaving}
                    />
                  )}
                </div>
              </div>

              {/* Key */}
              <div>
                <label
                  htmlFor="key"
                  className="block text-sm font-medium text-text-primary mb-1"
                >
                  Key
                </label>
                <Input
                  id="key"
                  value={key}
                  onChange={(e) => setKey(e.target.value)}
                  placeholder="e.g., timezone_convention, currency_code"
                  disabled={isSaving}
                />
                <p className="mt-1 text-xs text-text-tertiary">
                  A short identifier for this fact
                </p>
              </div>

              {/* Value */}
              <div>
                <label
                  htmlFor="value"
                  className="block text-sm font-medium text-text-primary mb-1"
                >
                  Value
                </label>
                <textarea
                  id="value"
                  value={value}
                  onChange={(e) => setValue(e.target.value)}
                  placeholder="Describe the fact or rule..."
                  rows={3}
                  disabled={isSaving}
                  className="flex w-full rounded-md border border-border-medium bg-surface-primary px-3 py-2 text-sm text-text-primary ring-offset-surface-primary placeholder:text-text-tertiary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 resize-none"
                />
              </div>

              {/* Context (optional) */}
              <div>
                <label
                  htmlFor="context"
                  className="block text-sm font-medium text-text-primary mb-1"
                >
                  Context <span className="text-text-tertiary">(optional)</span>
                </label>
                <textarea
                  id="context"
                  value={context}
                  onChange={(e) => setContext(e.target.value)}
                  placeholder="Additional context about where this fact was learned or how it applies..."
                  rows={2}
                  disabled={isSaving}
                  className="flex w-full rounded-md border border-border-medium bg-surface-primary px-3 py-2 text-sm text-text-primary ring-offset-surface-primary placeholder:text-text-tertiary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 resize-none"
                />
              </div>
            </>
          ) : (
            // CREATE MODE: Simple text input
            <div>
              <label
                htmlFor="free-form-text"
                className="block text-sm font-medium text-text-primary mb-1"
              >
                Fact
              </label>
              <textarea
                id="free-form-text"
                value={freeFormText}
                onChange={(e) => setFreeFormText(e.target.value)}
                placeholder="e.g., All timestamps are stored in UTC"
                rows={3}
                disabled={isSaving}
                autoFocus
                className="flex w-full rounded-md border border-border-medium bg-surface-primary px-3 py-2 text-sm text-text-primary ring-offset-surface-primary placeholder:text-text-tertiary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-purple focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 resize-none"
              />
              <p className="mt-1 text-xs text-text-tertiary">
                Enter a fact in plain language. The system will automatically determine the category and key.
              </p>
            </div>
          )}

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
            disabled={isEditing ? !canSaveEdit || isSaving : !canSaveCreate || isSaving}
          >
            {isSaving ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                {isEditing ? 'Saving...' : 'Processing...'}
              </>
            ) : (
              isEditing ? 'Update Fact' : 'Add Fact'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
