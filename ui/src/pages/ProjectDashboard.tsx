import type { LucideIcon } from 'lucide-react';
import {
  BookOpen,
  Bot,
  BrainCircuit,
  Database,
  Layers,
  Lightbulb,
  ListTree,
  Loader2,
  MessageCircleQuestion,
  Network,
  Plus,
  Search,
  Server,
  Shield,
  Sparkles,
} from 'lucide-react';
import { useState, useEffect, useMemo } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import AIConfigWidget from '../components/AIConfigWidget';
import MCPLogo from '../components/icons/MCPLogo';
import { Button } from '../components/ui/Button';
import { Card, CardHeader, CardTitle } from '../components/ui/Card';
import { useDatasourceConnection } from '../contexts/DatasourceConnectionContext';
import { useInstalledApps } from '../hooks/useInstalledApps';
import { ontologyService } from '../services/ontologyService';
import type {
  AIOption,
  OntologyWorkflowStatus,
} from '../types';
import { APP_ID_AI_DATA_LIAISON, APP_ID_AI_AGENTS } from '../types';

type TileColor = 'blue' | 'green' | 'purple' | 'orange' | 'gray' | 'indigo' | 'cyan' | 'amber';

interface Tile {
  title: string;
  description?: string;
  icon: LucideIcon;
  path: string;
  disabled: boolean;
  disabledReason?: string;
  color: TileColor;
}

/**
 * ProjectDashboard - Navigation tiles for project workspace
 * Displays navigation options for database, schema, ontology, etc.
 * This is the index page under /projects/:pid
 *
 * Note: Schema and Queries tiles are disabled when no datasource is configured.
 * Ontology Extraction tile is disabled when no datasource is configured OR no tables are selected.
 * Users must first configure a datasource via the Datasource tile and select tables in the Schema page.
 */
