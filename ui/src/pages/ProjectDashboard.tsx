import type { LucideIcon } from 'lucide-react';
import {
  Boxes,
  Brain,
  Check,
  ChevronDown,
  Database,
  Layers,
  ListTree,
  Loader2,
  MessageCircleQuestion,
  Network,
  Search,
  Server,
  Shield,
} from 'lucide-react';
import { useState, useEffect, useCallback } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

// TEMPORARY: Import commented out for MVP launch - Coming Soon UI replaces full panels
// import ManagedAIOptionPanel from '../components/ManagedAIOptionPanel';
import { Card, CardHeader, CardTitle } from '../components/ui/Card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../components/ui/Dialog';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';
import { fetchWithAuth } from '../lib/api';
import { ontologyService } from '../services/ontologyService';
import type {
  AIConfigForm,
  AIOption,
  AIOptionsResponse,
  AITestResult,
  OntologyWorkflowStatus,
} from '../types';

type TileColor = 'blue' | 'green' | 'purple' | 'orange' | 'gray' | 'indigo' | 'cyan';

interface Tile {
  title: string;
  icon: LucideIcon;
  path: string;
  disabled: boolean;
  color: TileColor;
}

/**
 * ProjectDashboard - Navigation tiles for project workspace
 * Displays navigation options for database, schema, ontology, etc.
 * This is the index page under /projects/:pid
 *
 * Note: Schema and Queries tiles are disabled when no datasource is configured.
 * Ontology tile is disabled when no datasource is configured OR no tables are selected.
 * Users must first configure a datasource via the Datasource tile and select tables in the Schema page.
 */
