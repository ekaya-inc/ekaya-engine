import { ArrowLeft, ArrowRight, MessageCircleQuestion } from 'lucide-react';
import { useState, useEffect, useCallback } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { AIAnsweringGuide } from '../components/ontology/AIAnsweringGuide';
import { QuestionsList } from '../components/ontology/QuestionsList';
import { Button } from '../components/ui/Button';
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/Card';
import { fetchWithAuth } from '../lib/api';

/**
 * Question data from the backend
 */
interface Question {
  id: string;
  text: string;
  priority: number;
  is_required: boolean;
  category: string;
  reasoning?: string;
  affected_tables?: string[];
  affected_columns?: string[];
  status: string;
  created_at: string;
}

/**
 * Response from GET /api/projects/{pid}/ontology/questions
 */
interface QuestionsListResponse {
  questions: Question[];
  total: number;
}

/**
 * OntologyQuestionsPage - Display and manage ontology questions
 *
 * Shows questions generated during ontology extraction that require
 * human or AI input to improve ontology quality beyond what can be
 * inferred from schema alone.
 */
const OntologyQuestionsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

  // State
  const [questions, setQuestions] = useState<Question[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Fetch questions
  const fetchQuestions = useCallback(async (): Promise<void> => {
    if (!pid) return;

    try {
      setLoading(true);
      setError(null);

      const response = await fetchWithAuth(`/api/projects/${pid}/ontology/questions`);
      const json = await response.json() as { success: boolean; data?: QuestionsListResponse; error?: string };

      if (!response.ok || !json.success) {
        throw new Error(json.error ?? `HTTP ${response.status}`);
      }

      setQuestions(json.data?.questions ?? []);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to fetch questions';
      console.error('Failed to fetch questions:', errorMessage);
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  }, [pid]);

  // Fetch on mount
  useEffect(() => {
    fetchQuestions();
  }, [fetchQuestions]);

  // Loading state
  if (loading) {
    return (
      <div className="mx-auto max-w-6xl">
        <div className="mb-6">
          <Button variant="ghost" onClick={() => navigate(`/projects/${pid}`)}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="text-center">
            <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-current border-r-transparent motion-reduce:animate-[spin_1.5s_linear_infinite]" role="status">
              <span className="sr-only">Loading questions...</span>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">Loading ontology questions...</p>
          </div>
        </div>
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div className="mx-auto max-w-6xl">
        <div className="mb-6">
          <Button variant="ghost" onClick={() => navigate(`/projects/${pid}`)}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
        </div>
        <div className="flex items-center justify-center min-h-[400px]">
          <div className="text-center max-w-md p-6">
            <div className="mb-4 text-destructive">
              <svg xmlns="http://www.w3.org/2000/svg" className="h-12 w-12 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
            </div>
            <h2 className="text-lg font-semibold mb-2">Failed to Load Questions</h2>
            <p className="text-sm text-muted-foreground mb-4">
              {error}
            </p>
            <button
              onClick={() => fetchQuestions()}
              className="px-4 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90"
            >
              Retry
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Count questions by category for summary
  const categoryCounts = questions.reduce((acc, q) => {
    const cat = q.category || 'other';
    acc[cat] = (acc[cat] ?? 0) + 1;
    return acc;
  }, {} as Record<string, number>);
  const categoryCount = Object.keys(categoryCounts).length;

  return (
    <div className="mx-auto max-w-6xl">
      {/* Header */}
      <div className="mb-6">
        <div className="flex items-center justify-between mb-4">
          <Button variant="ghost" onClick={() => navigate(`/projects/${pid}`)}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Dashboard
          </Button>
          <Button variant="outline" onClick={() => navigate(`/projects/${pid}/ontology`)}>
            Go to Ontology
            <ArrowRight className="ml-2 h-4 w-4" />
          </Button>
        </div>
        <h1 className="text-3xl font-bold text-text-primary">
          Ontology Questions
        </h1>
        <p className="mt-2 text-text-secondary">
          Ontology Extraction is limited by what is in the data. The answers to these
          questions go beyond the contents of the data and are important to enabling
          your users to ask ad-hoc questions.
        </p>
      </div>

      {/* Summary Card */}
      <Card className="mb-6">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-amber-500/10">
              <MessageCircleQuestion className="h-5 w-5 text-amber-500" />
            </div>
            <div>
              <CardTitle>Summary</CardTitle>
              <CardDescription>
                {questions.length} pending {questions.length === 1 ? 'question' : 'questions'} across {categoryCount} {categoryCount === 1 ? 'category' : 'categories'}
              </CardDescription>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* Content */}
      <div className="space-y-6">
        {/* AI Answering Guide */}
        <AIAnsweringGuide questionCount={questions.length} />

        {/* Questions List */}
        <QuestionsList questions={questions} />
      </div>
    </div>
  );
};

export default OntologyQuestionsPage;
