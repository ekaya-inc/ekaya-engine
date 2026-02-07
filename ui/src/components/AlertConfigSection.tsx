import { Bell, Loader2, Save } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { Button } from "./ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "./ui/Card";
import { Input } from "./ui/Input";
import { Label } from "./ui/Label";
import { Switch } from "./ui/Switch";
import engineApi from "../services/engineApi";
import type { AlertConfig, AlertTypeSetting } from "../types/audit";

interface AlertConfigSectionProps {
  projectId: string;
}

const ALERT_TYPE_META: Record<
  string,
  { label: string; description: string; thresholdFields?: ThresholdField[] }
> = {
  sql_injection_detected: {
    label: "SQL Injection Detection",
    description: "Alerts when SQL injection patterns are detected in queries",
  },
  unusual_query_volume: {
    label: "Unusual Query Volume",
    description: "Alerts when a user exceeds the query volume threshold",
    thresholdFields: [
      {
        key: "threshold_multiplier",
        label: "Threshold Multiplier",
        type: "number",
        step: 0.5,
        min: 1,
        placeholder: "5",
      },
    ],
  },
  sensitive_table_access: {
    label: "Sensitive Table Access",
    description: "Alerts when queries touch tables flagged as sensitive",
  },
  large_data_export: {
    label: "Large Data Export",
    description: "Alerts when a query returns more rows than the threshold",
    thresholdFields: [
      {
        key: "row_threshold",
        label: "Row Threshold",
        type: "number",
        min: 100,
        placeholder: "10000",
      },
    ],
  },
  after_hours_access: {
    label: "After-Hours Access",
    description: "Alerts when the system is accessed outside business hours",
    thresholdFields: [
      {
        key: "business_hours_start",
        label: "Start Time",
        type: "text",
        placeholder: "06:00",
      },
      {
        key: "business_hours_end",
        label: "End Time",
        type: "text",
        placeholder: "22:00",
      },
      {
        key: "timezone",
        label: "Timezone",
        type: "text",
        placeholder: "UTC",
      },
    ],
  },
  new_user_high_volume: {
    label: "New User High Volume",
    description:
      "Alerts when a user runs many queries within their first 24 hours",
    thresholdFields: [
      {
        key: "query_threshold",
        label: "Query Threshold",
        type: "number",
        min: 1,
        placeholder: "20",
      },
    ],
  },
  repeated_errors: {
    label: "Repeated Errors",
    description:
      "Alerts when the same error occurs repeatedly from a single user",
    thresholdFields: [
      {
        key: "error_count",
        label: "Error Count",
        type: "number",
        min: 1,
        placeholder: "5",
      },
      {
        key: "window_minutes",
        label: "Window (minutes)",
        type: "number",
        min: 1,
        placeholder: "10",
      },
    ],
  },
};

interface ThresholdField {
  key: string;
  label: string;
  type: string;
  step?: number;
  min?: number;
  placeholder?: string;
}

const SEVERITY_OPTIONS = ["critical", "warning", "info"];

