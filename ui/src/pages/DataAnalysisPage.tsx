import { ArrowLeft, Play, CheckCircle, AlertCircle, Plus, X } from 'lucide-react';
import { useState, useCallback } from 'react';
import { useNavigate, useParams, useLocation } from 'react-router-dom';

import { Button } from '../components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';

interface Column {
  name: string;
  type: string;
  notNull: boolean;
}

interface SelectedTableData {
  name: string;
  columns: Column[];
}

interface LocationState {
  selectedTables?: SelectedTableData[];
}

type TaskStatus = 'pending' | 'running' | 'completed' | 'error';
type RelationshipType = '1:1' | '1:N' | 'N:1';

interface Relationship {
  from: string;
  fromColumn: string;
  to: string;
  toColumn: string;
  type: string;
  description: string;
}

interface TableTasks {
  [tableName: string]: TaskStatus;
}

interface Task {
  name: string;
  status: TaskStatus;
  tables?: TableTasks;
  relationships?: Relationship[];
}

interface TaskStates {
  [taskId: string]: Task;
}

interface NewRelationship {
  fromTable: string;
  fromColumn: string;
  type: RelationshipType;
  toTable: string;
  toColumn: string;
}

type TableColumns = Record<string, string[]>;

const DataAnalysisPage = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { pid } = useParams<{ pid: string }>();

  // Get selected tables from navigation state
  const locationState = location.state as LocationState | undefined;
  const selectedTablesData = locationState?.selectedTables ?? [];
  const selectedTables = selectedTablesData.map(t => t.name);

  // Build tableColumns from selected tables data
  const tableColumns: TableColumns = selectedTablesData.reduce((acc, table) => {
    acc[table.name] = table.columns.map(col => col.name);
    return acc;
  }, {} as TableColumns);

  // Task states: 'pending', 'running', 'completed', 'error'
  const [taskStates, setTaskStates] = useState<TaskStates>({
    'understand_data_shape': {
      name: 'Understand Data Shape',
      status: 'pending',
      tables: selectedTables.reduce((acc, table) => {
        acc[table] = 'pending';
        return acc;
      }, {} as TableTasks)
    },
    'understand_relationships': {
      name: 'Understand Relationships',
      status: 'pending',
      relationships: []
    }
  });

  // Discovered relationships (will be populated from API or analysis)
  const [_discoveredRelationships, _setDiscoveredRelationships] = useState<Relationship[]>([]);

  const [isRunningAll, setIsRunningAll] = useState<boolean>(false);
  const [isAddingRelationship, setIsAddingRelationship] = useState<boolean>(false);
  const [newRelationship, setNewRelationship] = useState<NewRelationship>({
    fromTable: '',
    fromColumn: '',
    type: '1:N',
    toTable: '',
    toColumn: ''
  });

  // Simulate running a single table analysis
  const runTableAnalysis = useCallback(async (taskId: string, tableName: string): Promise<void> => {
    setTaskStates(prev => {
      const task = prev[taskId];
      if (!task) return prev;
      return {
        ...prev,
        [taskId]: {
          ...task,
          tables: {
            ...task.tables,
            [tableName]: 'running'
          }
        }
      };
    });

    // Simulate API call delay
    await new Promise(resolve => setTimeout(resolve, 1000 + Math.random() * 2000));

    setTaskStates(prev => {
      const task = prev[taskId];
      if (!task) return prev;
      return {
        ...prev,
        [taskId]: {
          ...task,
          tables: {
            ...task.tables,
            [tableName]: 'completed'
          }
        }
      };
    });
  }, []);

  // Run analysis for a specific task
  const runTask = useCallback(async (taskId: string): Promise<void> => {
    const task = taskStates[taskId];
    if (!task) return;

    setTaskStates(prev => {
      const currentTask = prev[taskId];
      if (!currentTask) return prev;
      return {
        ...prev,
        [taskId]: {
          ...currentTask,
          status: 'running'
        }
      };
    });

    if (taskId === 'understand_data_shape') {
      const tables = Object.keys(task.tables ?? {});
      // Run all tables for this task in parallel
      await Promise.all(tables.map(table => runTableAnalysis(taskId, table)));
    } else if (taskId === 'understand_relationships') {
      // Simulate relationship discovery
      await new Promise(resolve => setTimeout(resolve, 2000));

      setTaskStates(prev => {
        const currentTask = prev[taskId];
        if (!currentTask) return prev;
        return {
          ...prev,
          [taskId]: {
            ...currentTask,
            relationships: _discoveredRelationships
          }
        };
      });
    }

    setTaskStates(prev => {
      const currentTask = prev[taskId];
      if (!currentTask) return prev;
      return {
        ...prev,
        [taskId]: {
          ...currentTask,
          status: 'completed'
        }
      };
    });
  }, [taskStates, runTableAnalysis, _discoveredRelationships]);

  // Run all tasks
  const runAllTasks = useCallback(async (): Promise<void> => {
    setIsRunningAll(true);

    // Run tasks in dependency order
    await runTask('understand_data_shape');
    await runTask('understand_relationships');

    setIsRunningAll(false);
  }, [runTask]);

  // Get status icon for table
  const getTableStatusIcon = (status: TaskStatus): JSX.Element => {
    switch (status) {
      case 'pending':
        return <div className="w-4 h-4 border-2 border-gray-300 rounded-full" />;
      case 'running':
        return (
          <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        );
      case 'completed':
        return <CheckCircle className="w-4 h-4 text-green-500" />;
      case 'error':
        return <AlertCircle className="w-4 h-4 text-red-500" />;
      default:
        return <div className="w-4 h-4 border-2 border-gray-300 rounded-full" />;
    }
  };

  // Get task status
  const getTaskStatus = (task: Task): TaskStatus => {
    if (task.tables) {
      const tableStatuses = Object.values(task.tables);
      if (tableStatuses.every(status => status === 'completed')) return 'completed';
      if (tableStatuses.some(status => status === 'running')) return 'running';
      if (tableStatuses.some(status => status === 'error')) return 'error';
      return 'pending';
    } else {
      // For relationship task
      return task.status;
    }
  };

  // Check if first task is completed (for dependency)
  const firstTask = taskStates['understand_data_shape'];
  const isFirstTaskCompleted: boolean = firstTask !== undefined && (
    firstTask.status === 'completed' || getTaskStatus(firstTask) === 'completed'
  );

  // Check if all tasks are completed
  const allTasksCompleted: boolean = Object.values(taskStates).every(task => {
    const status = getTaskStatus(task);
    return status === 'completed';
  });

  // Handle adding a new relationship
  const handleAddRelationship = useCallback((): void => {
    const relationshipType = newRelationship.type;
    const description = relationshipType === '1:1' ? 'has one' :
                       relationshipType === '1:N' ? 'has many' : 'belongs to';

    const relationship: Relationship = {
      from: newRelationship.fromTable,
      fromColumn: newRelationship.fromColumn,
      to: newRelationship.toTable,
      toColumn: newRelationship.toColumn,
      type: relationshipType,
      description: description
    };

    setTaskStates(prev => {
      const relTask = prev['understand_relationships'];
      if (!relTask) return prev;
      return {
        ...prev,
        'understand_relationships': {
          ...relTask,
          relationships: [...(relTask.relationships ?? []), relationship]
        }
      };
    });

    // Reset form
    setNewRelationship({
      fromTable: '',
      fromColumn: '',
      type: '1:N',
      toTable: '',
      toColumn: ''
    });
    setIsAddingRelationship(false);
  }, [newRelationship]);

  // Handle canceling add relationship
  const handleCancelAddRelationship = useCallback((): void => {
    setNewRelationship({
      fromTable: '',
      fromColumn: '',
      type: '1:N',
      toTable: '',
      toColumn: ''
    });
    setIsAddingRelationship(false);
  }, []);

  return (
    <div className="mx-auto max-w-6xl">
      <div className="mb-6">
        <div className="flex items-center justify-between mb-4">
          <Button
            variant="ghost"
            onClick={() => navigate(`/projects/${pid}/schema`)}
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Schema Selection
          </Button>
          <Button
            onClick={() => navigate(`/projects/${pid}`)}
            disabled={!allTasksCompleted}
            className="bg-blue-600 hover:bg-blue-700 text-white font-medium disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Done
          </Button>
        </div>
        <h1 className="text-3xl font-bold text-text-primary">Data Analysis</h1>
        <p className="mt-2 text-text-secondary">
          Ekaya will scan the tables and columns to prepare the schema for an ontology. Only select Run All if the effect on database performance is acceptable. Otherwise run each task manually and watch database performance.
        </p>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Schema Analysis Tasks</CardTitle>
              <CardDescription>
                Analyze selected tables to understand data structure and relationships
              </CardDescription>
            </div>
            <Button
              onClick={runAllTasks}
              disabled={isRunningAll}
              className="bg-green-600 hover:bg-green-700 text-white font-medium"
            >
              <Play className="mr-2 h-4 w-4" />
              {isRunningAll ? 'Running All...' : 'Run All'}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-6">
            {Object.entries(taskStates).map(([taskId, task], index) => {
              const taskStatus = getTaskStatus(task);
              const isTaskRunning = taskStatus === 'running';
              const isTaskCompleted = taskStatus === 'completed';

              return (
                <div key={taskId} className="border border-border-light rounded-lg p-4">
                  <div className="flex items-center justify-between mb-4">
                    <div className="flex-1">
                      <div className="flex items-center gap-3 mb-1">
                        <div className={`flex h-8 w-8 items-center justify-center rounded-full font-semibold text-sm ${
                          isTaskCompleted
                            ? 'bg-green-600 text-white'
                            : 'bg-blue-100 text-blue-700'
                        }`}>
                          {index + 1}
                        </div>
                        <span className="text-lg font-medium text-text-primary">
                          {task.name}
                        </span>
                        {isTaskCompleted && (
                          <CheckCircle className="w-5 h-5 text-green-500" />
                        )}
                      </div>
                      {taskId === 'understand_data_shape' && (
                        <div className="text-sm text-text-secondary">
                          This task analyzes data types, nullable fields, distinct counts, and value ranges for each column.
                        </div>
                      )}
                      {taskId === 'understand_relationships' && (
                        <div className="text-sm text-text-secondary">
                          This task identifies how tables join data and the nature of the relationship.
                        </div>
                      )}
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => runTask(taskId)}
                      disabled={isTaskRunning || isRunningAll || (taskId === 'understand_relationships' && !isFirstTaskCompleted)}
                    >
                      <Play className="mr-2 h-3 w-3" />
                      {isTaskRunning ? 'Running...' : 'Run'}
                    </Button>
                  </div>

                  <div className="ml-6 space-y-2">
                    {/* Display tables for data shape task */}
                    {taskId === 'understand_data_shape' && task.tables && Object.entries(task.tables).map(([tableName, status]) => (
                      <div key={tableName} className="flex items-center gap-3">
                        {getTableStatusIcon(status)}
                        <span className="font-medium text-blue-600">{tableName}</span>
                        {status === 'running' && (
                          <span className="text-sm text-blue-500">Processing...</span>
                        )}
                        {status === 'completed' && (
                          <span className="text-sm text-green-500">Complete</span>
                        )}
                        {status === 'error' && (
                          <span className="text-sm text-red-500">Error</span>
                        )}
                      </div>
                    ))}

                    {/* Display relationships for relationships task */}
                    {taskId === 'understand_relationships' && (
                      <div className="space-y-3">
                        {task.relationships && task.relationships.length > 0 ? (
                          <>
                            {task.relationships.map((rel, index) => (
                              <div key={index} className="flex items-center text-sm bg-surface-secondary/30 rounded-lg p-3 border border-border-light/50">
                                <div className="flex-1 flex items-center">
                                  <span className="font-medium text-blue-600 min-w-0 truncate">
                                    {rel.from}.{rel.fromColumn}
                                  </span>
                                </div>
                                <div className="flex-1 flex items-center justify-center relative px-6">
                                  <div className="absolute left-2 right-2 flex items-center">
                                    <div className="flex-1 border-t border-text-tertiary/60"></div>
                                    <div className="flex-1 border-t border-text-tertiary/60"></div>
                                  </div>
                                  <div className="relative bg-surface-primary px-4 py-1.5 rounded-full border border-border-light shadow-sm">
                                    <span className="text-text-secondary font-medium text-xs whitespace-nowrap">
                                      {rel.description} ({rel.type})
                                    </span>
                                  </div>
                                </div>
                                <div className="flex-1 flex items-center justify-end">
                                  <span className="font-medium text-blue-600 min-w-0 truncate">
                                    {rel.to}.{rel.toColumn}
                                  </span>
                                </div>
                                <div className="ml-3">
                                  <button
                                    onClick={() => {
                                      setTaskStates(prev => {
                                        const relTask = prev['understand_relationships'];
                                        if (!relTask) return prev;
                                        return {
                                          ...prev,
                                          'understand_relationships': {
                                            ...relTask,
                                            relationships: (relTask.relationships ?? []).filter((_, i) => i !== index)
                                          }
                                        };
                                      });
                                    }}
                                    className="w-6 h-6 flex items-center justify-center rounded-full hover:bg-red-100 text-red-500 hover:text-red-700 transition-colors"
                                  >
                                    <X className="h-3 w-3" />
                                  </button>
                                </div>
                              </div>
                            ))}

                            {/* Add relationship form */}
                            {isAddingRelationship ? (
                              <div className="flex items-center text-sm bg-blue-50 rounded-lg p-4 border-2 border-blue-200 shadow-sm">
                                <div className="flex-1 flex items-center gap-2">
                                  <select
                                    value={newRelationship.fromTable}
                                    onChange={(e) => setNewRelationship(prev => ({ ...prev, fromTable: e.target.value, fromColumn: '' }))}
                                    className="flex-1 px-3 py-2 border border-gray-300 rounded-md text-sm min-w-0 bg-white shadow-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                                  >
                                    <option value="">Select Table</option>
                                    {selectedTables.map(table => (
                                      <option key={table} value={table}>{table}</option>
                                    ))}
                                  </select>
                                  <span className="text-gray-500 font-medium">.</span>
                                  <select
                                    value={newRelationship.fromColumn}
                                    onChange={(e) => setNewRelationship(prev => ({ ...prev, fromColumn: e.target.value }))}
                                    className="flex-1 px-3 py-2 border border-gray-300 rounded-md text-sm min-w-0 bg-white shadow-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:bg-gray-100"
                                    disabled={!newRelationship.fromTable}
                                  >
                                    <option value="">Select Column</option>
                                    {newRelationship.fromTable && tableColumns[newRelationship.fromTable]?.map(column => (
                                      <option key={column} value={column}>{column}</option>
                                    ))}
                                  </select>
                                </div>

                                <div className="flex-1 flex items-center justify-center relative px-6">
                                  <div className="absolute left-4 right-4 flex items-center">
                                    <div className="flex-1 border-t-2 border-blue-300"></div>
                                    <div className="flex-1 border-t-2 border-blue-300"></div>
                                  </div>
                                  <div className="relative bg-white px-4 py-2 rounded-full border-2 border-blue-300 shadow-sm">
                                    <select
                                      value={newRelationship.type}
                                      onChange={(e) => setNewRelationship(prev => ({ ...prev, type: e.target.value as RelationshipType }))}
                                      className="text-sm font-medium border-none bg-transparent outline-none text-blue-700"
                                    >
                                      <option value="1:1">1:1</option>
                                      <option value="1:N">1:N</option>
                                      <option value="N:1">N:1</option>
                                    </select>
                                  </div>
                                </div>

                                <div className="flex-1 flex items-center justify-end gap-2">
                                  <select
                                    value={newRelationship.toTable}
                                    onChange={(e) => setNewRelationship(prev => ({ ...prev, toTable: e.target.value, toColumn: '' }))}
                                    className="flex-1 px-3 py-2 border border-gray-300 rounded-md text-sm min-w-0 bg-white shadow-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                                  >
                                    <option value="">Select Table</option>
                                    {selectedTables.map(table => (
                                      <option key={table} value={table}>{table}</option>
                                    ))}
                                  </select>
                                  <span className="text-gray-500 font-medium">.</span>
                                  <select
                                    value={newRelationship.toColumn}
                                    onChange={(e) => setNewRelationship(prev => ({ ...prev, toColumn: e.target.value }))}
                                    className="flex-1 px-3 py-2 border border-gray-300 rounded-md text-sm min-w-0 bg-white shadow-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:bg-gray-100"
                                    disabled={!newRelationship.toTable}
                                  >
                                    <option value="">Select Column</option>
                                    {newRelationship.toTable && tableColumns[newRelationship.toTable]?.map(column => (
                                      <option key={column} value={column}>{column}</option>
                                    ))}
                                  </select>
                                </div>

                                <div className="ml-4 flex gap-2">
                                  <Button
                                    size="sm"
                                    onClick={handleAddRelationship}
                                    disabled={!newRelationship.fromTable || !newRelationship.fromColumn || !newRelationship.toTable || !newRelationship.toColumn}
                                    className="bg-green-600 hover:bg-green-700 text-white px-4 py-2"
                                  >
                                    Add
                                  </Button>

                                  <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={handleCancelAddRelationship}
                                    className="border-gray-300 hover:bg-gray-50"
                                  >
                                    <X className="h-4 w-4" />
                                  </Button>
                                </div>
                              </div>
                            ) : (
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => setIsAddingRelationship(true)}
                                className="mt-2"
                              >
                                <Plus className="mr-2 h-3 w-3" />
                                Add Relationship
                              </Button>
                            )}
                          </>
                        ) : (
                          !isTaskCompleted && !isTaskRunning && (
                            <div className="text-sm text-text-tertiary italic">
                              This task will show relationships once it is run.
                            </div>
                          )
                        )}
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </CardContent>
      </Card>

      {/* Bottom Done button */}
      <div className="mt-6 flex justify-end">
        <Button
          onClick={() => navigate(`/projects/${pid}`)}
          disabled={!allTasksCompleted}
          className="bg-blue-600 hover:bg-blue-700 text-white font-medium disabled:opacity-50 disabled:cursor-not-allowed px-8 py-3"
        >
          Done
        </Button>
      </div>
    </div>
  );
};

export default DataAnalysisPage;
