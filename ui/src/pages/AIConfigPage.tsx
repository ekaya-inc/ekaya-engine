import { ArrowLeft } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';

import AIConfigWidget from '../components/AIConfigWidget';
import { Button } from '../components/ui/Button';

const AIConfigPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          aria-label="Back to project dashboard"
          onClick={() => navigate(`/projects/${pid}`)}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">AI Configuration</h1>
          <p className="text-text-secondary">
            Configure a Large Language Model to enable AI-powered features like ontology extraction, semantic search, and natural language querying.
          </p>
        </div>
      </div>

      <AIConfigWidget projectId={pid ?? ''} />
    </div>
  );
};

export default AIConfigPage;
