import { Database, Tag, Layers, Key } from "lucide-react";
import { useState } from "react";

import type { EntitySummary } from "../../types/ontology";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../ui/Card";

interface TablesViewProps {
  entities?: EntitySummary[];
}

const TablesView = ({ entities }: TablesViewProps) => {
  const [expandedTable, setExpandedTable] = useState<string | null>(null);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Database className="h-5 w-5 text-purple-500" />
          <CardTitle>Entities Overview</CardTitle>
        </div>
        <CardDescription>
          Database entities with business terminology and semantic metadata
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {!entities || entities.length === 0 ? (
            <div className="text-sm text-text-tertiary italic">
              No entity data available yet. Start the ontology extraction
              workflow to analyze your database schema.
            </div>
          ) : (
            <div className="space-y-3">
              {entities.map((entity) => (
                <div
                  key={entity.table_name}
                  className="rounded-lg border border-border-light overflow-hidden"
                >
                  {/* Entity Header */}
                  <button
                    onClick={() =>
                      setExpandedTable(
                        expandedTable === entity.table_name ? null : entity.table_name
                      )
                    }
                    className="w-full p-4 text-left hover:bg-surface-secondary transition-colors"
                  >
                    <div className="flex items-start justify-between">
                      <div className="flex-1">
                        <div className="flex items-center gap-2 mb-1">
                          <span className="font-medium text-text-primary">
                            {entity.table_name}
                          </span>
                          {entity.domain && (
                            <span className="text-xs bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200 px-2 py-0.5 rounded">
                              {entity.domain}
                            </span>
                          )}
                        </div>

                        {entity.business_name && (
                          <div className="text-sm text-text-secondary mb-1">
                            <Tag className="h-3 w-3 inline mr-1" />
                            {entity.business_name}
                          </div>
                        )}

                        {entity.description && (
                          <p className="text-sm text-text-tertiary">
                            {entity.description}
                          </p>
                        )}

                        {entity.synonyms && entity.synonyms.length > 0 && (
                          <div className="text-xs text-text-tertiary mt-1">
                            Also known as:{" "}
                            <span className="text-text-secondary">
                              {entity.synonyms.join(", ")}
                            </span>
                          </div>
                        )}

                        {entity.relationships && entity.relationships.length > 0 && (
                          <div className="text-xs text-text-tertiary mt-1">
                            Related to:{" "}
                            <span className="text-text-secondary">
                              {entity.relationships.join(", ")}
                            </span>
                          </div>
                        )}
                      </div>

                      <div className="text-sm text-text-tertiary flex items-center gap-1">
                        <Layers className="h-4 w-4" />
                        {entity.column_count} columns
                      </div>
                    </div>
                  </button>

                  {/* Expanded Key Columns Details */}
                  {expandedTable === entity.table_name && entity.key_columns && entity.key_columns.length > 0 && (
                    <div className="border-t border-border-light bg-surface-secondary p-4">
                      <div className="text-xs text-text-tertiary mb-2 font-medium">Key Columns:</div>
                      <div className="grid gap-2">
                        {entity.key_columns.map((col) => (
                          <div
                            key={col.name}
                            className="flex items-start justify-between py-2 border-b border-border-light last:border-0"
                          >
                            <div className="flex-1">
                              <div className="flex items-center gap-2">
                                <Key className="h-3 w-3 text-amber-500" />
                                <span className="font-mono text-sm text-text-primary">
                                  {col.name}
                                </span>
                              </div>

                              {col.synonyms && col.synonyms.length > 0 && (
                                <div className="text-xs text-text-tertiary mt-1 ml-5">
                                  aka: {col.synonyms.join(", ")}
                                </div>
                              )}
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default TablesView;
