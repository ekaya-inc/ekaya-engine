/**
 * ManagedAIOptionPanel - Reusable panel for Community and Embedded AI options
 * Consolidates duplicate UI for model info, test results, errors, and action buttons
 */
import { Brain, Loader2, Search } from 'lucide-react';

import type { AIOption, AIOptionsResponse, AITestResult } from '../types';

type ManagedOptionId = 'community' | 'embedded';

interface ManagedAIOptionPanelProps {
  optionId: ManagedOptionId;
  description: string | string[];
  unavailableMessage: string;
  helpText: string;
  aiOptions: AIOptionsResponse | null;
  isTesting: boolean;
  isSaving: boolean;
  testResult: AITestResult | null;
  saveError: string | null;
  activeAIConfig: AIOption;
  canSave: boolean;
  onTest: () => void;
  onSave: () => void;
  onRemove: () => void;
}

const ManagedAIOptionPanel = ({
  optionId,
  description,
  unavailableMessage,
  helpText,
  aiOptions,
  isTesting,
  isSaving,
  testResult,
  saveError,
  activeAIConfig,
  canSave,
  onTest,
  onSave,
  onRemove,
}: ManagedAIOptionPanelProps) => {
  const optionConfig = aiOptions?.[optionId];
  const isAvailable = optionConfig?.available ?? false;
  const isActive = activeAIConfig === optionId;

  // Render description paragraphs
  const descriptionParagraphs = Array.isArray(description) ? description : [description];

  return (
    <>
      {/* Description */}
      {descriptionParagraphs.map((text, index) => (
        <p key={index} className="text-text-secondary mb-4">
          {text}
        </p>
      ))}

      {/* Model info display */}
      {isAvailable && optionConfig && (
        <div className="grid md:grid-cols-2 gap-4 mb-4 p-4 bg-surface-primary rounded-lg border border-border-light">
          <div>
            <h4 className="text-sm font-medium text-text-primary flex items-center gap-2 mb-2">
              <Brain className="h-4 w-4" /> Chat Model
            </h4>
            <p className="text-sm text-text-secondary font-mono">{optionConfig.llm_model}</p>
          </div>
          <div>
            <h4 className="text-sm font-medium text-text-primary flex items-center gap-2 mb-2">
              <Search className="h-4 w-4" /> Embedding Model
            </h4>
            <p className="text-sm text-text-secondary font-mono">
              {optionConfig.embedding_model ?? 'Not configured'}
            </p>
          </div>
        </div>
      )}

      {/* Unavailable warning */}
      {!isAvailable && (
        <div className="mb-4 p-3 rounded-lg text-sm bg-amber-500/10 border border-amber-500/20 text-amber-600">
          {unavailableMessage}
        </div>
      )}

      {/* Test Result Display */}
      {testResult && (
        <div
          className={`mb-4 p-3 rounded-lg text-sm ${
            testResult.success
              ? 'bg-green-500/10 border border-green-500/20 text-green-600'
              : 'bg-red-500/10 border border-red-500/20 text-red-600'
          }`}
        >
          <p className="font-medium">{testResult.message}</p>
          {testResult.llm_message && (
            <p className="mt-1 text-xs opacity-80">LLM: {testResult.llm_message}</p>
          )}
          {testResult.embedding_message && (
            <p className="mt-1 text-xs opacity-80">Embedding: {testResult.embedding_message}</p>
          )}
        </div>
      )}

      {/* Save Error Display */}
      {saveError && (
        <div className="mb-4 p-3 rounded-lg text-sm bg-red-500/10 border border-red-500/20 text-red-600">
          {saveError}
        </div>
      )}

      {/* Button bar */}
      <div className="pt-2 border-t border-border-light flex items-center justify-between">
        <p className="text-xs text-text-tertiary">
          {activeAIConfig && activeAIConfig !== optionId
            ? 'Remove current configuration before saving a different one.'
            : helpText}
        </p>
        <div className="flex items-center gap-3">
          <button
            onClick={onTest}
            disabled={isTesting || !isAvailable}
            className="rounded-lg px-4 py-2 text-sm font-medium border border-border-medium bg-surface-primary text-text-primary hover:bg-surface-hover transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            {isTesting && <Loader2 className="h-4 w-4 animate-spin" />}
            Test Connection
          </button>
          <button
            onClick={isActive ? onRemove : onSave}
            disabled={isSaving || !canSave || !isAvailable}
            className={`rounded-lg px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2 ${
              isActive
                ? 'bg-red-600 text-white hover:bg-red-700'
                : 'bg-surface-submit text-white hover:bg-surface-submit-hover'
            }`}
          >
            {isSaving && <Loader2 className="h-4 w-4 animate-spin" />}
            {isActive ? 'Remove Configuration' : 'Save Configuration'}
          </button>
        </div>
      </div>
    </>
  );
};

export default ManagedAIOptionPanel;
