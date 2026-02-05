import { GitBranch } from 'lucide-react';

import type { RelationshipEdge } from '../../types/ontology';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../ui/Card';

interface RelationshipsViewProps {
  relationships?: RelationshipEdge[];
}

const RelationshipsView = ({ relationships }: RelationshipsViewProps) => {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <GitBranch className="h-5 w-5 text-purple-500" />
          <CardTitle>Relationships Overview</CardTitle>
        </div>
        <CardDescription>
          Table relationships discovered in your database
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {!relationships || relationships.length === 0 ? (
            <div className="text-sm text-text-tertiary italic">
              No relationship data available yet. Start the ontology extraction workflow to discover table relationships.
            </div>
          ) : (
            <div className="text-sm text-text-secondary">
              {relationships.map((rel, index) => (
                <div key={index} className="mb-2">
                  • <strong>{rel.from} → {rel.to}</strong> ({rel.type})
                </div>
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default RelationshipsView;
