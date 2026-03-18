import { Brain } from 'lucide-react';
import { useParams } from 'react-router-dom';

import AppPageHeader from '../components/AppPageHeader';
import AIConfigWidget from '../components/AIConfigWidget';

const AIConfigPage = () => {
  const { pid } = useParams<{ pid: string }>();

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <AppPageHeader
        title="AI Configuration"
        slug="ai-configuration"
        icon={<Brain className="h-8 w-8 text-purple-500" />}
        description="Configure a Large Language Model to enable AI-powered features like ontology extraction, semantic search, and natural language querying."
        showInfoLink={false}
      />

      <AIConfigWidget projectId={pid ?? ''} />
    </div>
  );
};

export default AIConfigPage;
