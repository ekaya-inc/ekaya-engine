import type { LucideIcon } from "lucide-react";
import {
  HelpCircle,
  ArrowLeft,
  Book,
  MessageCircle,
  FileText,
  ExternalLink,
  ListTree,
  Zap,
  Shield,
  Database,
} from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";

import { Button } from "../components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/Card";

interface QuickStartItem {
  icon: LucideIcon;
  title: string;
  description: string;
  link: string;
}

interface Resource {
  title: string;
  description: string;
  link: string;
  icon: LucideIcon;
}

interface FAQ {
  question: string;
  answer: string;
}

const HelpPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const quickStartItems: QuickStartItem[] = [
    {
      icon: Database,
      title: "Connect Your Datasource",
      description: "Connect Ekaya to your PostgreSQL or SQL Server database",
      link: "datasource",
    },
    {
      icon: ListTree,
      title: "Extract Your Ontology",
      description: "Let AI analyze your schema and discover semantic meaning",
      link: "ontology-extraction",
    },
    {
      icon: Zap,
      title: "Configure MCP Server",
      description: "Enable AI tools to access your data through the MCP protocol",
      link: "mcp-server",
    },
    {
      icon: Shield,
      title: "Create Pre-Approved Queries",
      description: "Define safe, parameterized queries for AI and users",
      link: "queries",
    },
  ];

  const resources: Resource[] = [
    {
      title: "Documentation",
      description: "Comprehensive guides and API references",
      link: "#",
      icon: Book,
    },
    {
      title: "Community Forum",
      description: "Connect with other users and experts",
      link: "#",
      icon: MessageCircle,
    },
    {
      title: "Video Tutorials",
      description: "Step-by-step video walkthroughs",
      link: "#",
      icon: FileText,
    },
  ];

  const faqs: FAQ[] = [
    {
      question: "How do I connect to a PostgreSQL datasource?",
      answer:
        "Navigate to the Datasource page and enter your connection details including host, port, datasource name, username, and password.",
    },
    {
      question: "What is an ontology in this context?",
      answer:
        "An ontology represents the semantic model of your data, including tables, columns, relationships, and business rules that govern your data structure.",
    },
    {
      question: "Can I save and reuse queries?",
      answer:
        "Yes! In the Queries page, you can save frequently used queries and access them from the Saved Queries panel.",
    },
    {
      question: "How do I change the application theme?",
      answer:
        "Go to Settings > Appearance and select your preferred theme: Light, Dark, or System (which follows your OS preferences).",
    },
  ];

  return (
    <div className="mx-auto max-w-6xl">
      <div className="mb-6">
        <Button
          variant="ghost"
          onClick={() => navigate(`/projects/${pid}`)}
          className="mb-4"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Dashboard
        </Button>
        <h1 className="text-3xl font-bold text-text-primary">
          Help & Resources
        </h1>
        <p className="mt-2 text-text-secondary">
          Everything you need to get the most out of Ekaya Project UI
        </p>
      </div>

      {/* Quick Start Guide */}
      <Card className="mb-6">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-indigo-500/10">
              <Zap className="h-5 w-5 text-indigo-500" />
            </div>
            <div>
              <CardTitle>Quick Start Guide</CardTitle>
              <CardDescription>
                Get up and running quickly with these essential topics
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 md:grid-cols-2">
            {quickStartItems.map((item, i) => {
              const Icon = item.icon;
              return (
                <div
                  key={i}
                  onClick={() => navigate(`/projects/${pid}/${item.link}`)}
                  className="cursor-pointer rounded-lg border border-border-light p-4 hover:bg-surface-secondary hover:border-indigo-500/50 transition-colors flex gap-3"
                >
                  <Icon className="h-5 w-5 text-indigo-500 mt-0.5 shrink-0" />
                  <div>
                    <h3 className="font-medium text-text-primary">
                      {item.title}
                    </h3>
                    <p className="mt-1 text-sm text-text-secondary">
                      {item.description}
                    </p>
                  </div>
                </div>
              );
            })}
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-6 lg:grid-cols-2">
        {/* Resources */}
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-500/10">
                <Book className="h-5 w-5 text-blue-500" />
              </div>
              <CardTitle>Resources</CardTitle>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {resources.map((resource, i) => {
                const Icon = resource.icon;
                return (
                  <a
                    key={i}
                    href={resource.link}
                    className="flex items-center justify-between rounded-lg border border-border-light p-3 hover:bg-surface-secondary"
                  >
                    <div className="flex items-center gap-3">
                      <Icon className="h-4 w-4 text-text-secondary" />
                      <div>
                        <div className="font-medium text-text-primary">
                          {resource.title}
                        </div>
                        <div className="text-sm text-text-secondary">
                          {resource.description}
                        </div>
                      </div>
                    </div>
                    <ExternalLink className="h-4 w-4 text-text-secondary" />
                  </a>
                );
              })}
            </div>
          </CardContent>
        </Card>

        {/* Support */}
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-green-500/10">
                <MessageCircle className="h-5 w-5 text-green-500" />
              </div>
              <CardTitle>Get Support</CardTitle>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div>
                <h3 className="font-medium text-text-primary">
                  Contact Support
                </h3>
                <p className="mt-1 text-sm text-text-secondary">
                  Our support team is available Monday-Friday, 9AM-5PM EST
                </p>
                <a href="mailto:support@ekaya.com?subject=Support%20Request">
                  <Button className="mt-3" variant="outline">
                    <MessageCircle className="mr-2 h-4 w-4" />
                    Open Support Ticket
                  </Button>
                </a>
              </div>
              <div className="border-t border-border-light pt-4">
                <h3 className="font-medium text-text-primary">System Status</h3>
                <div className="mt-2 flex items-center gap-2">
                  <div className="h-2 w-2 rounded-full bg-green-500"></div>
                  <span className="text-sm text-text-secondary">
                    All systems operational
                  </span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* FAQs */}
      <Card className="mt-6">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-purple-500/10">
              <HelpCircle className="h-5 w-5 text-purple-500" />
            </div>
            <div>
              <CardTitle>Frequently Asked Questions</CardTitle>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {faqs.map((faq, i) => (
              <div
                key={i}
                className="border-b border-border-light pb-4 last:border-0"
              >
                <h3 className="font-medium text-text-primary">
                  {faq.question}
                </h3>
                <p className="mt-2 text-sm text-text-secondary">{faq.answer}</p>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Version Info */}
      <Card className="mt-6">
        <CardContent className="py-4">
          <div className="flex items-center justify-between text-sm">
            <span className="text-text-secondary">
              Ekaya Project UI Version
            </span>
            <span className="font-mono text-text-primary">v0.1.0</span>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

export default HelpPage;
