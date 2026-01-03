import { Settings, ArrowLeft, Moon, Sun, Monitor, LogOut } from "lucide-react";
import { useState } from "react";
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
  const [isSigningOut, setIsSigningOut] = useState(false);

  const handleSignOut = async () => {
    setIsSigningOut(true);
    try {
      const response = await fetch("/api/auth/logout", {
        method: "POST",
        credentials: "include",
      });
      const data = await response.json();
      if (data.success && data.redirect_url) {
        window.location.href = data.redirect_url;
      }
    } catch (error) {
      console.error("Sign out failed:", error);
      setIsSigningOut(false);
    }
  };

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

        {/* Account */}
        <Card>
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-red-500/10">
                <LogOut className="h-5 w-5 text-red-500" />
              </div>
              <div>
                <CardTitle>Account</CardTitle>
                <CardDescription>
                  Sign out of this project
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <Button
              variant="outline"
              onClick={handleSignOut}
              disabled={isSigningOut}
              className="text-red-600 hover:text-red-700 hover:bg-red-50"
            >
              {isSigningOut ? "Signing out..." : "Sign Out"}
            </Button>
          </CardContent>
        </Card>

      </div>
    </div>
  );
};

export default SettingsPage;