const AlertConfigSection = ({ projectId }: AlertConfigSectionProps) => {
  const [config, setConfig] = useState<AlertConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);

  const loadConfig = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await engineApi.getAlertConfig(projectId);
      if (response.success && response.data) {
        setConfig(response.data);
      }
    } catch (err) {
      console.error("Failed to load alert config:", err);
      setError("Failed to load alert configuration");
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    loadConfig();
  }, [loadConfig]);

  const handleSave = async () => {
    if (!config) return;
    setSaving(true);
    setError(null);
    setSaveSuccess(false);
    try {
      const response = await engineApi.updateAlertConfig(projectId, config);
      if (response.success && response.data) {
        setConfig(response.data);
        setSaveSuccess(true);
        setTimeout(() => setSaveSuccess(false), 3000);
      }
    } catch (err) {
      console.error("Failed to save alert config:", err);
      setError("Failed to save alert configuration");
    } finally {
      setSaving(false);
    }
  };

  const updateSetting = (
    alertType: string,
    field: string,
    value: unknown
  ) => {
    if (!config) return;
    const currentSetting: AlertTypeSetting = config.alert_settings[alertType] ?? {
      enabled: true,
      severity: "warning",
    };
    const updated: AlertTypeSetting = {
      ...currentSetting,
      [field]: value,
    };
    setConfig({
      ...config,
      alert_settings: {
        ...config.alert_settings,
        [alertType]: updated,
      },
    });
  };

  if (loading) {
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-purple/10">
              <Bell className="h-5 w-5 text-brand-purple" />
            </div>
            <div>
              <CardTitle>Alert Configuration</CardTitle>
              <CardDescription>Loading...</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-text-secondary" />
          </div>
        </CardContent>
      </Card>
    );
  }

  if (!config) {
    return (
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-purple/10">
              <Bell className="h-5 w-5 text-brand-purple" />
            </div>
            <div>
              <CardTitle>Alert Configuration</CardTitle>
              <CardDescription>
                {error ?? "Unable to load alert configuration"}
              </CardDescription>
            </div>
          </div>
        </CardHeader>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-purple/10">
              <Bell className="h-5 w-5 text-brand-purple" />
            </div>
            <div>
              <CardTitle>Alert Configuration</CardTitle>
              <CardDescription>
                Configure which security and governance alerts are active
              </CardDescription>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Label htmlFor="alerts-enabled" className="text-sm text-text-secondary">
              Alerts Enabled
            </Label>
            <Switch
              id="alerts-enabled"
              checked={config.alerts_enabled}
              onCheckedChange={(checked) =>
                setConfig({ ...config, alerts_enabled: checked })
              }
            />
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {!config.alerts_enabled && (
          <p className="mb-4 text-sm text-text-secondary">
            All alerts are disabled. Enable the master toggle above to activate alert monitoring.
          </p>
        )}

        <div className={`space-y-4 ${!config.alerts_enabled ? "opacity-50 pointer-events-none" : ""}`}>
          {Object.entries(ALERT_TYPE_META).map(([alertType, meta]) => {
            const setting: AlertTypeSetting = config.alert_settings[alertType] ?? {
              enabled: true,
              severity: "warning",
            };

            return (
              <div
                key={alertType}
                className="rounded-lg border border-border-light p-4"
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="flex-1">
                    <div className="flex items-center gap-3">
                      <Switch
                        checked={setting.enabled}
                        onCheckedChange={(checked) =>
                          updateSetting(alertType, "enabled", checked)
                        }
                      />
                      <div>
                        <p className="text-sm font-medium text-text-primary">
                          {meta.label}
                        </p>
                        <p className="text-xs text-text-secondary">
                          {meta.description}
                        </p>
                      </div>
                    </div>

                    {setting.enabled && (
                      <div className="mt-3 ml-14 flex flex-wrap gap-4">
                        <div>
                          <Label className="text-xs text-text-secondary">
                            Severity
                          </Label>
                          <select
                            value={setting.severity}
                            onChange={(e) =>
                              updateSetting(alertType, "severity", e.target.value)
                            }
                            className="mt-1 block text-xs px-2 py-1 rounded border border-border-light bg-surface-primary text-text-primary"
                          >
                            {SEVERITY_OPTIONS.map((sev) => (
                              <option key={sev} value={sev}>
                                {sev.charAt(0).toUpperCase() + sev.slice(1)}
                              </option>
                            ))}
                          </select>
                        </div>

                        {meta.thresholdFields?.map((field) => (
                          <div key={field.key}>
                            <Label className="text-xs text-text-secondary">
                              {field.label}
                            </Label>
                            <Input
                              type={field.type}
                              step={field.step}
                              min={field.min}
                              placeholder={field.placeholder}
                              value={
                                String(
                                  (setting as unknown as Record<string, unknown>)[
                                    field.key
                                  ] ?? ""
                                )
                              }
                              onChange={(e) => {
                                const val =
                                  field.type === "number"
                                    ? e.target.value === ""
                                      ? undefined
                                      : Number(e.target.value)
                                    : e.target.value || undefined;
                                updateSetting(alertType, field.key, val);
                              }}
                              className="mt-1 w-32 text-xs"
                            />
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>

        {error && (
          <p className="mt-4 text-sm text-red-600">{error}</p>
        )}

        <div className="mt-6 flex items-center gap-3">
          <Button onClick={handleSave} disabled={saving}>
            {saving ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              <>
                <Save className="mr-2 h-4 w-4" />
                Save Configuration
              </>
            )}
          </Button>
          {saveSuccess && (
            <span className="text-sm text-green-600">
              Configuration saved successfully
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default AlertConfigSection;