const ProjectDashboard = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { isConnected, hasSelectedTables } = useDatasourceConnection();
  const [selectedAIOption, setSelectedAIOption] = useState<AIOption>(null);
  const [activeAIConfig, setActiveAIConfig] = useState<AIOption>(null);

  // Ontology workflow status for badge
  const [ontologyStatus, setOntologyStatus] = useState<OntologyWorkflowStatus | null>(null);

  // AI Config form state
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

  // Preset LLM provider URLs for combo box
  const llmProviderPresets = [
    { label: 'OpenAI', url: 'https://api.openai.com/v1' },
    { label: 'Anthropic', url: 'https://api.anthropic.com/v1' },
    { label: 'Azure OpenAI', url: '' }, // User needs to fill in their resource name
    { label: 'Custom', url: '' },
  ];
  const [selectedProvider, setSelectedProvider] = useState<string>('OpenAI');
  const [isProviderDropdownOpen, setIsProviderDropdownOpen] = useState(false);

  // Load AI config and options on mount
  const loadAIConfig = useCallback(async () => {
    if (!pid) return;

    setIsLoadingConfig(true);
    try {
      // Load project-specific config and server AI options in parallel
      const [configResponse, optionsResponse] = await Promise.all([
        fetchWithAuth(`/api/projects/${pid}/ai-config`),
        fetchWithAuth(`/api/config/project`),
      ]);

      if (configResponse.ok) {
        const data = await configResponse.json();
        if (data.config_type && data.config_type !== 'none') {
          setActiveAIConfig(data.config_type as AIOption);
          // Don't auto-expand panel - let user click to expand
          setAiConfig({
            llmBaseUrl: data.llm_base_url ?? '',
            llmApiKey: data.llm_api_key ?? '', // Will be masked: "sk-a...xyz"
            llmModel: data.llm_model ?? '',
            embeddingBaseUrl: data.embedding_base_url ?? '',
            embeddingApiKey: data.embedding_api_key ?? '',
            embeddingModel: data.embedding_model ?? '',
          });
          // Set provider dropdown based on loaded URL
          const loadedUrl = data.llm_base_url ?? '';
          if (loadedUrl.includes('api.openai.com')) {
            setSelectedProvider('OpenAI');
          } else if (loadedUrl.includes('api.anthropic.com')) {
            setSelectedProvider('Anthropic');
          } else if (loadedUrl.includes('openai.azure.com')) {
            setSelectedProvider('Azure OpenAI');
          } else if (loadedUrl) {
            setSelectedProvider('Custom');
          }
        }
      }

      if (optionsResponse.ok) {
        const projectConfig = await optionsResponse.json();
        setAiOptions(projectConfig.ai_options);
      }
    } catch (err) {
      console.error('Failed to load AI config:', err);
    } finally {
      setIsLoadingConfig(false);
    }
  }, [pid]);

  useEffect(() => {
    loadAIConfig();
  }, [loadAIConfig]);

  // Subscribe to ontology status for badge display
  useEffect(() => {
    const unsubscribe = ontologyService.subscribe((status) => {
      setOntologyStatus(status);
    });
    return () => {
      unsubscribe();
    };
  }, []);

  // Test AI connection (with current form values, without saving)
  // configType parameter allows testing community/embedded configs from server
  const handleTestConnection = async (configType?: AIOption) => {
    setIsTesting(true);
    setTestResult(null);
    setSaveError(null);

    try {
      let body: Record<string, string | boolean>;

      // For community/embedded, use server-configured endpoints
      if (configType === 'community' || configType === 'embedded') {
        body = { config_type: configType };
      } else {
        // BYOK flow - check if API key looks like a masked value (saved credentials)
        const isMaskedKey = aiConfig.llmApiKey.includes('...');

        if (isMaskedKey) {
          // Use saved credentials - send empty body
          body = {};
        } else {
          // Use form values for testing
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

      const response = await fetchWithAuth(`/api/projects/${pid}/ai-config/test`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });

      const data = await response.json();
      setTestResult(data);
    } catch (err) {
      setTestResult({
        success: false,
        message: err instanceof Error ? err.message : 'Connection test failed',
      });
    } finally {
      setIsTesting(false);
    }
  };

  // Save AI configuration
  // configType parameter specifies which type to save (byok, community, embedded)
  const handleSaveConfig = async (configType: AIOption = 'byok') => {
    setIsSaving(true);
    setSaveError(null);
    setTestResult(null);

    try {
      let body: Record<string, string>;

      if (configType === 'community' || configType === 'embedded') {
        // For community/embedded, we just save the config type
        // The actual endpoints come from server config
        const optionConfig = configType === 'community' ? aiOptions?.community : aiOptions?.embedded;
        body = {
          config_type: configType,
          llm_base_url: optionConfig?.llm_base_url ?? '',
          llm_model: optionConfig?.llm_model ?? '',
          embedding_base_url: optionConfig?.embedding_url ?? '',
          embedding_model: optionConfig?.embedding_model ?? '',
        };
      } else {
        // BYOK - check if API key looks like a masked value
        const isMaskedLLMKey = aiConfig.llmApiKey.includes('...');
        const isMaskedEmbeddingKey = aiConfig.embeddingApiKey.includes('...');

        // Don't allow saving with masked keys (user needs to re-enter)
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

        // Only send API keys if they're not masked
        if (!isMaskedLLMKey) {
          body.llm_api_key = aiConfig.llmApiKey;
        }
        if (!isMaskedEmbeddingKey && aiConfig.embeddingApiKey) {
          body.embedding_api_key = aiConfig.embeddingApiKey;
        }
      }

      const response = await fetchWithAuth(`/api/projects/${pid}/ai-config`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.message ?? 'Failed to save configuration');
      }

      setActiveAIConfig(configType);
      // Reload to get latest config
      await loadAIConfig();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save configuration');
    } finally {
      setIsSaving(false);
    }
  };

  // Remove AI configuration
  const handleRemoveConfig = async () => {
    setShowRemoveConfirmation(false);
    setIsSaving(true);
    setSaveError(null);

    try {
      const response = await fetchWithAuth(`/api/projects/${pid}/ai-config`, {
        method: 'DELETE',
      });

      if (!response.ok && response.status !== 404) {
        throw new Error('Failed to remove configuration');
      }

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
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to remove configuration');
    } finally {
      setIsSaving(false);
    }
  };

  // Update form field
  const updateAiConfig = (field: keyof AIConfigForm, value: string) => {
    setAiConfig((prev) => ({ ...prev, [field]: value }));
    setTestResult(null); // Clear test result when config changes
    setSaveError(null);
  };

  // Get input border class based on error type
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

  // Full list of AI options - preserved for when Community/Embedded models are ready
  const aiOptionsList = [
    {
      id: 'byok' as const,
      title: 'Bring Your Own AI Keys',
      badge: 'Free',
      badgeColor: 'bg-gray-500 text-white',
    },
    {
      id: 'community' as const,
      title: 'Community Model',
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

  // TEMPORARY: For launch, Community/Embedded show "Coming Soon" instead of full config
  // TODO: Remove this comment and enable full functionality when models are ready

  // Check if save is allowed (disabled if another config is already active)
  const canSaveConfig = (optionId: AIOption): boolean => {
    if (!activeAIConfig) return true; // No config active, can save any
    return activeAIConfig === optionId; // Can only modify current active config
  };

  const dataTiles: Tile[] = [
    {
      title: 'Datasource',
      icon: Database,
      path: `/projects/${pid}/datasource`,
      disabled: false, // Always enabled - needed to configure datasource
      color: 'blue',
    },
    {
      title: 'Schema',
      icon: ListTree,
      path: `/projects/${pid}/schema`,
      disabled: !isConnected, // Disabled if no datasource configured
      color: 'green',
    },
    {
      title: 'Queries',
      icon: Search,
      path: `/projects/${pid}/queries`,
      disabled: !isConnected, // Disabled if no datasource configured
      color: 'orange',
    },
  ];

  const intelligenceTiles: Tile[] = [
    {
      title: 'Entities',
      icon: Boxes,
      path: `/projects/${pid}/entities`,
      disabled: !isConnected || !hasSelectedTables, // Disabled if no datasource or no tables (entities are database-derived, not AI-derived)
      color: 'green',
    },
    {
      title: 'Relationships',
      icon: Network,
      path: `/projects/${pid}/relationships`,
      disabled: !isConnected || !hasSelectedTables || !activeAIConfig, // Disabled if no datasource, no tables, or no AI config
      color: 'indigo',
    },
    {
      title: 'Ontology',
      icon: Layers,
      path: `/projects/${pid}/ontology`,
      disabled: !isConnected || !hasSelectedTables || !activeAIConfig, // Disabled if no datasource, no tables, or no AI config
      color: 'purple',
    },
    {
      title: 'Security',
      icon: Shield,
      path: `/projects/${pid}/security`,
      disabled: !isConnected || !hasSelectedTables || !activeAIConfig, // Disabled if no datasource, no tables, or no AI config
      color: 'gray',
    },
  ];

  const applicationTiles: Tile[] = [
    {
      title: 'MCP Server',
      icon: Server,
      path: `/projects/${pid}/mcp-server`,
      disabled: !isConnected || !hasSelectedTables, // Requires datasource and schema
      color: 'cyan',
    },
  ];

    const handleTileClick = (tile: Tile): void => {
      if (!tile.disabled) {
      navigate(tile.path);
    }
  };

  const getColorClasses = (color: TileColor): string => {
    const colorMap: Record<TileColor, string> = {
      blue: 'bg-blue-500/10 text-blue-500 hover:bg-blue-500/20',
      green: 'bg-green-500/10 text-green-500 hover:bg-green-500/20',
      purple: 'bg-purple-500/10 text-purple-500 hover:bg-purple-500/20',
      orange: 'bg-orange-500/10 text-orange-500 hover:bg-orange-500/20',
      gray: 'bg-gray-500/10 text-gray-500 hover:bg-gray-500/20',
      indigo: 'bg-indigo-500/10 text-indigo-500 hover:bg-indigo-500/20',
      cyan: 'bg-cyan-500/10 text-cyan-500 hover:bg-cyan-500/20',
    };

    return colorMap[color];
  };

  const renderTile = (tile: Tile) => {
    const Icon = tile.icon;
    const colorClasses = getColorClasses(tile.color);

    // Check for ontology badge
    const isOntologyTile = tile.title === 'Ontology';
    const pendingQuestions = ontologyStatus?.pendingQuestionCount ?? 0;
    const isBuilding = ontologyStatus?.progress.state === 'building' || ontologyStatus?.progress.state === 'initializing';
    const progressCurrent = ontologyStatus?.progress.current ?? 0;
    const progressTotal = ontologyStatus?.progress.total ?? 0;

    return (
      <Card
        key={tile.title}
        className={`transition-all ${
          tile.disabled
            ? 'opacity-50 cursor-not-allowed'
            : 'cursor-pointer hover:shadow-lg'
        }`}
        onClick={() => handleTileClick(tile)}
      >
        <CardHeader className="pb-4">
          <div className="relative">
            <div
              className={`mb-4 flex h-16 w-16 items-center justify-center rounded-lg ${colorClasses}`}
            >
              <Icon className="h-8 w-8" />
            </div>
            {/* Badge for pending questions */}
            {isOntologyTile && pendingQuestions > 0 && !tile.disabled && (
              <div className="absolute -top-1 -right-1 flex items-center gap-1 bg-amber-500 text-white text-xs font-medium px-2 py-0.5 rounded-full">
                <MessageCircleQuestion className="h-3 w-3" />
                {pendingQuestions}
              </div>
            )}
          </div>
          <div className="flex items-center gap-2">
            <CardTitle className="text-xl">{tile.title}</CardTitle>
          </div>
          {/* Mini progress indicator for ontology */}
          {isOntologyTile && isBuilding && !tile.disabled && (
            <div className="mt-2">
              <div className="flex items-center gap-2 text-xs text-blue-600">
                <Loader2 className="h-3 w-3 animate-spin" />
                <span>Building... {progressCurrent}/{progressTotal}</span>
              </div>
              <div className="mt-1 h-1 w-full rounded-full bg-surface-tertiary overflow-hidden">
                <div
                  className="h-full bg-blue-500 rounded-full transition-all"
                  style={{ width: `${progressTotal > 0 ? (progressCurrent / progressTotal) * 100 : 0}%` }}
                />
              </div>
            </div>
          )}
        </CardHeader>
      </Card>
    );
  };

  const renderApplicationTile = (tile: Tile) => {
    const Icon = tile.icon;
    const colorClasses = getColorClasses(tile.color);

    return (
      <Card
        key={tile.title}
        className={`transition-all ${
          tile.disabled
            ? 'opacity-50 cursor-not-allowed'
            : 'cursor-pointer hover:shadow-lg'
        }`}
        onClick={() => handleTileClick(tile)}
      >
        <CardHeader className="pb-6">
          <div
            className={`mb-6 flex h-24 w-24 items-center justify-center rounded-xl ${colorClasses}`}
          >
            <Icon className="h-12 w-12" />
          </div>
          <CardTitle className="text-2xl">{tile.title}</CardTitle>
          {tile.disabled && (
            <p className="text-sm text-text-tertiary mt-2">
              Requires a datasource and schema to be configured.
            </p>
          )}
        </CardHeader>
      </Card>
    );
  };

  return (
    <div className="mx-auto max-w-6xl space-y-8">
      {/* Applications Section */}
      <section>
        <h1 className="text-2xl font-semibold mb-2">Applications</h1>
        <p className="text-text-secondary mb-4">
          Deploy applications that connect to your data through secure, governed interfaces.
        </p>
        <div className="grid gap-6 md:grid-cols-2">
          {applicationTiles.map(renderApplicationTile)}
        </div>
      </section>

      {/* Data Section */}
      <section>
        <h1 className="text-2xl font-semibold mb-2">Data</h1>
        <p className="text-text-secondary mb-4">
          Enable data access safely and securely. Choose the tables and fields that are exposed. Add pre-approved queries that your users can run.
        </p>
        <div className="grid gap-6 md:grid-cols-3">
          {dataTiles.map(renderTile)}
        </div>
      </section>

      {/* Intelligence Section */}
      <section>
        {!hasSelectedTables && (
          <p className="text-sm text-red-500 mb-2">
            Configure a datasource and select tables in the Schema page to enable Intelligence features.
          </p>
        )}
        <h1 className="text-2xl font-semibold mb-2">Intelligence</h1>
        {/* TEMPORARY: Simplified description for BYOK-only launch
            Original text: Add intelligence to the data by connecting a Large Language Model. You can bring your own AI keys or use Ekaya's models that are customized for data querying and analytics. Ekaya offers a free community model as well as licensed embeddable models that you can host so that no data leaves your data boundary.
            TODO: Restore original text when Community/Embedded models are ready */}
        <p className={`mb-4 ${!hasSelectedTables || !activeAIConfig ? 'text-text-secondary/50' : 'text-text-secondary'}`}>
          Add intelligence to the data by connecting a Large Language Model. Configure your own OpenAI-compatible API keys to enable AI-powered features like ontology extraction, semantic search, and natural language querying.
        </p>
        
        {/* AI Selection Bar */}
        <div className={`mb-6 ${!hasSelectedTables ? 'opacity-50' : ''}`}>
          <div className={`flex border border-border-light overflow-hidden ${selectedAIOption && hasSelectedTables ? 'rounded-t-lg' : 'rounded-lg'}`}>
            {aiOptionsList.map((option, index) => {
              const isSelected = selectedAIOption === option.id;
              return (
                <button
                  key={option.id}
                  disabled={!hasSelectedTables}
                  className={`flex-1 px-4 py-3 flex items-center justify-center gap-2 transition-all ${
                    isSelected && hasSelectedTables
                      ? 'bg-surface-secondary text-text-primary'
                      : 'bg-surface-primary hover:bg-surface-secondary/50 text-text-primary'
                  } ${index > 0 ? 'border-l border-border-light' : ''} ${!hasSelectedTables ? 'cursor-not-allowed' : ''}`}
                  onClick={() => hasSelectedTables && setSelectedAIOption(isSelected ? null : option.id)}
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
          
          {/* Expandable settings panel - visually connected to the bar */}
          {selectedAIOption === 'byok' && (
            <div className="rounded-b-lg bg-surface-secondary p-6 border border-t-0 border-border-light">
              {isLoadingConfig ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
                  <span className="ml-2 text-text-secondary">Loading configuration...</span>
                </div>
              ) : (
                <>
              {/* TEMPORARY: Simplified description for LLM-only MVP
                  Original: Configure OpenAI-compatible endpoints for chat completions and embeddings. Most providers (OpenAI, Anthropic, Azure, Ollama, vLLM, Together AI) support this interface. */}
              <p className="text-text-secondary text-sm mb-6">
                Configure an OpenAI-compatible endpoint for chat completions. Most providers (OpenAI, Anthropic, Azure, Ollama, vLLM, Together AI) support this interface.
              </p>

              {/* TEMPORARY: Single column layout for LLM-only MVP. Original was grid md:grid-cols-2 */}
              <div>
                {/* LLM Configuration */}
                <div className="space-y-4 max-w-md">
                  <h3 className="text-base font-semibold text-text-primary flex items-center gap-2">
                    <Brain className="h-4 w-4" />
                    Chat Model (LLM)
                  </h3>

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-1">
                      Provider
                    </label>
                    {/* Custom dropdown - replaces native select for consistent styling */}
                    <div className="relative">
                      <button
                        type="button"
                        onClick={() => setIsProviderDropdownOpen(!isProviderDropdownOpen)}
                        className={`w-full rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary text-left flex items-center justify-between focus:ring-1 focus:outline-none ${getInputErrorClass('llm', 'endpoint') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
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
                                // Set URL for presets with URLs, clear for Custom/Azure
                                updateAiConfig('llmBaseUrl', preset.url);
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
                        className={`w-full mt-2 rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-secondary/50 focus:ring-1 focus:outline-none ${getInputErrorClass('llm', 'endpoint') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
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
                      className={`w-full rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-secondary/50 focus:ring-1 focus:outline-none ${getInputErrorClass('llm', 'auth') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
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
                      placeholder="gpt-4o, claude-sonnet-4-5, llama3.1"
                      value={aiConfig.llmModel}
                      onChange={(e) => updateAiConfig('llmModel', e.target.value)}
                      className={`w-full rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-secondary/50 focus:ring-1 focus:outline-none ${getInputErrorClass('llm', 'model') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
                    />
                  </div>
                </div>

                {/* TEMPORARY: Embedding Configuration hidden for MVP launch
                <div className="space-y-4">
                  <h3 className="text-base font-semibold text-text-primary flex items-center gap-2">
                    <Search className="h-4 w-4" />
                    Embedding Model
                  </h3>

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-1">
                      Base URL
                    </label>
                    <input
                      type="text"
                      placeholder="https://api.openai.com/v1"
                      value={aiConfig.embeddingBaseUrl}
                      onChange={(e) => updateAiConfig('embeddingBaseUrl', e.target.value)}
                      className={`w-full rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-secondary/50 focus:ring-1 focus:outline-none ${getInputErrorClass('embedding', 'endpoint') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
                    />
                    <p className="mt-1 text-xs text-text-tertiary">
                      Leave empty to use same endpoint as Chat Model
                    </p>
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-1">
                      API Key
                    </label>
                    <input
                      type="password"
                      placeholder="sk-..."
                      value={aiConfig.embeddingApiKey}
                      onChange={(e) => updateAiConfig('embeddingApiKey', e.target.value)}
                      className={`w-full rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-secondary/50 focus:ring-1 focus:outline-none ${getInputErrorClass('embedding', 'auth') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
                    />
                    <p className="mt-1 text-xs text-text-tertiary">
                      Leave empty to use same key as Chat Model
                    </p>
                  </div>

                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-1">
                      Model
                    </label>
                    <input
                      type="text"
                      placeholder="text-embedding-3-small, nomic-embed-text"
                      value={aiConfig.embeddingModel}
                      onChange={(e) => updateAiConfig('embeddingModel', e.target.value)}
                      className={`w-full rounded-lg border bg-surface-primary px-3 py-2 text-sm text-text-primary placeholder:text-text-secondary/50 focus:ring-1 focus:outline-none ${getInputErrorClass('embedding', 'model') || 'border-border-light focus:border-brand-purple focus:ring-brand-purple'}`}
                    />
                  </div>
                </div>
                END TEMPORARY: Embedding Configuration */}
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
                    disabled={isSaving || !canSaveConfig('byok')}
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
                The Ekaya Community Models are free to use. We fine-tune the Community Models using anonymized schema, natural language and SQL queries from participants. The end result is a more robust and accurate model than the frontier models in this domain.
              </p>
              <div className="flex items-center justify-center py-8 text-text-tertiary">
                <span className="text-lg font-medium">Coming Soon</span>
              </div>
            </div>
          )}
          {/* END TEMPORARY: Original ManagedAIOptionPanel code preserved below
          {selectedAIOption === 'community' && (
            <div className="rounded-b-lg bg-surface-secondary p-6 border border-t-0 border-border-light">
              <ManagedAIOptionPanel
                optionId="community"
                description="The Ekaya Community Models are free to use. We fine-tune the Community Models using anonymized schema, natural language and SQL queries from participants. The end result is a more robust and accurate model than the frontier models in this domain."
                unavailableMessage="Community models are not configured on this server."
                helpText="No API key required - uses Ekaya-hosted models."
                aiOptions={aiOptions}
                isTesting={isTesting}
                isSaving={isSaving}
                testResult={testResult}
                saveError={saveError}
                activeAIConfig={activeAIConfig}
                canSave={canSaveConfig('community')}
                onTest={() => handleTestConnection('community')}
                onSave={() => handleSaveConfig('community')}
                onRemove={() => setShowRemoveConfirmation(true)}
              />
            </div>
          )}
          */}

          {/* TEMPORARY: Embedded - Coming Soon for MVP launch */}
          {selectedAIOption === 'embedded' && (
            <div className="rounded-b-lg bg-surface-secondary p-6 border border-t-0 border-border-light">
              <p className="text-text-secondary mb-4">
                Ekaya offers custom models that you can host on your own infrastructure or as part of your hybrid cloud so that data never leaves your security boundary. Your data is not used for training these models although Ekaya can build models customized for your internal needs.
              </p>
              <p className="text-text-secondary mb-4">
                These models have been fine-tuned for this domain and include prevention of SQL and LLM prompt injection attacks as well as detecting data leakage. These models are required for some features.
              </p>
              <div className="flex items-center justify-center py-8 text-text-tertiary">
                <span className="text-lg font-medium">Coming Soon</span>
              </div>
            </div>
          )}
          {/* END TEMPORARY: Original ManagedAIOptionPanel code preserved below
          {selectedAIOption === 'embedded' && (
            <div className="rounded-b-lg bg-surface-secondary p-6 border border-t-0 border-border-light">
              <ManagedAIOptionPanel
                optionId="embedded"
                description={[
                  "Ekaya offers custom models that you can host on your own infrastructure or as part of your hybrid cloud so that data never leaves your security boundary. Your data is not used for training these models although Ekaya can build models customized for your internal needs.",
                  "These models have been fine-tuned for this domain and include prevention of SQL and LLM prompt injection attacks as well as detecting data leakage. These models are required for some features."
                ]}
                unavailableMessage="Embedded models are not configured on this server."
                helpText="No API key required - uses customer-hosted models."
                aiOptions={aiOptions}
                isTesting={isTesting}
                isSaving={isSaving}
                testResult={testResult}
                saveError={saveError}
                activeAIConfig={activeAIConfig}
                canSave={canSaveConfig('embedded')}
                onTest={() => handleTestConnection('embedded')}
                onSave={() => handleSaveConfig('embedded')}
                onRemove={() => setShowRemoveConfirmation(true)}
              />
            </div>
          )}
          */}
        </div>

        {/* Status message when AI not configured (shown after AI selector, before tiles) */}
        {hasSelectedTables && !activeAIConfig && (
          <p className="text-sm text-red-500 mb-4">
            Configure an AI model above to enable Intelligence features.
          </p>
        )}

        <div className="grid gap-6 md:grid-cols-3">
          {intelligenceTiles.map(renderTile)}
        </div>
      </section>

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
    </div>
  );
};

export default ProjectDashboard;
