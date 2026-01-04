import type { LucideIcon } from 'lucide-react';
import {
  ArrowLeft,
  BrainCircuit,
  MessageSquare,
  Package,
  Sparkles,
} from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';

import { Button } from '../components/ui/Button';
import {
  Card,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '../components/ui/Card';
import { cn } from '../utils/cn';

type AppColor = 'blue' | 'purple' | 'green' | 'gray';

interface ApplicationInfo {
  id: string;
  title: string;
  subtitle: string;
  description: string;
  icon: LucideIcon;
  color: AppColor;
  available: boolean;
}

const applications: ApplicationInfo[] = [
  {
    id: 'ai-data-liaison',
    title: 'AI Data Liaison',
    subtitle: 'Make Better Business Decisions 10x Faster',
    description:
      'AI-powered data analysis and insights for faster, smarter business decisions.',
    icon: BrainCircuit,
    color: 'blue',
    available: true,
  },
  {
    id: 'product-kit',
    title: 'Product Kit',
    subtitle: 'Enable AI Features in your existing SaaS Product',
    description:
      'Integrate AI capabilities directly into your product with pre-built components and APIs.',
    icon: Package,
    color: 'purple',
    available: true,
  },
  {
    id: 'on-premise-chat',
    title: 'On-Premise Chat',
    subtitle: 'Deploy AI Chat where data never leaves your data boundary',
    description:
      'Self-hosted chat solution for maximum data privacy and compliance.',
    icon: MessageSquare,
    color: 'green',
    available: true,
  },
  {
    id: 'more-coming',
    title: 'More Coming!',
    subtitle: 'Additional applications in development',
    description:
      'We are building more applications to help you leverage your data.',
    icon: Sparkles,
    color: 'gray',
    available: false,
  },
];

const getColorClasses = (color: AppColor): { bg: string; text: string } => {
  const colorMap: Record<AppColor, { bg: string; text: string }> = {
    blue: { bg: 'bg-blue-500/10', text: 'text-blue-500' },
    purple: { bg: 'bg-purple-500/10', text: 'text-purple-500' },
    green: { bg: 'bg-green-500/10', text: 'text-green-500' },
    gray: { bg: 'bg-gray-500/10', text: 'text-gray-500' },
  };
  return colorMap[color];
};

const ApplicationsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();

  const handleInstall = (appId: string) => {
    // MVP: Navigate to app-specific page
    if (appId === 'ai-data-liaison') {
      navigate(`/projects/${pid}/ai-data-liaison`);
    } else if (appId === 'product-kit') {
      navigate(`/projects/${pid}/product-kit`);
    } else if (appId === 'on-premise-chat') {
      navigate(`/projects/${pid}/on-premise-chat`);
    }
  };

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      {/* Header with back button */}
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          onClick={() => navigate(`/projects/${pid}`)}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">Install Application</h1>
          <p className="text-text-secondary">
            Choose an application to add to your project
          </p>
        </div>
      </div>

      {/* Application tiles - 3 column grid with smaller tiles */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {applications.map((app) => {
          const Icon = app.icon;
          const colors = getColorClasses(app.color);

          return (
            <Card
              key={app.id}
              className={cn(
                'transition-all',
                app.available
                  ? 'cursor-pointer hover:shadow-md'
                  : 'cursor-not-allowed opacity-60',
              )}
              onClick={() => app.available && handleInstall(app.id)}
            >
              <CardHeader className="pb-2">
                <div
                  className={cn(
                    'mb-2 flex h-12 w-12 items-center justify-center rounded-lg',
                    colors.bg,
                  )}
                >
                  <Icon className={cn('h-6 w-6', colors.text)} />
                </div>
                <CardTitle className="text-base">{app.title}</CardTitle>
                <CardDescription className="line-clamp-2 text-xs">
                  {app.subtitle}
                </CardDescription>
              </CardHeader>
              {!app.available && (
                <CardFooter className="pt-0">
                  <span className="text-xs text-text-secondary">
                    Coming Soon
                  </span>
                </CardFooter>
              )}
            </Card>
          );
        })}
      </div>
    </div>
  );
};

export default ApplicationsPage;
