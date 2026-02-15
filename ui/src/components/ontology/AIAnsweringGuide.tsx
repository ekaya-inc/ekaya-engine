import { Bot, ExternalLink, Server } from 'lucide-react';
import { useParams, Link } from 'react-router-dom';

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../ui/Card';

interface AIAnsweringGuideProps {
  questionCount: number;
}

/**
 * AIAnsweringGuide - Explains how to use MCP Client to answer ontology questions
 *
 * Provides guidance on the recommended workflow for answering questions:
 * - Connect an AI assistant via MCP
 * - AI researches codebase and updates ontology
 * - Manual fallback for projects without accessible code
 */
export const AIAnsweringGuide = ({ questionCount }: AIAnsweringGuideProps) => {
  const { pid } = useParams<{ pid: string }>();

  if (questionCount === 0) {
    return null;
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-500/10 text-blue-500">
            <Bot className="h-5 w-5" />
          </div>
          <div>
            <CardTitle className="text-lg">Answering Questions with AI</CardTitle>
            <CardDescription>
              The fastest way to improve your ontology
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          The recommended way to answer these questions is using an AI assistant connected
          via MCP (Model Context Protocol). When connected to your codebase, AI can research
          each question and update the ontology automatically.
        </p>

        <div className="rounded-lg border bg-surface-secondary p-4 space-y-3">
          <h4 className="font-medium text-sm flex items-center gap-2">
            <Server className="h-4 w-4" />
            How it works
          </h4>
          <ol className="text-sm text-muted-foreground space-y-2 list-decimal list-inside">
            <li>
              Open your MCP Client (e.g., Claude Code) in the directory that has access to the
              source code operating on this datasource
            </li>
            <li>
              Connect it to this project&apos;s{' '}
              <Link
                to={`/projects/${pid}/mcp-server`}
                className="text-primary hover:underline inline-flex items-center gap-1"
              >
                MCP Server
                <ExternalLink className="h-3 w-3" />
              </Link>
            </li>
            <li>
              Ask it to &ldquo;answer all ontology questions&rdquo; &mdash; it will use the
              available tools to research each question and update the ontology automatically
            </li>
          </ol>
        </div>
      </CardContent>
    </Card>
  );
};
