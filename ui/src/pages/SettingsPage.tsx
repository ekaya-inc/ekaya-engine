import {
  Settings,
  ArrowLeft,
  Moon,
  Sun,
  Monitor,
  User,
  Bell,
  Shield,
} from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";

import { useTheme } from "../components/ThemeProvider";
import { Button } from "../components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/Card";

const SettingsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const { theme, setTheme } = useTheme();

  return (
    <div className="mx-auto max-w-4xl">
      <div className="mb-6">
        <Button
          variant="ghost"
          onClick={() => navigate(`/projects/${pid}`)}
          className="mb-4"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Dashboard
        </Button>
        <h1 className="text-3xl font-bold text-text-primary">Settings</h1>
      </div>

      <div className="space-y-6">
        {/* Appearance Settings */}
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-gray-500/10">
                <Settings className="h-5 w-5 text-gray-500" />
              </div>
              <div>
                <CardTitle>Appearance</CardTitle>
                <CardDescription>
                  Customize how Ekaya Project UI looks
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-6">
              <div>
                <label className="mb-3 block text-sm font-medium text-text-primary">
                  Theme
                </label>
                <div className="grid gap-3 md:grid-cols-3">
                  <button
                    onClick={() => setTheme("light")}
                    className={`flex items-center justify-between rounded-lg border-2 p-3 transition-colors ${
                      theme === "light"
                        ? "border-blue-500 bg-blue-50 dark:bg-blue-950"
                        : "border-border-light hover:border-gray-400"
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      <Sun className="h-4 w-4" />
                      <span className="font-medium">Light</span>
                    </div>
                    {theme === "light" && (
                      <div className="h-2 w-2 rounded-full bg-blue-500" />
                    )}
                  </button>
                  <button
                    onClick={() => setTheme("dark")}
                    className={`flex items-center justify-between rounded-lg border-2 p-3 transition-colors ${
                      theme === "dark"
                        ? "border-blue-500 bg-blue-50 dark:bg-blue-950"
                        : "border-border-light hover:border-gray-400"
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      <Moon className="h-4 w-4" />
                      <span className="font-medium">Dark</span>
                    </div>
                    {theme === "dark" && (
                      <div className="h-2 w-2 rounded-full bg-blue-500" />
                    )}
                  </button>
                  <button
                    onClick={() => setTheme("system")}
                    className={`flex items-center justify-between rounded-lg border-2 p-3 transition-colors ${
                      theme === "system"
                        ? "border-blue-500 bg-blue-50 dark:bg-blue-950"
                        : "border-border-light hover:border-gray-400"
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      <Monitor className="h-4 w-4" />
                      <span className="font-medium">System</span>
                    </div>
                    {theme === "system" && (
                      <div className="h-2 w-2 rounded-full bg-blue-500" />
                    )}
                  </button>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* User Preferences */}
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-500/10">
                <User className="h-5 w-5 text-blue-500" />
              </div>
              <div>
                <CardTitle>User Preferences</CardTitle>
                <CardDescription>
                  Configure your personal settings
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <div className="font-medium text-text-primary">Language</div>
                  <div className="text-sm text-text-secondary">
                    Select your preferred language
                  </div>
                </div>
                <select className="rounded-md border border-border-light bg-surface-primary px-3 py-2 text-sm">
                  <option>English</option>
                  <option>Spanish</option>
                  <option>French</option>
                </select>
              </div>
              <div className="flex items-center justify-between">
                <div>
                  <div className="font-medium text-text-primary">Time Zone</div>
                  <div className="text-sm text-text-secondary">
                    Set your local time zone
                  </div>
                </div>
                <select className="rounded-md border border-border-light bg-surface-primary px-3 py-2 text-sm">
                  <option>UTC</option>
                  <option>EST</option>
                  <option>PST</option>
                </select>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Notifications */}
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-yellow-500/10">
                <Bell className="h-5 w-5 text-yellow-500" />
              </div>
              <div>
                <CardTitle>Notifications</CardTitle>
                <CardDescription>
                  Manage how you receive notifications
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <label className="flex items-center justify-between">
                <div>
                  <div className="font-medium text-text-primary">
                    Query Completion
                  </div>
                  <div className="text-sm text-text-secondary">
                    Notify when queries finish running
                  </div>
                </div>
                <input type="checkbox" className="rounded" defaultChecked />
              </label>
              <label className="flex items-center justify-between">
                <div>
                  <div className="font-medium text-text-primary">
                    Schema Changes
                  </div>
                  <div className="text-sm text-text-secondary">
                    Alert on database schema modifications
                  </div>
                </div>
                <input type="checkbox" className="rounded" defaultChecked />
              </label>
              <label className="flex items-center justify-between">
                <div>
                  <div className="font-medium text-text-primary">
                    System Updates
                  </div>
                  <div className="text-sm text-text-secondary">
                    Receive system update notifications
                  </div>
                </div>
                <input type="checkbox" className="rounded" />
              </label>
            </div>
          </CardContent>
        </Card>

        {/* Security */}
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-red-500/10">
                <Shield className="h-5 w-5 text-red-500" />
              </div>
              <div>
                <CardTitle>Security</CardTitle>
                <CardDescription>Manage your security settings</CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              <Button variant="outline" className="w-full justify-start">
                Change Password
              </Button>
              <Button variant="outline" className="w-full justify-start">
                Two-Factor Authentication
              </Button>
              <Button variant="outline" className="w-full justify-start">
                API Keys
              </Button>
              <Button
                variant="outline"
                className="w-full justify-start text-red-600 hover:text-red-700"
              >
                Clear All Sessions
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default SettingsPage;
