import {
  TrendingUp,
  Plus,
  Code,
  Edit2,
  Save,
  X,
  CheckCircle2,
  BarChart,
  Database,
} from 'lucide-react';
import type React from 'react';
import { useState } from 'react';

import { Button } from '../ui/Button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../ui/Card';

type MetricCategory = 'Performance' | 'Financial' | 'Customer' | 'Operational' | 'Growth';

interface Metric {
  id: number;
  name: string;
  description: string;
  sql: string;
  category: MetricCategory;
  createdAt: Date;
  verified: boolean;
  lastUpdated: Date;
}

interface NewMetric {
  name: string;
  description: string;
  sql: string;
  category: MetricCategory;
}

interface MetricsViewProps {
  metrics: Metric[];
  setMetrics: React.Dispatch<React.SetStateAction<Metric[]>>;
}

const MetricsView = ({ metrics, setMetrics }: MetricsViewProps) => {
  const [isAddingMetric, setIsAddingMetric] = useState<boolean>(false);
  const [editingMetric, setEditingMetric] = useState<Metric | null>(null);
  const [newMetric, setNewMetric] = useState<NewMetric>({
    name: '',
    description: '',
    sql: '',
    category: 'Performance',
  });

  // Handle adding new metric
  const handleAddMetric = (): void => {
    if (!newMetric.name || !newMetric.description) return;

    const metric: Metric = {
      id: metrics.length + 1,
      ...newMetric,
      createdAt: new Date(),
      verified: false,
      lastUpdated: new Date(),
    };

    setMetrics([...metrics, metric]);
    setNewMetric({
      name: '',
      description: '',
      sql: '',
      category: 'Performance',
    });
    setIsAddingMetric(false);
  };

  // Handle editing metric
  const handleSaveEdit = (): void => {
    if (!editingMetric) return;

    setMetrics(
      metrics.map((m) =>
        m.id === editingMetric.id
          ? { ...editingMetric, lastUpdated: new Date() }
          : m
      )
    );
    setEditingMetric(null);
  };

  // Handle deleting metric
  const handleDeleteMetric = (id: number): void => {
    setMetrics(metrics.filter((m) => m.id !== id));
  };

  // Handle verifying metric
  const handleVerifyMetric = (id: number): void => {
    setMetrics(
      metrics.map((m) => (m.id === id ? { ...m, verified: !m.verified } : m))
    );
  };

  // Categories for metrics
  const categories: MetricCategory[] = [
    'Performance',
    'Financial',
    'Customer',
    'Operational',
    'Growth',
  ];

  return (
    <>
      {/* Metrics List */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <TrendingUp className="h-5 w-5 text-purple-500" />
              <CardTitle>Business Metrics</CardTitle>
            </div>
            <Button
              size="sm"
              onClick={() => setIsAddingMetric(true)}
              className="flex items-center gap-2"
            >
              <Plus className="h-4 w-4" />
              Add Metric
            </Button>
          </div>
          <CardDescription>
            Define important metrics and how they are calculated -{' '}
            {metrics.filter((m) => m.verified).length}/{metrics.length} verified
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4 max-h-96 overflow-y-auto">
            {/* Add New Metric Form */}
            {isAddingMetric && (
              <div className="p-4 border-2 border-purple-500 rounded-lg bg-purple-50/50 dark:bg-purple-950/20">
                <h4 className="font-medium text-sm mb-3">Add New Metric</h4>
                <div className="space-y-3">
                  <div>
                    <label className="text-xs text-text-secondary">
                      Metric Name
                    </label>
                    <input
                      type="text"
                      value={newMetric.name}
                      onChange={(e) =>
                        setNewMetric({ ...newMetric, name: e.target.value })
                      }
                      placeholder="e.g., Monthly Recurring Revenue"
                      className="w-full px-3 py-2 text-sm border border-border-light rounded bg-surface-primary"
                    />
                  </div>
                  <div>
                    <label className="text-xs text-text-secondary">
                      Description
                    </label>
                    <input
                      type="text"
                      value={newMetric.description}
                      onChange={(e) =>
                        setNewMetric({
                          ...newMetric,
                          description: e.target.value,
                        })
                      }
                      placeholder="How this metric is calculated"
                      className="w-full px-3 py-2 text-sm border border-border-light rounded bg-surface-primary"
                    />
                  </div>
                  <div>
                    <label className="text-xs text-text-secondary">
                      Category
                    </label>
                    <select
                      value={newMetric.category}
                      onChange={(e) =>
                        setNewMetric({
                          ...newMetric,
                          category: e.target.value as MetricCategory,
                        })
                      }
                      className="w-full px-3 py-2 text-sm border border-border-light rounded bg-surface-primary"
                    >
                      {categories.map((cat) => (
                        <option key={cat} value={cat}>
                          {cat}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label className="text-xs text-text-secondary">
                      SQL Query (Optional)
                    </label>
                    <textarea
                      value={newMetric.sql}
                      onChange={(e) =>
                        setNewMetric({ ...newMetric, sql: e.target.value })
                      }
                      placeholder="SELECT SUM(amount) FROM orders WHERE status = 'completed'..."
                      className="w-full px-3 py-2 text-sm border border-border-light rounded bg-surface-primary font-mono text-xs h-24"
                    />
                  </div>
                  <div className="flex gap-2">
                    <Button size="sm" onClick={handleAddMetric}>
                      <Save className="h-4 w-4 mr-1" />
                      Save Metric
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setIsAddingMetric(false);
                        setNewMetric({
                          name: '',
                          description: '',
                          sql: '',
                          category: 'Performance',
                        });
                      }}
                    >
                      Cancel
                    </Button>
                  </div>
                </div>
              </div>
            )}

            {/* Metrics List */}
            {metrics.length === 0 && !isAddingMetric ? (
              <p className="text-sm text-text-secondary text-center py-8">
                No metrics defined yet. Click &quot;Add Metric&quot; to define your
                important business metrics.
              </p>
            ) : (
              metrics.map((metric) => (
                <div
                  key={metric.id}
                  className={`p-4 rounded-lg border transition-all ${
                    metric.verified
                      ? 'border-green-500/30 bg-green-500/5'
                      : 'border-border-light bg-surface-secondary'
                  }`}
                >
                  {editingMetric?.id === metric.id ? (
                    // Edit Mode
                    <div className="space-y-3">
                      <input
                        type="text"
                        value={editingMetric.name}
                        onChange={(e) =>
                          setEditingMetric({
                            ...editingMetric,
                            name: e.target.value,
                          })
                        }
                        className="w-full px-3 py-2 text-sm border border-border-light rounded bg-surface-primary font-medium"
                      />
                      <input
                        type="text"
                        value={editingMetric.description}
                        onChange={(e) =>
                          setEditingMetric({
                            ...editingMetric,
                            description: e.target.value,
                          })
                        }
                        className="w-full px-3 py-2 text-sm border border-border-light rounded bg-surface-primary"
                      />
                      <textarea
                        value={editingMetric.sql}
                        onChange={(e) =>
                          setEditingMetric({
                            ...editingMetric,
                            sql: e.target.value,
                          })
                        }
                        className="w-full px-3 py-2 text-sm border border-border-light rounded bg-surface-primary font-mono text-xs h-24"
                      />
                      <div className="flex gap-2">
                        <Button size="sm" onClick={handleSaveEdit}>
                          <Save className="h-4 w-4 mr-1" />
                          Save
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setEditingMetric(null)}
                        >
                          Cancel
                        </Button>
                      </div>
                    </div>
                  ) : (
                    // View Mode
                    <div>
                      <div className="flex items-start justify-between mb-2">
                        <div className="flex items-start gap-3">
                          <button
                            onClick={() => handleVerifyMetric(metric.id)}
                            className="mt-0.5"
                          >
                            <CheckCircle2
                              className={`h-4 w-4 ${
                                metric.verified
                                  ? 'text-green-500'
                                  : 'text-gray-400'
                              }`}
                            />
                          </button>
                          <div className="flex-1">
                            <div className="flex items-center gap-2 mb-1">
                              <h4 className="font-medium text-sm text-text-primary">
                                {metric.name}
                              </h4>
                              <span className="text-xs px-2 py-0.5 bg-purple-500/10 text-purple-600 rounded">
                                {metric.category}
                              </span>
                            </div>
                            <p className="text-sm text-text-secondary mb-2">
                              {metric.description}
                            </p>
                            {metric.sql && (
                              <div className="mt-2 p-2 bg-surface-primary rounded border border-border-light">
                                <div className="flex items-center gap-2 mb-1">
                                  <Code className="h-3 w-3 text-text-tertiary" />
                                  <span className="text-xs text-text-tertiary">
                                    SQL Query
                                  </span>
                                </div>
                                <pre className="text-xs font-mono text-text-primary overflow-x-auto">
                                  {metric.sql}
                                </pre>
                              </div>
                            )}
                            <div className="flex items-center gap-4 mt-2 text-xs text-text-tertiary">
                              <span>
                                Created: {metric.createdAt.toLocaleDateString()}
                              </span>
                              <span>
                                Updated:{' '}
                                {metric.lastUpdated.toLocaleDateString()}
                              </span>
                            </div>
                          </div>
                        </div>
                        <div className="flex gap-1">
                          <button
                            onClick={() => setEditingMetric(metric)}
                            className="p-1 hover:bg-surface-secondary rounded"
                          >
                            <Edit2 className="h-4 w-4 text-text-tertiary" />
                          </button>
                          <button
                            onClick={() => handleDeleteMetric(metric.id)}
                            className="p-1 hover:bg-surface-secondary rounded"
                          >
                            <X className="h-4 w-4 text-text-tertiary hover:text-red-500" />
                          </button>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              ))
            )}
          </div>

          {/* Helper Text */}
          <div className="mt-4 pt-4 border-t border-border-light">
            <h4 className="text-sm font-medium text-text-primary mb-2 flex items-center gap-2">
              <BarChart className="h-4 w-4 text-purple-500" />
              Common Metrics to Define
            </h4>
            <div className="grid grid-cols-2 gap-2 text-xs text-text-secondary">
              <div className="space-y-1">
                <p>• Monthly Recurring Revenue (MRR)</p>
                <p>• Customer Acquisition Cost (CAC)</p>
                <p>• Customer Lifetime Value (CLV)</p>
                <p>• Churn Rate</p>
                <p>• Average Order Value (AOV)</p>
              </div>
              <div className="space-y-1">
                <p>• Gross Margin</p>
                <p>• Conversion Rate</p>
                <p>• Monthly Active Users (MAU)</p>
                <p>• Net Promoter Score (NPS)</p>
                <p>• Revenue Growth Rate</p>
              </div>
            </div>
            <div className="mt-3 p-2 bg-blue-50 dark:bg-blue-950/20 rounded text-xs text-blue-600 dark:text-blue-400">
              <Database className="h-3 w-3 inline mr-1" />
              Tip: Paste SQL queries to help the system understand exact
              calculations for your metrics
            </div>
          </div>
        </CardContent>
      </Card>
    </>
  );
};

export default MetricsView;
