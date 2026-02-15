import { Settings, ArrowLeft, Moon, Sun, Monitor, LogOut, Trash2, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

// AlertConfigSection hidden until Alerts tile/screen is implemented.
// See plans/PLAN-alerts-tile-and-screen.md
// import AlertConfigSection from "../components/AlertConfigSection";
import { useTheme } from "../components/ThemeProvider";
import { Button } from "../components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../components/ui/Card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../components/ui/Dialog";
import { Input } from "../components/ui/Input";
import { useProject } from "../contexts/ProjectContext";
import { useToast } from "../hooks/useToast";
import engineApi from "../services/engineApi";

const SettingsPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const { theme, setTheme } = useTheme();
  const { projectName, urls } = useProject();
  const { toast } = useToast();
  const [isSigningOut, setIsSigningOut] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // Process callback from central redirect (after delete confirmation)
  useEffect(() => {
    const callbackAction = searchParams.get("callback_action");
    const callbackState = searchParams.get("callback_state");
    const callbackStatus = searchParams.get("callback_status") ?? "success";

    if (!callbackAction || !callbackState || !pid) return;
    if (callbackAction !== "delete") return;

    // Clear callback params from URL immediately to prevent re-processing
    setSearchParams({}, { replace: true });

    if (callbackStatus === "cancelled") {
      toast({ title: "Deletion cancelled", description: "Project was not deleted." });
      return;
    }

    const processCallback = async () => {
      try {
        const response = await engineApi.completeDeleteCallback(
          pid, "delete", callbackStatus, callbackState
        );
        if (response.error) {
          toast({ title: "Error", description: response.error, variant: "destructive" });
          return;
        }
        // Project deleted — redirect to central projects list
        const redirectUrl = response.data?.redirect_url ?? urls.projectsPageUrl;
        if (redirectUrl) {
          window.location.href = redirectUrl;
        } else {
          window.location.href = "/";
        }
      } catch (error) {
        toast({
          title: "Error",
          description: error instanceof Error ? error.message : "Failed to complete deletion",
          variant: "destructive",
        });
      }
    };

    processCallback();
  }, [searchParams, setSearchParams, pid, urls.projectsPageUrl, toast]);

  const handleDeleteProject = async () => {
    if (!pid || deleteConfirmation !== "delete project") return;

    setIsDeleting(true);
    setDeleteError(null);

    try {
      const result = await engineApi.deleteProject(pid);
      // If central requires a redirect (billing confirmation), follow it
      if (result && result.redirectUrl) {
        window.location.href = result.redirectUrl;
        return;
      }
      // No redirect — deleted directly, go to central projects list
      if (urls.projectsPageUrl) {
        window.location.href = urls.projectsPageUrl;
      } else {
        window.location.href = "/";
      }
    } catch (error) {
      console.error("Failed to delete project:", error);
      setDeleteError(error instanceof Error ? error.message : "Failed to delete project");
      setIsDeleting(false);
    }
  };

  const handleSignOut = async () => {
    setIsSigningOut(true);
    try {
      const response = await fetch(`/api/projects/${pid}/auth/logout`, {
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

        {/* Alert Configuration — hidden until Alerts tile/screen is implemented.
           See plans/PLAN-alerts-tile-and-screen.md */}
        {/* {pid && <AlertConfigSection projectId={pid} />} */}

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

        {/* Danger Zone */}
        <Card className="border-red-200 dark:border-red-900">
          <CardHeader>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-red-500/10">
                <Trash2 className="h-5 w-5 text-red-500" />
              </div>
              <div>
                <CardTitle className="text-red-600 dark:text-red-400">Danger Zone</CardTitle>
                <CardDescription>
                  Permanently delete this project from Ekaya Engine
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-text-secondary mb-4">
              This will permanently delete all project data from Ekaya Engine, including datasources,
              schema, ontology, and approved queries. This action cannot be undone.
            </p>
            <Button
              variant="outline"
              onClick={() => setShowDeleteDialog(true)}
              className="text-red-600 hover:text-red-700 hover:bg-red-50 border-red-300"
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Delete Project
            </Button>
          </CardContent>
        </Card>

      </div>

      {/* Delete Project Confirmation Dialog */}
      <Dialog open={showDeleteDialog} onOpenChange={(open) => {
        setShowDeleteDialog(open);
        if (!open) {
          setDeleteConfirmation("");
          setDeleteError(null);
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Project?</DialogTitle>
            <DialogDescription>
              This will permanently delete <strong>{projectName ?? "this project"}</strong> and all
              associated data from Ekaya Engine, including datasources, schema, ontology, and
              approved queries. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <label className="text-sm font-medium text-text-primary">
              Type <span className="font-mono bg-gray-100 dark:bg-gray-800 px-1 rounded">delete project</span> to confirm
            </label>
            <Input
              value={deleteConfirmation}
              onChange={(e) => setDeleteConfirmation(e.target.value)}
              placeholder="delete project"
              className="mt-2"
              disabled={isDeleting}
            />
            {deleteError && (
              <p className="mt-2 text-sm text-red-600">{deleteError}</p>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setShowDeleteDialog(false)}
              disabled={isDeleting}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteProject}
              disabled={deleteConfirmation !== "delete project" || isDeleting}
            >
              {isDeleting ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Deleting...
                </>
              ) : (
                "Delete Project"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default SettingsPage;
