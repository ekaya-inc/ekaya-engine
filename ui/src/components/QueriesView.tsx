import {
  Database,
  Plus,
  Edit3,
  Trash2,
  Play,
  Save,
  X,
  MessageSquare,
  FileText,
  CheckCircle2,
  AlertCircle,
  Copy,
  Search,
} from 'lucide-react';
import { useState } from 'react';

import { Button } from './ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/Card';
import { Input } from './ui/Input';

export type QueryCategory =
  | 'general'
  | 'critical'
  | 'analytics'
  | 'reporting'
  | 'operational';

export interface Query {
  id: number;
  naturalLanguagePrompt: string;
  additionalContext: string;
  sqlQuery: string;
  category: QueryCategory;
  isActive: boolean;
  createdAt: Date;
  lastUsed: Date | null;
  usageCount: number;
  updatedAt?: Date;
}

interface NewQuery {
  naturalLanguagePrompt: string;
  additionalContext: string;
  sqlQuery: string;
  category: QueryCategory;
  isActive: boolean;
}

interface QueriesViewProps {
  queries: Query[];
  setQueries: React.Dispatch<React.SetStateAction<Query[]>>;
}

const QueriesView = ({ queries, setQueries }: QueriesViewProps) => {
  const [selectedQuery, setSelectedQuery] = useState<Query | null>(null);
  const [isCreating, setIsCreating] = useState<boolean>(false);
  const [editingQuery, setEditingQuery] = useState<Query | null>(null);
  const [searchTerm, setSearchTerm] = useState<string>('');
  const [newQuery, setNewQuery] = useState<NewQuery>({
    naturalLanguagePrompt: '',
    additionalContext: '',
    sqlQuery: '',
    category: 'general',
    isActive: true,
  });

  // Filter queries based on search
  const filteredQueries = queries.filter(
    (query) =>
      query.naturalLanguagePrompt
        .toLowerCase()
        .includes(searchTerm.toLowerCase()) ||
      query.sqlQuery.toLowerCase().includes(searchTerm.toLowerCase()) ||
      query.category.toLowerCase().includes(searchTerm.toLowerCase())
  );

  // Handle creating a new query
  const handleCreateQuery = (): void => {
    if (
      !newQuery.naturalLanguagePrompt.trim() ||
      !newQuery.sqlQuery.trim()
    ) {
      return;
    }

    const query: Query = {
      id: Date.now(),
      ...newQuery,
      createdAt: new Date(),
      lastUsed: null,
      usageCount: 0,
    };

    setQueries((prev) => [...prev, query]);
    setNewQuery({
      naturalLanguagePrompt: '',
      additionalContext: '',
      sqlQuery: '',
      category: 'general',
      isActive: true,
    });
    setIsCreating(false);
    setSelectedQuery(query);
  };

  // Handle editing a query
  const handleEditQuery = (query: Query): void => {
    setEditingQuery({ ...query });
    setSelectedQuery(query);
  };

  // Handle saving edited query
  const handleSaveEdit = (): void => {
    if (!editingQuery) return;

    if (
      !editingQuery.naturalLanguagePrompt.trim() ||
      !editingQuery.sqlQuery.trim()
    ) {
      return;
    }

    setQueries((prev) =>
      prev.map((q) =>
        q.id === editingQuery.id
          ? { ...editingQuery, updatedAt: new Date() }
          : q
      )
    );
    setSelectedQuery(editingQuery);
    setEditingQuery(null);
  };

  // Handle deleting a query
  const handleDeleteQuery = (queryId: number): void => {
    if (window.confirm('Are you sure you want to delete this query?')) {
      setQueries((prev) => prev.filter((q) => q.id !== queryId));
      if (selectedQuery?.id === queryId) {
        setSelectedQuery(null);
      }
    }
  };

  // Handle toggling query active status
  const handleToggleActive = (queryId: number): void => {
    setQueries((prev) =>
      prev.map((q) => (q.id === queryId ? { ...q, isActive: !q.isActive } : q))
    );
  };

  // Handle testing a query
  const handleTestQuery = (query: Query): void => {
    setQueries((prev) =>
      prev.map((q) =>
        q.id === query.id
          ? { ...q, lastUsed: new Date(), usageCount: q.usageCount + 1 }
          : q
      )
    );
    alert('Query executed successfully! (This is a simulation)');
  };

  // Copy query to clipboard
  const handleCopyQuery = (sqlQuery: string): void => {
    navigator.clipboard.writeText(sqlQuery);
  };

  const getCategoryColor = (category: QueryCategory): string => {
    switch (category) {
      case 'critical':
        return 'text-red-500 bg-red-500/10 border-red-500/30';
      case 'analytics':
        return 'text-blue-500 bg-blue-500/10 border-blue-500/30';
      case 'reporting':
        return 'text-green-500 bg-green-500/10 border-green-500/30';
      case 'operational':
        return 'text-orange-500 bg-orange-500/10 border-orange-500/30';
      default:
        return 'text-gray-500 bg-gray-500/10 border-gray-500/30';
    }
  };

  const getCategoryDot = (category: QueryCategory): string => {
    switch (category) {
      case 'critical':
        return 'bg-red-500';
      case 'analytics':
        return 'bg-blue-500';
      case 'reporting':
        return 'bg-green-500';
      case 'operational':
        return 'bg-orange-500';
      default:
        return 'bg-gray-500';
    }
  };

  return (
    <div className="flex h-[calc(100vh-12rem)] gap-6">
      {/* Left Sidebar - Query List */}
      <div className="w-80 flex flex-col">
        <Card className="h-full flex flex-col">
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between mb-2">
              <CardTitle className="text-lg">Queries</CardTitle>
              <Button
                size="sm"
                onClick={() => {
                  setIsCreating(true);
                  setSelectedQuery(null);
                  setEditingQuery(null);
                }}
                className="h-8 px-2"
              >
                <Plus className="h-4 w-4" />
              </Button>
            </div>
            <div className="relative">
              <Search className="absolute left-2 top-2.5 h-4 w-4 text-text-tertiary" />
              <Input
                placeholder="Search queries..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="pl-8 h-9"
              />
            </div>
          </CardHeader>
          <CardContent className="flex-1 overflow-y-auto p-2">
            {filteredQueries.length === 0 ? (
              <div className="text-center py-8">
                <Database className="h-8 w-8 text-text-tertiary mx-auto mb-2" />
                <p className="text-sm text-text-secondary">
                  {searchTerm ? 'No queries found' : 'No queries created yet'}
                </p>
              </div>
            ) : (
              <div className="space-y-1">
                {filteredQueries.map((query) => (
                  <button
                    key={query.id}
                    onClick={() => {
                      setSelectedQuery(query);
                      setIsCreating(false);
                      setEditingQuery(null);
                    }}
                    className={`w-full text-left p-2 rounded-lg transition-colors ${
                      selectedQuery?.id === query.id
                        ? 'bg-purple-500/10 border border-purple-500/30'
                        : 'hover:bg-surface-secondary/50'
                    } ${!query.isActive ? 'opacity-50' : ''}`}
                  >
                    <div className="flex items-center justify-between mb-0.5">
                      <div className="flex items-center gap-1.5">
                        <div
                          className={`h-1.5 w-1.5 rounded-full ${getCategoryDot(query.category)}`}
                        />
                        <span className="text-xs text-text-tertiary uppercase">
                          {query.category}
                        </span>
                      </div>
                      {!query.isActive && (
                        <AlertCircle className="h-3 w-3 text-gray-500" />
                      )}
                    </div>
                    <div className="text-sm font-medium text-text-primary line-clamp-1">
                      {query.naturalLanguagePrompt}
                    </div>
                    {query.usageCount > 0 && (
                      <div className="text-xs text-text-tertiary mt-0.5">
                        Used {query.usageCount} times
                      </div>
                    )}
                  </button>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Right Side - Query Editor/Viewer */}
      <div className="flex-1">
        <Card className="h-full flex flex-col">
          {isCreating ? (
            // Create New Query Form
            <>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <CardTitle>Create New Query</CardTitle>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => setIsCreating(false)}
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              </CardHeader>
              <CardContent className="flex-1 overflow-y-auto space-y-4">
                <div>
                  <label className="block text-sm font-medium text-text-primary mb-2">
                    Natural Language Prompt{' '}
                    <span className="text-red-500">*</span>
                  </label>
                  <textarea
                    value={newQuery.naturalLanguagePrompt}
                    onChange={(e) =>
                      setNewQuery({
                        ...newQuery,
                        naturalLanguagePrompt: e.target.value,
                      })
                    }
                    placeholder="Describe what you want to query in natural language..."
                    className="w-full h-24 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-text-primary mb-2">
                    Additional Context
                  </label>
                  <textarea
                    value={newQuery.additionalContext}
                    onChange={(e) =>
                      setNewQuery({
                        ...newQuery,
                        additionalContext: e.target.value,
                      })
                    }
                    placeholder="Any additional context or information..."
                    className="w-full h-20 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-text-primary mb-2">
                    SQL Query <span className="text-red-500">*</span>
                  </label>
                  <textarea
                    value={newQuery.sqlQuery}
                    onChange={(e) =>
                      setNewQuery({ ...newQuery, sqlQuery: e.target.value })
                    }
                    placeholder="SELECT * FROM..."
                    className="w-full h-40 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary font-mono text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-text-primary mb-2">
                    Category
                  </label>
                  <select
                    value={newQuery.category}
                    onChange={(e) =>
                      setNewQuery({
                        ...newQuery,
                        category: e.target.value as QueryCategory,
                      })
                    }
                    className="w-full px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                  >
                    <option value="general">General</option>
                    <option value="critical">Critical</option>
                    <option value="analytics">Analytics</option>
                    <option value="reporting">Reporting</option>
                    <option value="operational">Operational</option>
                  </select>
                </div>

                <div className="flex justify-end gap-2 pt-4 border-t border-border-light">
                  <Button
                    variant="outline"
                    onClick={() => setIsCreating(false)}
                  >
                    Cancel
                  </Button>
                  <Button
                    onClick={handleCreateQuery}
                    disabled={
                      !newQuery.naturalLanguagePrompt.trim() ||
                      !newQuery.sqlQuery.trim()
                    }
                  >
                    <Save className="mr-2 h-4 w-4" />
                    Create Query
                  </Button>
                </div>
              </CardContent>
            </>
          ) : editingQuery ? (
            // Edit Query Form
            <>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <CardTitle>Edit Query</CardTitle>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => setEditingQuery(null)}
                  >
                    <X className="h-4 w-4" />
                  </Button>
                </div>
              </CardHeader>
              <CardContent className="flex-1 overflow-y-auto space-y-4">
                <div>
                  <label className="block text-sm font-medium text-text-primary mb-2">
                    Natural Language Prompt{' '}
                    <span className="text-red-500">*</span>
                  </label>
                  <textarea
                    value={editingQuery.naturalLanguagePrompt}
                    onChange={(e) =>
                      setEditingQuery({
                        ...editingQuery,
                        naturalLanguagePrompt: e.target.value,
                      })
                    }
                    placeholder="Describe what you want to query in natural language..."
                    className="w-full h-24 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-text-primary mb-2">
                    Additional Context
                  </label>
                  <textarea
                    value={editingQuery.additionalContext}
                    onChange={(e) =>
                      setEditingQuery({
                        ...editingQuery,
                        additionalContext: e.target.value,
                      })
                    }
                    placeholder="Any additional context or information..."
                    className="w-full h-20 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-text-primary mb-2">
                    SQL Query <span className="text-red-500">*</span>
                  </label>
                  <textarea
                    value={editingQuery.sqlQuery}
                    onChange={(e) =>
                      setEditingQuery({
                        ...editingQuery,
                        sqlQuery: e.target.value,
                      })
                    }
                    placeholder="SELECT * FROM..."
                    className="w-full h-40 px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary font-mono text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-text-primary mb-2">
                    Category
                  </label>
                  <select
                    value={editingQuery.category}
                    onChange={(e) =>
                      setEditingQuery({
                        ...editingQuery,
                        category: e.target.value as QueryCategory,
                      })
                    }
                    className="w-full px-3 py-2 border border-border-light rounded-lg bg-surface-primary text-text-primary focus:outline-none focus:ring-2 focus:ring-purple-500"
                  >
                    <option value="general">General</option>
                    <option value="critical">Critical</option>
                    <option value="analytics">Analytics</option>
                    <option value="reporting">Reporting</option>
                    <option value="operational">Operational</option>
                  </select>
                </div>

                <div className="flex justify-end gap-2 pt-4 border-t border-border-light">
                  <Button
                    variant="outline"
                    onClick={() => setEditingQuery(null)}
                  >
                    Cancel
                  </Button>
                  <Button
                    onClick={handleSaveEdit}
                    disabled={
                      !editingQuery.naturalLanguagePrompt.trim() ||
                      !editingQuery.sqlQuery.trim()
                    }
                  >
                    <Save className="mr-2 h-4 w-4" />
                    Save Changes
                  </Button>
                </div>
              </CardContent>
            </>
          ) : selectedQuery ? (
            // View Query Details
            <>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <div>
                    <div className="flex items-center gap-2 mb-1">
                      <CardTitle>{selectedQuery.naturalLanguagePrompt}</CardTitle>
                      <span
                        className={`text-xs px-2 py-1 rounded border ${getCategoryColor(selectedQuery.category)}`}
                      >
                        {selectedQuery.category}
                      </span>
                    </div>
                    <CardDescription>
                      Created{' '}
                      {selectedQuery.createdAt.toLocaleDateString()} • Used{' '}
                      {selectedQuery.usageCount} times
                    </CardDescription>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleToggleActive(selectedQuery.id)}
                      title={
                        selectedQuery.isActive
                          ? 'Deactivate query'
                          : 'Activate query'
                      }
                    >
                      {selectedQuery.isActive ? (
                        <CheckCircle2 className="h-4 w-4 text-green-500" />
                      ) : (
                        <AlertCircle className="h-4 w-4 text-gray-500" />
                      )}
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleEditQuery(selectedQuery)}
                    >
                      <Edit3 className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDeleteQuery(selectedQuery.id)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="flex-1 overflow-y-auto space-y-6">
                {selectedQuery.additionalContext && (
                  <div>
                    <div className="flex items-center gap-2 mb-2">
                      <MessageSquare className="h-4 w-4 text-text-tertiary" />
                      <h3 className="text-sm font-medium text-text-primary">
                        Additional Context
                      </h3>
                    </div>
                    <p className="text-sm text-text-secondary bg-surface-secondary p-3 rounded-lg">
                      {selectedQuery.additionalContext}
                    </p>
                  </div>
                )}

                <div>
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <FileText className="h-4 w-4 text-text-tertiary" />
                      <h3 className="text-sm font-medium text-text-primary">
                        SQL Query
                      </h3>
                    </div>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleCopyQuery(selectedQuery.sqlQuery)}
                    >
                      <Copy className="h-3 w-3 mr-1" />
                      Copy
                    </Button>
                  </div>
                  <pre className="text-sm bg-surface-secondary p-4 rounded-lg overflow-x-auto font-mono">
                    {selectedQuery.sqlQuery}
                  </pre>
                </div>

                <div className="flex gap-2 pt-4 border-t border-border-light">
                  <Button
                    onClick={() => handleTestQuery(selectedQuery)}
                    disabled={!selectedQuery.isActive}
                  >
                    <Play className="mr-2 h-4 w-4" />
                    Test Query
                  </Button>
                </div>
              </CardContent>
            </>
          ) : (
            // Empty State
            <CardContent className="flex-1 flex items-center justify-center">
              <div className="text-center">
                <Database className="h-12 w-12 text-text-tertiary mx-auto mb-4" />
                <h3 className="text-lg font-medium text-text-primary mb-2">
                  No Query Selected
                </h3>
                <p className="text-sm text-text-secondary mb-4">
                  Select a query from the list or create a new one
                </p>
                <Button
                  onClick={() => {
                    setIsCreating(true);
                    setSelectedQuery(null);
                  }}
                >
                  <Plus className="mr-2 h-4 w-4" />
                  Create New Query
                </Button>
              </div>
            </CardContent>
          )}
        </Card>
      </div>
    </div>
  );
};

export default QueriesView;
