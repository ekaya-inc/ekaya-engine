import type { LucideIcon } from 'lucide-react';
import {
  ArrowLeft,
  Bot,
  BrainCircuit,
  Check,
  ExternalLink,
  Hammer,
  Loader2,
  MessageSquare,
  Package,
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
import { useProject } from '../contexts/ProjectContext';
import { useInstalledApps, useInstallApp } from '../hooks/useInstalledApps';
import { cn } from '../utils/cn';

/** Map ekaya-central origins to marketing site origins */
const marketingOriginMap: Record<string, string> = {
  'http://localhost:5002': 'http://localhost:3030',
  'http://localhost:3040': 'http://localhost:3030',
  'https://us.dev.ekaya.ai': 'https://dev.ekaya.ai',
  'https://us.ekaya.ai': 'https://ekaya.ai',
};

function getMarketingOrigin(projectsPageUrl: string | null): string {
  if (projectsPageUrl) {
    try {
      const origin = new URL(projectsPageUrl).origin;
      const mapped = marketingOriginMap[origin];
      if (mapped) return mapped;
    } catch {
      // invalid URL, fall through to default
    }
  }
  return 'https://ekaya.ai';
}

type AppColor = 'blue' | 'purple' | 'green' | 'gray' | 'orange';

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
  /** If true, show a Contact Support button instead of install/sales actions */
  contactSupport?: boolean;
}

const applications: ApplicationInfo[] = [
  {
    id: 'ai-data-liaison',
    title: 'AI Data Liaison',
    subtitle: 'Make Better Business Decisions 10x Faster and lower the burden on the data team',
    icon: BrainCircuit,
    color: 'blue',
    available: true,
    installable: true,
    learnMoreUrl: '/enterprise/',
  },
  {
    id: 'ai-agents',
    title: 'AI Agents and Automation',
    subtitle: 'Connect AI coding agents and automation tools to your data via API key authentication',
    icon: Bot,
    color: 'orange',
    available: true,
    installable: true,
    learnMoreUrl: '/ai-agents/',
  },
  {
    id: 'product-kit',
    title: 'Product Kit [BETA]',
    subtitle: 'Enable AI Features in your existing SaaS Product',
    icon: Package,
    color: 'purple',
    available: true,
  },
  {
    id: 'on-premise-chat',
    title: 'On-Premise Chat [BETA]',
    subtitle: 'Deploy AI Chat where data never leaves your data boundary',
    icon: MessageSquare,
    color: 'green',
    available: true,
  },
  {
    id: 'build-your-own',
    title: 'Your own Data Application',
    subtitle: 'Need custom functionality or want to sell your own solution? Ekaya is the platform for connecting AI to Data.',
    icon: Hammer,
    color: 'purple',
    available: true,
    contactSupport: true,
  },
];

const getColorClasses = (color: AppColor): { bg: string; text: string } => {
  const colorMap: Record<AppColor, { bg: string; text: string }> = {
    blue: { bg: 'bg-blue-500/10', text: 'text-blue-500' },
    purple: { bg: 'bg-purple-500/10', text: 'text-purple-500' },
    green: { bg: 'bg-green-500/10', text: 'text-green-500' },
    gray: { bg: 'bg-gray-500/10', text: 'text-gray-500' },
    orange: { bg: 'bg-orange-500/10', text: 'text-orange-500' },
  };
  return colorMap[color];
};

const ApplicationsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { urls } = useProject();
  const { isInstalled, refetch } = useInstalledApps(pid);
  const { install, isLoading: isInstalling } = useInstallApp(pid);
  const [installingAppId, setInstallingAppId] = useState<string | null>(null);
  const marketingOrigin = getMarketingOrigin(urls.projectsPageUrl);

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
      // Return to project dashboard so the user sees the app tile appear
      navigate(`/projects/${pid}`);
    }
  };

  const handleLearnMore = (path: string) => {
    window.open(`${marketingOrigin}${path}`, '_blank', 'noopener,noreferrer');
  };

  const handleContactSupport = () => {
    const subject = encodeURIComponent(
      'Interest in building a custom data application on Ekaya',
    );
    const link = document.createElement('a');
    link.href = `mailto:support@ekaya.ai?subject=${subject}`;
    link.click();
  };

  const renderAppFooter = (app: ApplicationInfo) => {
    // Not available (no actions to show)
    if (!app.available) {
      return null;
    }

    // Contact Support tile - same style as Contact Sales (full-width, show on hover)
    if (app.contactSupport) {
      return (
        <CardFooter className="pt-0">
          <Button
            variant="outline"
            size="sm"
            className="w-full opacity-0 transition-opacity group-hover:opacity-100"
            onClick={handleContactSupport}
          >
            Contact Support
          </Button>
        </CardFooter>
      );
    }

    // Installable apps - show Install/Installed state
    if (app.installable) {
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
        {applications.filter(app => app.id !== 'ai-agents').map((app) => {
          const Icon = app.icon;
          const colors = getColorClasses(app.color);
          const appIsInstalled = app.installable === true && isInstalled(app.id);

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
