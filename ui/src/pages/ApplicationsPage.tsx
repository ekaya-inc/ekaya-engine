import type { LucideIcon } from 'lucide-react';
import {
  ArrowLeft,
  BrainCircuit,
  Check,
  ExternalLink,
  Loader2,
  MessageSquare,
  Package,
  Sparkles,
} from 'lucide-react';
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { Button } from '../components/ui/Button';
import {
  Card,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '../components/ui/Card';
import { useInstalledApps, useInstallApp } from '../hooks/useInstalledApps';
import { APP_ID_AI_DATA_LIAISON } from '../types';
import { cn } from '../utils/cn';

type AppColor = 'blue' | 'purple' | 'green' | 'gray';

interface ApplicationInfo {
  id: string;
  title: string;
  subtitle: string;
  icon: LucideIcon;
  color: AppColor;
  available: boolean;
  /** If true, this app can be installed (has install button) */
  installable?: boolean;
  /** URL for Learn More link */
  learnMoreUrl?: string;
}

const applications: ApplicationInfo[] = [
  {
    id: 'ai-data-liaison',
    title: 'AI Data Liaison',
    subtitle: 'Make Better Business Decisions 10x Faster',
    icon: BrainCircuit,
    color: 'blue',
    available: true,
    installable: true,
    learnMoreUrl: 'https://ekaya.ai/enterprise/',
  },
  {
    id: 'product-kit',
    title: 'Product Kit',
    subtitle: 'Enable AI Features in your existing SaaS Product',
    icon: Package,
    color: 'purple',
    available: true,
  },
  {
    id: 'on-premise-chat',
    title: 'On-Premise Chat',
    subtitle: 'Deploy AI Chat where data never leaves your data boundary',
    icon: MessageSquare,
    color: 'green',
    available: true,
  },
  {
    id: 'more-coming',
    title: 'More Coming!',
    subtitle: 'Additional applications in development',
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
  const { isInstalled, refetch } = useInstalledApps(pid);
  const { install, isLoading: isInstalling } = useInstallApp(pid);
  const [installingAppId, setInstallingAppId] = useState<string | null>(null);

  const handleContactSales = (app: ApplicationInfo) => {
    const subject = encodeURIComponent(
      `Interest in ${app.title} for my Ekaya project`,
    );
    const link = document.createElement('a');
    link.href = `mailto:sales@ekaya.ai?subject=${subject}`;
    link.click();
  };

  const handleInstall = async (appId: string) => {
    setInstallingAppId(appId);
    const result = await install(appId);
    setInstallingAppId(null);
    if (result) {
      await refetch();
      // Navigate to the app's configuration page
      navigate(`/projects/${pid}/${appId}`);
    }
  };

  const handleLearnMore = (url: string) => {
    window.open(url, '_blank', 'noopener,noreferrer');
  };

  const renderAppFooter = (app: ApplicationInfo) => {
    // Not available (Coming Soon)
    if (!app.available) {
      return (
        <CardFooter className="pt-0">
          <span className="text-xs text-text-secondary">Coming Soon</span>
        </CardFooter>
      );
    }

    // AI Data Liaison - special handling with Install/Installed state
    if (app.id === APP_ID_AI_DATA_LIAISON) {
      const appIsInstalled = isInstalled(app.id);
      const isCurrentlyInstalling = installingAppId === app.id && isInstalling;

      if (appIsInstalled) {
        // Installed state - show "Installed" badge and Configure button
        return (
          <CardFooter className="pt-0 flex gap-2">
            <span className="text-xs text-green-600 font-medium flex items-center gap-1">
              <Check className="h-3 w-3" />
              Installed
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => navigate(`/projects/${pid}/ai-data-liaison`)}
            >
              Configure
            </Button>
          </CardFooter>
        );
      }

      // Not installed - show Learn More and Install buttons
      const learnMoreUrl = app.learnMoreUrl;
      return (
        <CardFooter className="pt-0 flex gap-2">
          {learnMoreUrl ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleLearnMore(learnMoreUrl)}
            >
              <ExternalLink className="h-3 w-3 mr-1" />
              Learn More
            </Button>
          ) : null}
          <Button
            variant="default"
            size="sm"
            disabled={isCurrentlyInstalling}
            onClick={() => handleInstall(app.id)}
          >
            {isCurrentlyInstalling ? (
              <>
                <Loader2 className="h-3 w-3 mr-1 animate-spin" />
                Installing...
              </>
            ) : (
              'Install'
            )}
          </Button>
        </CardFooter>
      );
    }

    // Default: Contact Sales button for other available apps
    return (
      <CardFooter className="pt-0">
        <Button
          variant="outline"
          size="sm"
          className="w-full opacity-0 transition-opacity group-hover:opacity-100"
          onClick={() => handleContactSales(app)}
        >
          Contact Sales
        </Button>
      </CardFooter>
    );
  };

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
          <h1 className="text-2xl font-bold">Applications</h1>
          <p className="text-text-secondary">
            Choose an application to add to your project
          </p>
        </div>
      </div>

      {/* Application tiles - 2 column grid with smaller tiles */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {applications.map((app) => {
          const Icon = app.icon;
          const colors = getColorClasses(app.color);
          const appIsInstalled = app.id === APP_ID_AI_DATA_LIAISON && isInstalled(app.id);

          return (
            <Card
              key={app.id}
              data-testid={`app-card-${app.id}`}
              className={cn(
                'group relative transition-all',
                app.available
                  ? appIsInstalled
                    ? 'cursor-pointer hover:shadow-md'
                    : 'hover:shadow-md'
                  : 'cursor-not-allowed border-dashed opacity-60',
              )}
              onClick={
                appIsInstalled
                  ? () => navigate(`/projects/${pid}/${app.id}`)
                  : undefined
              }
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
              {renderAppFooter(app)}
            </Card>
          );
        })}
      </div>
    </div>
  );
};

export default ApplicationsPage;
