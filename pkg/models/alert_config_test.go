package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultAlertConfig_AlertsEnabled(t *testing.T) {
	config := DefaultAlertConfig()
	assert.True(t, config.AlertsEnabled)
}

func TestDefaultAlertConfig_AllTypesPresent(t *testing.T) {
	config := DefaultAlertConfig()
	expectedTypes := []string{
		AlertTypeSQLInjection,
		AlertTypeUnusualQueryVolume,
		AlertTypeSensitiveTable,
		AlertTypeLargeDataExport,
		AlertTypeAfterHoursAccess,
		AlertTypeNewUserHighVolume,
		AlertTypeRepeatedErrors,
	}
	for _, at := range expectedTypes {
		_, ok := config.AlertSettings[at]
		assert.True(t, ok, "expected alert type %s in default config", at)
	}
}

func TestDefaultAlertConfig_AfterHoursDisabledByDefault(t *testing.T) {
	config := DefaultAlertConfig()
	setting := config.AlertSettings[AlertTypeAfterHoursAccess]
	assert.False(t, setting.Enabled)
}

func TestIsAlertEnabled_MasterToggleOff(t *testing.T) {
	config := DefaultAlertConfig()
	config.AlertsEnabled = false
	assert.False(t, config.IsAlertEnabled(AlertTypeSQLInjection))
}

func TestIsAlertEnabled_MasterToggleOn_AlertEnabled(t *testing.T) {
	config := DefaultAlertConfig()
	assert.True(t, config.IsAlertEnabled(AlertTypeSQLInjection))
}

func TestIsAlertEnabled_MasterToggleOn_AlertDisabled(t *testing.T) {
	config := DefaultAlertConfig()
	assert.False(t, config.IsAlertEnabled(AlertTypeAfterHoursAccess))
}

func TestIsAlertEnabled_UnknownAlertType(t *testing.T) {
	config := DefaultAlertConfig()
	assert.True(t, config.IsAlertEnabled("unknown_alert_type"))
}

func TestGetSeverity_ConfiguredSeverity(t *testing.T) {
	config := DefaultAlertConfig()
	assert.Equal(t, AlertSeverityCritical, config.GetSeverity(AlertTypeSQLInjection, AlertSeverityWarning))
}

func TestGetSeverity_UnknownType_UsesDefault(t *testing.T) {
	config := DefaultAlertConfig()
	assert.Equal(t, AlertSeverityWarning, config.GetSeverity("unknown_type", AlertSeverityWarning))
}

func TestGetSeverity_InvalidSeverity_UsesDefault(t *testing.T) {
	config := DefaultAlertConfig()
	config.AlertSettings[AlertTypeSQLInjection] = AlertTypeSetting{
		Enabled:  true,
		Severity: "invalid",
	}
	assert.Equal(t, AlertSeverityCritical, config.GetSeverity(AlertTypeSQLInjection, AlertSeverityCritical))
}
