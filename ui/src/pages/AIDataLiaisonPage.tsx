import { ArrowLeft, BrainCircuit } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';

import { Button } from '../components/ui/Button';
import { Card, CardDescription, CardHeader, CardTitle } from '../components/ui/Card';

const AIDataLiaisonPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header with back button */}
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
          <h1 className="text-2xl font-bold">AI Data Liaison</h1>
          <p className="text-text-secondary">
            Make Better Business Decisions 10x Faster
          </p>
        </div>
      </div>

      {/* Coming Soon Card */}
      <Card className="border-dashed">
        <CardHeader className="items-center text-center py-12">
          <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-xl bg-blue-500/10">
            <BrainCircuit className="h-8 w-8 text-blue-500" />
          </div>
          <CardTitle className="text-xl">Coming Soon</CardTitle>
          <CardDescription className="max-w-md">
            AI Data Liaison is currently in development. This application will
            provide AI-powered data analysis and insights for faster, smarter
            business decisions.
          </CardDescription>
        </CardHeader>
      </Card>
    </div>
  );
};

export default AIDataLiaisonPage;