const ProjectDashboard = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { isConnected, hasSelectedTables } = useDatasourceConnection();
  const { apps: installedApps } = useInstalledApps(pid);
  const [activeAIConfig, setActiveAIConfig] = useState<AIOption>(null);

  // Ontology workflow status for badge
  const [ontologyStatus, setOntologyStatus] = useState<OntologyWorkflowStatus | null>(null);

  // Subscribe to ontology status for badge display
  useEffect(() => {
    const unsubscribe = ontologyService.subscribe((status) => {
      setOntologyStatus(status);
    });
    return () => {
      unsubscribe();
    };
  }, []);

  const dataTiles: Tile[] = [
    {
      title: 'Datasource',
      description: 'Connect to your database and configure credentials.',
      icon: Database,
      path: `/projects/${pid}/datasource`,
      disabled: false, // Always enabled - needed to configure datasource
      color: 'blue',
    },
    {
      title: 'Schema',
      description: 'Select the tables and columns to include in your ontology.',
      icon: ListTree,
      path: `/projects/${pid}/schema`,
      disabled: !isConnected, // Disabled if no datasource configured
      color: 'green',
    },
    {
      title: 'Pre-Approved Queries',
      description: 'Create safe, parameterized queries your users can execute.',
      icon: Search,
      path: `/projects/${pid}/queries`,
      disabled: !isConnected, // Disabled if no datasource configured
      color: 'orange',
    },
  ];

  const intelligenceTiles: Tile[] = useMemo(() => {
    const tiles: Tile[] = [
      {
        title: 'Ontology Extraction',
        description: 'Run AI analysis to understand your schema and data semantics.',
        icon: Layers,
        path: `/projects/${pid}/ontology`,
        disabled: !isConnected || !hasSelectedTables || !activeAIConfig, // Disabled if no datasource, no tables, or no AI config
        color: 'purple',
      },
      {
        title: 'Ontology Questions',
        description: 'Review and answer questions the AI has about your data.',
        icon: MessageCircleQuestion,
        path: `/projects/${pid}/ontology-questions`,
        disabled: !isConnected || !hasSelectedTables || !activeAIConfig, // Disabled if no datasource, no tables, or no AI config
        color: 'amber',
      },
      {
        title: 'Project Knowledge',
        description: 'Domain facts and business rules that guide AI understanding.',
        icon: Lightbulb,
        path: `/projects/${pid}/project-knowledge`,
        disabled: !isConnected || !hasSelectedTables || !activeAIConfig, // Disabled if no datasource, no tables, or no AI config
        color: 'indigo',
      },
      {
        title: 'Enrichment',
        description: 'Review and refine AI-generated metadata for tables and columns.',
        icon: Sparkles,
        path: `/projects/${pid}/enrichment`,
        disabled: !isConnected || !hasSelectedTables || !activeAIConfig, // Disabled if no datasource, no tables, or no AI config
        color: 'orange',
      },
      {
        title: 'Relationships',
        description: 'View discovered foreign key and entity relationships.',
        icon: Network,
        path: `/projects/${pid}/relationships`,
        disabled: !isConnected || !hasSelectedTables || !activeAIConfig, // Disabled if no datasource, no tables, or no AI config
        color: 'indigo',
      },
      // HIDDEN: Glossary tile temporarily removed from dashboard (see plans/FIX-unhide-glossary-tile.md)
      // {
      //   title: 'Glossary',
      //   description: 'Business terms and their SQL definitions for consistent reporting.',
      //   icon: BookOpen,
      //   path: `/projects/${pid}/glossary`,
      //   disabled: !isConnected || !hasSelectedTables || !activeAIConfig,
      //   color: 'cyan',
      // },
    ];

    // Add Audit Log tile if AI Data Liaison is installed
    if (installedApps.some((app) => app.app_id === APP_ID_AI_DATA_LIAISON)) {
      tiles.push({
        title: 'Audit Log',
        description: 'Monitor query activity, security alerts, and governance events.',
        icon: Shield,
        path: `/projects/${pid}/audit`,
        disabled: false,
        color: 'green',
      });
    }

    return tiles;
  }, [pid, isConnected, hasSelectedTables, activeAIConfig, installedApps]);

  const applicationTiles: Tile[] = useMemo(() => {
    const tiles: Tile[] = [
      {
        title: 'MCP Server',
        description: 'Configure the tools AI can use to access your data via Model Context Protocol (MCP), the industry standard for integration.',
        icon: Server,
        path: `/projects/${pid}/mcp-server`,
        disabled: !isConnected, // Requires datasource
        disabledReason: 'Requires a datasource to be configured.',
        color: 'cyan',
      },
    ];

    // Add AI Data Liaison tile if installed
    if (installedApps.some((app) => app.app_id === APP_ID_AI_DATA_LIAISON)) {
      tiles.push({
        title: 'AI Data Liaison',
        description: 'Make Better Business Decisions 10x Faster and lower the burden on the data team.',
        icon: BrainCircuit,
        path: `/projects/${pid}/ai-data-liaison`,
        disabled: !isConnected,
        disabledReason: 'Requires MCP Server to be enabled.',
        color: 'blue',
      });
    }

    // Add AI Agents tile if installed
    if (installedApps.some((app) => app.app_id === APP_ID_AI_AGENTS)) {
      tiles.push({
        title: 'AI Agents and Automation',
        description: 'Connect AI coding agents and automation tools to your data via API key authentication.',
        icon: Bot,
        path: `/projects/${pid}/ai-agents`,
        disabled: !isConnected,
        disabledReason: 'Requires MCP Server to be enabled.',
        color: 'orange',
      });
    }

    return tiles;
  }, [pid, isConnected, installedApps]);

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
      amber: 'bg-amber-500/10 text-amber-500 hover:bg-amber-500/20',
    };

    return colorMap[color];
  };

  const renderTile = (tile: Tile) => {
    const Icon = tile.icon;
    const colorClasses = getColorClasses(tile.color);

    // Check for ontology badges
    const isOntologyTile = tile.title === 'Ontology Extraction';
    const isQuestionsTile = tile.title === 'Ontology Questions';
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
          <div className="flex items-center gap-5">
            <div className="relative shrink-0">
              <div
                className={`flex h-16 w-16 items-center justify-center rounded-lg ${colorClasses}`}
              >
                <Icon className="h-8 w-8" />
              </div>
              {/* Badge for pending questions (show on Ontology and Ontology Questions tiles) */}
              {(isOntologyTile || isQuestionsTile) && pendingQuestions > 0 && !tile.disabled && (
                <div className="absolute -top-1 -right-1 flex items-center gap-1 bg-amber-500 text-white text-xs font-medium px-2 py-0.5 rounded-full">
                  <MessageCircleQuestion className="h-3 w-3" />
                  {pendingQuestions}
                </div>
              )}
            </div>
            {tile.description && (
              <p className="text-xs text-text-tertiary leading-relaxed">{tile.description}</p>
            )}
          </div>
          <CardTitle className="text-xl mt-3">{tile.title}</CardTitle>
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
    const isMCPServerTile = tile.title === 'MCP Server';

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
          <div className="flex items-center gap-5">
            <div
              className={`shrink-0 flex h-24 w-24 items-center justify-center rounded-xl ${colorClasses}`}
            >
              {isMCPServerTile ? (
                <MCPLogo size={48} />
              ) : (
                <Icon className="h-12 w-12" />
              )}
            </div>
            {tile.description && (
              <p className="text-sm text-text-tertiary leading-relaxed">{tile.description}</p>
            )}
          </div>
          <CardTitle className="text-2xl mt-4">{tile.title}</CardTitle>
          {tile.disabled && tile.disabledReason && (
            <p className="text-sm text-text-tertiary mt-2">
              {tile.disabledReason}
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
        <div className="flex items-center justify-between mb-2">
          <h1 className="text-2xl font-semibold">Applications</h1>
          <Button
            variant="outline"
            size="sm"
            onClick={() => navigate(`/projects/${pid}/applications`)}
          >
            <Plus className="h-4 w-4" />
            Install Applications
          </Button>
        </div>
        <p className="text-text-secondary mb-4">
          Install applications that safely connect to your data through secure interfaces accessed only by authenticated and authorized users.
        </p>
        <div className="grid gap-6 md:grid-cols-2">
          {applicationTiles.map(renderApplicationTile)}
        </div>
      </section>

      {/* Data Section */}
      <section>
        <h1 className="text-2xl font-semibold mb-2">Data</h1>
        <p className="text-text-secondary mb-4">
          Enable data access safely and securely. The Data functionality is always available and does not require a Large Language Model (LLM).
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

        <AIConfigWidget
          projectId={pid ?? ''}
          disabled={!hasSelectedTables}
          onConfigChange={setActiveAIConfig}
        />

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
    </div>
  );
};

export default ProjectDashboard;
