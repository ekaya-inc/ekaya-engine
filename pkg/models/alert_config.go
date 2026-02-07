package models

// AlertConfig represents the per-project alert configuration stored in engine_mcp_config.alert_config.
type AlertConfig struct {
	AlertsEnabled bool                        `json:"alerts_enabled"`
	AlertSettings map[string]AlertTypeSetting `json:"alert_settings"`
}

// AlertTypeSetting contains the configuration for a single alert type.
type AlertTypeSetting struct {
	Enabled  bool   `json:"enabled"`
	Severity string `json:"severity"`

	// Type-specific thresholds (only relevant fields are set per alert type)
	ThresholdMultiplier *float64 `json:"threshold_multiplier,omitempty"` // unusual_query_volume
	RowThreshold        *int     `json:"row_threshold,omitempty"`        // large_data_export
	BusinessHoursStart  *string  `json:"business_hours_start,omitempty"` // after_hours_access
	BusinessHoursEnd    *string  `json:"business_hours_end,omitempty"`   // after_hours_access
	Timezone            *string  `json:"timezone,omitempty"`             // after_hours_access
	QueryThreshold      *int     `json:"query_threshold,omitempty"`      // new_user_high_volume
	ErrorCount          *int     `json:"error_count,omitempty"`          // repeated_errors
	WindowMinutes       *int     `json:"window_minutes,omitempty"`       // repeated_errors
}

// DefaultAlertConfig returns the default alert configuration matching the hardcoded thresholds
// in the alert trigger service.
func DefaultAlertConfig() *AlertConfig {
	five := float64(5)
	tenThousand := 10000
	twenty := 20
	errorCount := 5
	windowMin := 10

	return &AlertConfig{
		AlertsEnabled: true,
		AlertSettings: map[string]AlertTypeSetting{
			AlertTypeSQLInjection: {
				Enabled:  true,
				Severity: AlertSeverityCritical,
			},
			AlertTypeUnusualQueryVolume: {
				Enabled:             true,
				Severity:            AlertSeverityWarning,
				ThresholdMultiplier: &five,
			},
			AlertTypeSensitiveTable: {
				Enabled:  true,
				Severity: AlertSeverityWarning,
			},
			AlertTypeLargeDataExport: {
				Enabled:      true,
				Severity:     AlertSeverityInfo,
				RowThreshold: &tenThousand,
			},
			AlertTypeAfterHoursAccess: {
				Enabled:            false,
				Severity:           AlertSeverityInfo,
				BusinessHoursStart: strPtr("06:00"),
				BusinessHoursEnd:   strPtr("22:00"),
				Timezone:           strPtr("UTC"),
			},
			AlertTypeNewUserHighVolume: {
				Enabled:        true,
				Severity:       AlertSeverityWarning,
				QueryThreshold: &twenty,
			},
			AlertTypeRepeatedErrors: {
				Enabled:       true,
				Severity:      AlertSeverityWarning,
				ErrorCount:    &errorCount,
				WindowMinutes: &windowMin,
			},
		},
	}
}

func strPtr(s string) *string {
	return &s
}

// IsAlertEnabled checks if a specific alert type is enabled, considering the master toggle.
func (c *AlertConfig) IsAlertEnabled(alertType string) bool {
	if !c.AlertsEnabled {
		return false
	}
	setting, ok := c.AlertSettings[alertType]
	if !ok {
		// Unknown alert type: default to enabled
		return true
	}
	return setting.Enabled
}

// GetSeverity returns the configured severity for an alert type, falling back to the provided default.
func (c *AlertConfig) GetSeverity(alertType string, defaultSeverity string) string {
	setting, ok := c.AlertSettings[alertType]
	if !ok || !ValidAlertSeverity(setting.Severity) {
		return defaultSeverity
	}
	return setting.Severity
}
