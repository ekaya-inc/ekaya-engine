import {
  Brain,
  Check,
  ChevronDown,
  Loader2,
} from 'lucide-react';
import { useState, useEffect, useCallback } from 'react';

import engineApi from '../services/engineApi';
import type {
  AIConfigForm,
  AIConfigSaveRequest,
  AIOption,
  AIOptionsResponse,
  AITestResult,
} from '../types';

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog';

interface AIConfigWidgetProps {
  projectId: string;
  disabled?: boolean;
  onConfigChange?: (configType: AIOption) => void;
}

const llmProviderPresets = [
  { label: 'OpenAI', url: 'https://api.openai.com/v1' },
  { label: 'Anthropic', url: 'https://api.anthropic.com/v1' },
  { label: 'Azure OpenAI', url: '' },
  { label: 'Custom', url: '' },
];

const aiOptionsList = [
  {
    id: 'byok' as const,
    title: 'Bring Your Own AI Keys',
    badge: 'Free',
    badgeColor: 'bg-gray-500 text-white',
  },
  {
    id: 'community' as const,
    title: 'Community Models',
    badge: 'Easy',
    badgeColor: 'bg-gray-500 text-white',
  },
  {
    id: 'embedded' as const,
    title: 'Embedded AI',
    badge: 'Secure',
    badgeColor: 'bg-gray-500 text-white',
  },
];

const AIConfigWidget = ({ projectId, disabled = false, onConfigChange }: AIConfigWidgetProps) => {
  const [selectedAIOption, setSelectedAIOption] = useState<AIOption>(null);
  const [activeAIConfig, setActiveAIConfig] = useState<AIOption>(null);
  const [aiConfig, setAiConfig] = useState<AIConfigForm>({
    llmBaseUrl: '',
    llmApiKey: '',
    llmModel: '',
    embeddingBaseUrl: '',
    embeddingApiKey: '',
    embeddingModel: '',
  });
  const [isLoadingConfig, setIsLoadingConfig] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [testResult, setTestResult] = useState<AITestResult | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [showRemoveConfirmation, setShowRemoveConfirmation] = useState(false);
  const [aiOptions, setAiOptions] = useState<AIOptionsResponse | null>(null);
  const [selectedProvider, setSelectedProvider] = useState<string>('OpenAI');
  const [isProviderDropdownOpen, setIsProviderDropdownOpen] = useState(false);

  const loadAIConfig = useCallback(async () => {
    if (!projectId) return;

    setIsLoadingConfig(true);
    try {
      const [configResponse, optionsResponse] = await Promise.all([
        engineApi.getAIConfig(projectId),
        engineApi.getProjectConfig(),
      ]);

      const config = configResponse.data;
      if (config?.config_type && config.config_type !== 'none') {
        const configType = config.config_type as AIOption;
        setActiveAIConfig(configType);
        onConfigChange?.(configType);
        setAiConfig({
          llmBaseUrl: config.llm_base_url ?? '',
          llmApiKey: config.llm_api_key ?? '',
          llmModel: config.llm_model ?? '',
          embeddingBaseUrl: config.embedding_base_url ?? '',
          embeddingApiKey: config.embedding_api_key ?? '',
          embeddingModel: config.embedding_model ?? '',
        });
        const loadedUrl = config.llm_base_url ?? '';
        if (loadedUrl.includes('api.openai.com')) {
          setSelectedProvider('OpenAI');
        } else if (loadedUrl.includes('api.anthropic.com')) {
          setSelectedProvider('Anthropic');
        } else if (loadedUrl.includes('openai.azure.com')) {
          setSelectedProvider('Azure OpenAI');
        } else if (loadedUrl) {
          setSelectedProvider('Custom');
        }
      } else {
        onConfigChange?.(null);
      }

      if (optionsResponse) {
        setAiOptions(optionsResponse.ai_options);
      }
    } catch (err) {
      console.error('Failed to load AI config:', err);
      onConfigChange?.(null);
    } finally {
      setIsLoadingConfig(false);
    }
  }, [projectId, onConfigChange]);

  useEffect(() => {
    loadAIConfig();
  }, [loadAIConfig]);

  const handleTestConnection = async (configType?: AIOption) => {
    setIsTesting(true);
    setTestResult(null);
    setSaveError(null);

    try {
      let body: Record<string, string | boolean>;

      if (configType === 'community' || configType === 'embedded') {
        body = { config_type: configType };
      } else {
        const isMaskedKey = aiConfig.llmApiKey.includes('...');
        if (isMaskedKey) {
          body = {};
        } else {
          body = {
            llm_base_url: aiConfig.llmBaseUrl,
            llm_api_key: aiConfig.llmApiKey,
            llm_model: aiConfig.llmModel,
            embedding_base_url: aiConfig.embeddingBaseUrl,
            embedding_api_key: aiConfig.embeddingApiKey,
            embedding_model: aiConfig.embeddingModel,
          };
        }
      }

      const result = await engineApi.testAIConnection(projectId, body);
      setTestResult(result.data ?? { success: false, message: 'No response data' });
    } catch (err) {
      setTestResult({
        success: false,
        message: err instanceof Error ? err.message : 'Connection test failed',
      });
    } finally {
      setIsTesting(false);
    }
  };

  const handleSaveConfig = async (configType: AIOption = 'byok') => {
    setIsSaving(true);
    setSaveError(null);
    setTestResult(null);
    setIsProviderDropdownOpen(false);

    try {
      let body: AIConfigSaveRequest;

      if (configType === 'community' || configType === 'embedded') {
        const optionConfig = configType === 'community' ? aiOptions?.community : aiOptions?.embedded;
        body = {
          config_type: configType,
          llm_base_url: optionConfig?.llm_base_url ?? '',
          llm_model: optionConfig?.llm_model ?? '',
          embedding_base_url: optionConfig?.embedding_url ?? '',
          embedding_model: optionConfig?.embedding_model ?? '',
        };
      } else {
        const isMaskedLLMKey = aiConfig.llmApiKey.includes('...');
        const isMaskedEmbeddingKey = aiConfig.embeddingApiKey.includes('...');

        if (isMaskedLLMKey && activeAIConfig !== 'byok') {
          setSaveError('Please enter your API key to save configuration');
          setIsSaving(false);
          return;
        }

        body = {
          config_type: 'byok',
          llm_base_url: aiConfig.llmBaseUrl,
          llm_model: aiConfig.llmModel,
          embedding_base_url: aiConfig.embeddingBaseUrl,
          embedding_model: aiConfig.embeddingModel,
        };

        if (!isMaskedLLMKey) {
          body.llm_api_key = aiConfig.llmApiKey;
        }
        if (!isMaskedEmbeddingKey && aiConfig.embeddingApiKey) {
          body.embedding_api_key = aiConfig.embeddingApiKey;
        }
      }

      await engineApi.saveAIConfig(projectId, body);
      setActiveAIConfig(configType);
      setSelectedAIOption(null);
      onConfigChange?.(configType);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save configuration');
    } finally {
      setIsSaving(false);
    }
  };

  const handleRemoveConfig = async () => {
    setShowRemoveConfirmation(false);
    setIsSaving(true);
    setSaveError(null);

    try {
      await engineApi.deleteAIConfig(projectId);
      setActiveAIConfig(null);
      setAiConfig({
        llmBaseUrl: '',
        llmApiKey: '',
        llmModel: '',
        embeddingBaseUrl: '',
        embeddingApiKey: '',
        embeddingModel: '',
      });
      setTestResult(null);
      onConfigChange?.(null);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to remove configuration');
    } finally {
      setIsSaving(false);
    }
  };

  const updateAiConfig = (field: keyof AIConfigForm, value: string) => {
    setAiConfig((prev) => ({ ...prev, [field]: value }));
    setTestResult(null);
    setSaveError(null);
  };

  const getInputErrorClass = (
    section: 'llm' | 'embedding',
    field: 'endpoint' | 'auth' | 'model'
  ): string => {
    if (!testResult) return '';

    const errorType = section === 'llm'
      ? testResult.llm_error_type
      : testResult.embedding_error_type;

    const isError = section === 'llm'
      ? !testResult.llm_success
      : !testResult.embedding_success;

    if (!isError || !errorType) return '';

    if (errorType === field) {
      return 'border-red-500 focus:border-red-500 focus:ring-red-500';
    }
    return '';
  };

  const canSaveConfig = (optionId: AIOption): boolean => {
    if (!activeAIConfig) return true;
    return activeAIConfig === optionId;
  };

  return (
    <>
      {/* AI Selection Bar */}
      <div className={`mb-6 ${disabled ? 'opacity-50' : ''}`}>
        <div className={`flex border border-border-light overflow-hidden ${selectedAIOption && !disabled ? 'rounded-t-lg' : 'rounded-lg'}`}>
          {aiOptionsList.map((option, index) => {
            const isSelected = selectedAIOption === option.id;
            return (
              <button
                key={option.id}
                disabled={disabled}
                className={`flex-1 px-4 py-3 flex items-center justify-center gap-2 transition-all ${
                  isSelected && !disabled
                    ? 'bg-surface-secondary text-text-primary'
                    : 'bg-surface-primary hover:bg-surface-secondary/50 text-text-primary'
                } ${index > 0 ? 'border-l border-border-light' : ''} ${disabled ? 'cursor-not-allowed' : ''}`}
                onClick={() => !disabled && setSelectedAIOption(isSelected ? null : option.id)}
              >
                {activeAIConfig === option.id && (
                  <Check className="h-4 w-4 text-green-600" />
                )}
                <span className="font-medium">{option.title}</span>
                <span className={`rounded px-2 py-0.5 text-xs font-medium ${option.badgeColor}`}>
                  {option.badge}
                </span>
              </button>
            );
          })}
        </div>

        {/* BYOK Panel */}
        {selectedAIOption === 'byok' && (
          <div className="rounded-b-lg bg-surface-secondary p-6 border border-t-0 border-border-light">
            {isLoadingConfig ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
                <span className="ml-2 text-text-secondary">Loading configuration...</span>
              </div>
            ) : (
              <>
            <p className="text-text-secondary text-sm mb-6">
              Configure an OpenAI-compatible endpoint for chat completions. Most providers (OpenAI, Anthropic, Azure, Ollama, vLLM, Together AI) support this interface.
            </p>

            <div>
              <div className="space-y-4 max-w-md">
                <h3 className="text-base font-semibold text-text-primary flex items-center gap-2">
                  <Brain className="h-4 w-4" />
                  Chat Model (LLM)
                </h3>

                <div>
                  <label className="block text-sm font-medium text-text-primary mb-1">
                    Provider
                  </label>
                  <div className="relative">
                    <button
                      type="button"
                      onClick={() => !activeAIConfig && setIsProviderDropdownOpen(!isProviderDropdownOpen)}
                      disabled={activeAIConfig === 'byok'}
                      className={`w-full rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary text-left flex items-center justify-between focus:ring-1 focus:outline-none ${activeAIConfig === 'byok' ? 'opacity-60 cursor-not-allowed' : ''} ${getInputErrorClass('llm', 'endpoint') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
                    >
                      <span>{selectedProvider}</span>
                      <ChevronDown className={`h-4 w-4 text-text-tertiary transition-transform ${isProviderDropdownOpen ? 'rotate-180' : ''}`} />
                    </button>
                    {isProviderDropdownOpen && (
                      <div className="absolute z-10 w-full mt-1 rounded-lg border border-border-light bg-surface-primary shadow-lg overflow-hidden">
                        {llmProviderPresets.map((preset) => (
                          <button
                            key={preset.label}
                            type="button"
                            onClick={() => {
                              setSelectedProvider(preset.label);
                              setIsProviderDropdownOpen(false);
                              updateAiConfig('llmBaseUrl', preset.url);
                              if (preset.label === 'Anthropic' && !aiConfig.llmModel) {
                                updateAiConfig('llmModel', 'claude-haiku-4-5');
                              }
                            }}
                            className={`w-full px-3 py-2 text-sm text-left hover:bg-surface-hover flex items-center justify-between ${
                              selectedProvider === preset.label ? 'bg-surface-secondary text-text-primary' : 'text-text-primary'
                            }`}
                          >
                            <span>{preset.label}</span>
                            {selectedProvider === preset.label && <Check className="h-4 w-4 text-brand-purple" />}
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                  {(selectedProvider === 'Custom' || selectedProvider === 'Azure OpenAI') && (
                    <input
                      type="text"
                      placeholder={selectedProvider === 'Azure OpenAI' ? 'https://your-resource.openai.azure.com' : 'https://your-endpoint.com/v1'}
                      value={aiConfig.llmBaseUrl}
                      onChange={(e) => updateAiConfig('llmBaseUrl', e.target.value)}
                      disabled={activeAIConfig === 'byok'}
                      className={`w-full mt-2 rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-secondary/50 focus:ring-1 focus:outline-none ${activeAIConfig === 'byok' ? 'opacity-60 cursor-not-allowed' : ''} ${getInputErrorClass('llm', 'endpoint') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
                    />
                  )}
                  {selectedProvider === 'Azure OpenAI' && (
                    <p className="mt-1 text-xs text-text-tertiary">
                      Enter your Azure OpenAI resource endpoint
                    </p>
                  )}
                </div>

                <div>
                  <label className="block text-sm font-medium text-text-primary mb-1">
                    API Key
                  </label>
                  <input
                    type="password"
                    placeholder="sk-..."
                    value={aiConfig.llmApiKey}
                    onChange={(e) => updateAiConfig('llmApiKey', e.target.value)}
                    disabled={activeAIConfig === 'byok'}
                    className={`w-full rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-secondary/50 focus:ring-1 focus:outline-none ${activeAIConfig === 'byok' ? 'opacity-60 cursor-not-allowed' : ''} ${getInputErrorClass('llm', 'auth') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
                  />
                  <p className="mt-1 text-xs text-text-tertiary">
                    Not required for local endpoints (Ollama, vLLM)
                  </p>
                </div>

                <div>
                  <label className="block text-sm font-medium text-text-primary mb-1">
                    Model
                  </label>
                  <input
                    type="text"
                    placeholder="gpt-4o, claude-haiku-4-5, llama3.1"
                    value={aiConfig.llmModel}
                    onChange={(e) => updateAiConfig('llmModel', e.target.value)}
                    disabled={activeAIConfig === 'byok'}
                    className={`w-full rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-secondary/50 focus:ring-1 focus:outline-none ${activeAIConfig === 'byok' ? 'opacity-60 cursor-not-allowed' : ''} ${getInputErrorClass('llm', 'model') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
                  />
                </div>
              </div>

              {/* TEMPORARY: Embedding Configuration hidden for MVP launch */}
            </div>

            {/* Test Result Display */}
            {testResult && (
              <div className={`mt-4 p-3 rounded-lg text-sm ${
                testResult.success
                  ? 'bg-green-500/10 border border-green-500/20 text-green-600'
                  : 'bg-red-500/10 border border-red-500/20 text-red-600'
              }`}>
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
              <div className="mt-4 p-3 rounded-lg text-sm bg-red-500/10 border border-red-500/20 text-red-600">
                {saveError}
              </div>
            )}

            <div className="mt-6 pt-4 border-t border-border-light flex items-center justify-between">
              <p className="text-xs text-text-tertiary">
                {activeAIConfig && activeAIConfig !== 'byok'
                  ? 'Remove current configuration before saving a different one.'
                  : 'Credentials are encrypted and stored per-project.'}
              </p>
              <div className="flex items-center gap-3">
                <button
                  onClick={() => handleTestConnection()}
                  disabled={isTesting || (!aiConfig.llmBaseUrl && activeAIConfig !== 'byok')}
                  className="rounded-lg px-4 py-2 text-sm font-medium border border-border-medium bg-surface-primary text-text-primary hover:bg-surface-hover transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                >
                  {isTesting && <Loader2 className="h-4 w-4 animate-spin" />}
                  Test Connection
                </button>
                <button
                  onClick={activeAIConfig === 'byok' ? () => setShowRemoveConfirmation(true) : () => handleSaveConfig('byok')}
                  disabled={isSaving || !canSaveConfig('byok') || (!testResult?.success && activeAIConfig !== 'byok')}
                  className={`rounded-lg px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2 ${
                    activeAIConfig === 'byok'
                      ? 'bg-red-600 text-white hover:bg-red-700'
                      : 'bg-surface-submit text-white hover:bg-surface-submit-hover'
                  }`}
                >
                  {isSaving && <Loader2 className="h-4 w-4 animate-spin" />}
                  {activeAIConfig === 'byok' ? 'Remove Configuration' : 'Save Configuration'}
                </button>
              </div>
            </div>
            </>
            )}
          </div>
        )}

        {/* TEMPORARY: Community - Coming Soon for MVP launch */}
        {selectedAIOption === 'community' && (
          <div className="rounded-b-lg bg-surface-secondary p-6 border border-t-0 border-border-light">
            <p className="text-text-secondary mb-4">
              The Ekaya Community Models are open weight and free to use. We fine-tune the Community Models using anonymized schema, natural language and SQL queries from participants who opt-in to sharing their data. The end result is a more robust and accurate model than the frontier models in this domain.
            </p>
            <div className="flex items-center justify-center py-8">
              <a
                href="mailto:support@ekaya.ai?subject=Add Community Models to my installation"
                className="rounded-lg px-6 py-3 text-sm font-medium bg-surface-submit text-white hover:bg-surface-submit-hover transition-colors"
              >
                Contact Support
              </a>
            </div>
          </div>
        )}

        {/* TEMPORARY: Embedded - Coming Soon for MVP launch */}
        {selectedAIOption === 'embedded' && (
          <div className="rounded-b-lg bg-surface-secondary p-6 border border-t-0 border-border-light">
            <p className="text-text-secondary mb-4">
              Ekaya offers custom models that you can host on your own infrastructure or as part of your hybrid cloud so that data never leaves your security boundary. Your data is not used for training these models although Ekaya can build models customized for your internal needs.
            </p>
            <p className="text-text-secondary mb-4">
              These models have been fine-tuned for this domain and include prevention of SQL and LLM prompt injection attacks as well as detecting data leakage. These models are required for some features.
            </p>
            <div className="flex items-center justify-center py-8">
              <a
                href="mailto:sales@ekaya.ai?subject=Add Security Models to my installation"
                className="rounded-lg px-6 py-3 text-sm font-medium bg-surface-submit text-white hover:bg-surface-submit-hover transition-colors"
              >
                Contact Sales
              </a>
            </div>
          </div>
        )}
      </div>

      {/* Remove AI Configuration Confirmation Dialog */}
      <Dialog open={showRemoveConfirmation} onOpenChange={setShowRemoveConfirmation}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove AI Configuration?</DialogTitle>
            <DialogDescription className="space-y-3 pt-2">
              <p>
                Intelligent features require Large Language Models to function. Removing this configuration will disable:
              </p>
              <ul className="list-disc pl-5 space-y-1 text-text-secondary">
                <li>Natural language query generation</li>
                <li>Ontology extraction and business rules</li>
                <li>Semantic search and memory features</li>
                <li>AI-powered data analysis</li>
              </ul>
              <p className="text-amber-600 font-medium">
                Portions of your ontology and memory will no longer be available until you reconfigure an AI provider.
              </p>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <button
              onClick={() => setShowRemoveConfirmation(false)}
              className="rounded-lg px-4 py-2 text-sm font-medium border border-border-medium bg-surface-primary text-text-primary hover:bg-surface-hover transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleRemoveConfig}
              className="rounded-lg px-4 py-2 text-sm font-medium bg-red-600 text-white hover:bg-red-700 transition-colors"
            >
              Remove Configuration
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
};

export default AIConfigWidget;
