package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockAlertTriggerRepo implements repositories.AlertTriggerRepository for testing.
type mockAlertTriggerRepo struct {
	eventCount       int
	errorCount       int
	firstEventTime   *time.Time
	hasOpenUserAlert bool
	hasOpenAlert     bool

	// Error simulation
	countEventsErr  error
	countErrorsErr  error
	firstTimeErr    error
	hasAlertUserErr error
	hasAlertErr     error
}

func (m *mockAlertTriggerRepo) CountUserEventsInWindow(_ context.Context, _ uuid.UUID, _ string, _ time.Time) (int, error) {
	if m.countEventsErr != nil {
		return 0, m.countEventsErr
	}
	return m.eventCount, nil
}

func (m *mockAlertTriggerRepo) CountUserErrorsWithMessage(_ context.Context, _ uuid.UUID, _ string, _ string, _ time.Time) (int, error) {
	if m.countErrorsErr != nil {
		return 0, m.countErrorsErr
	}
	return m.errorCount, nil
}

func (m *mockAlertTriggerRepo) GetUserFirstEventTime(_ context.Context, _ uuid.UUID, _ string) (*time.Time, error) {
	if m.firstTimeErr != nil {
		return nil, m.firstTimeErr
	}
	return m.firstEventTime, nil
}

func (m *mockAlertTriggerRepo) HasOpenAlertForUserAndType(_ context.Context, _ uuid.UUID, _ string, _ string, _ time.Time) (bool, error) {
	if m.hasAlertUserErr != nil {
		return false, m.hasAlertUserErr
	}
	return m.hasOpenUserAlert, nil
}

func (m *mockAlertTriggerRepo) HasOpenAlertForType(_ context.Context, _ uuid.UUID, _ string, _ time.Time) (bool, error) {
	if m.hasAlertErr != nil {
		return false, m.hasAlertErr
	}
	return m.hasOpenAlert, nil
}

func newTestTriggerService(alertRepo *mockAlertRepo, triggerRepo *mockAlertTriggerRepo) AlertTriggerService {
	return NewAlertTriggerService(alertRepo, triggerRepo, zap.NewNop())
}

func newBaseEvent() *models.MCPAuditEvent {
	toolName := "query"
	return &models.MCPAuditEvent{
		ID:            uuid.New(),
		ProjectID:     uuid.New(),
		UserID:        "user@example.com",
		EventType:     models.MCPEventToolCall,
		ToolName:      &toolName,
		WasSuccessful: true,
		SecurityLevel: models.MCPSecurityNormal,
	}
}

// findAlertByType returns the first alert with the given type, or nil if not found.
func findAlertByType(alerts []*models.AuditAlert, alertType string) *models.AuditAlert {
	for _, a := range alerts {
		if a.AlertType == alertType {
			return a
		}
	}
	return nil
}

// --- EvaluateEvent basic tests ---

func TestAlertTrigger_EvaluateEvent_NilEvent(t *testing.T) {
	svc := newTestTriggerService(&mockAlertRepo{}, &mockAlertTriggerRepo{})
	err := svc.EvaluateEvent(context.Background(), nil)
	assert.NoError(t, err)
}

func TestAlertTrigger_EvaluateEvent_NilProjectID(t *testing.T) {
	svc := newTestTriggerService(&mockAlertRepo{}, &mockAlertTriggerRepo{})
	event := &models.MCPAuditEvent{ProjectID: uuid.Nil}
	err := svc.EvaluateEvent(context.Background(), event)
	assert.NoError(t, err)
}

func TestAlertTrigger_EvaluateEvent_NormalEvent_NoSecurityAlerts(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{
		eventCount: 5,
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	// No security, volume, or data export alerts should fire (after_hours may fire depending on time)
	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeSQLInjection, a.AlertType)
		assert.NotEqual(t, models.AlertTypeUnusualQueryVolume, a.AlertType)
		assert.NotEqual(t, models.AlertTypeLargeDataExport, a.AlertType)
		assert.NotEqual(t, models.AlertTypeNewUserHighVolume, a.AlertType)
		assert.NotEqual(t, models.AlertTypeRepeatedErrors, a.AlertType)
	}
}

// --- SQL Injection trigger ---

func TestAlertTrigger_SQLInjection_SecurityFlagTriggersAlert(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.SecurityFlags = []string{"sql_injection_attempt"}
	event.SecurityLevel = models.MCPSecurityCritical

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	alert := findAlertByType(alertRepo.alerts, models.AlertTypeSQLInjection)
	require.NotNil(t, alert, "expected sql_injection_detected alert")
	assert.Equal(t, models.AlertSeverityCritical, alert.Severity)
	assert.Contains(t, alert.Title, "SQL injection")
}

func TestAlertTrigger_SQLInjection_CriticalSecurityLevel(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.SecurityLevel = models.MCPSecurityCritical

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	// Should trigger because security_level is critical
	var found bool
	for _, a := range alertRepo.alerts {
		if a.AlertType == models.AlertTypeSQLInjection {
			found = true
		}
	}
	assert.True(t, found, "expected sql_injection_detected alert")
}

func TestAlertTrigger_SQLInjection_IdempotencyPreventsDouble(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{
		hasOpenUserAlert: true,
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.SecurityFlags = []string{"sql_injection_attempt"}
	event.SecurityLevel = models.MCPSecurityCritical

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Empty(t, alertRepo.alerts, "should not create duplicate alert")
}

// --- Unusual Query Volume trigger ---

func TestAlertTrigger_UnusualQueryVolume_BelowThreshold(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{
		eventCount: 10, // below 50 threshold
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeUnusualQueryVolume, a.AlertType)
	}
}

func TestAlertTrigger_UnusualQueryVolume_AboveThreshold(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{
		eventCount: 55, // above 50 threshold
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	var found bool
	for _, a := range alertRepo.alerts {
		if a.AlertType == models.AlertTypeUnusualQueryVolume {
			found = true
			assert.Equal(t, models.AlertSeverityWarning, a.Severity)
			assert.Contains(t, a.Title, "55 queries")
		}
	}
	assert.True(t, found, "expected unusual_query_volume alert")
}

func TestAlertTrigger_UnusualQueryVolume_EmptyUserID(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{eventCount: 100}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.UserID = ""
	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeUnusualQueryVolume, a.AlertType)
	}
}

// --- Large Data Export trigger ---

func TestAlertTrigger_LargeDataExport_BelowThreshold(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.ResultSummary = map[string]any{"row_count": 500}

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeLargeDataExport, a.AlertType)
	}
}

func TestAlertTrigger_LargeDataExport_AboveThreshold(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.ResultSummary = map[string]any{"row_count": 15000}

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	var found bool
	for _, a := range alertRepo.alerts {
		if a.AlertType == models.AlertTypeLargeDataExport {
			found = true
			assert.Equal(t, models.AlertSeverityInfo, a.Severity)
			assert.Contains(t, a.Title, "15000 rows")
		}
	}
	assert.True(t, found, "expected large_data_export alert")
}

func TestAlertTrigger_LargeDataExport_Float64RowCount(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	// JSON unmarshaling produces float64 for numbers
	event.ResultSummary = map[string]any{"row_count": float64(20000)}

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	var found bool
	for _, a := range alertRepo.alerts {
		if a.AlertType == models.AlertTypeLargeDataExport {
			found = true
		}
	}
	assert.True(t, found, "expected large_data_export alert with float64 row_count")
}

func TestAlertTrigger_LargeDataExport_NoResultSummary(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.ResultSummary = nil

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeLargeDataExport, a.AlertType)
	}
}

// --- After Hours Access trigger ---

func TestAlertTrigger_AfterHoursAccess_DuringBusinessHours(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	// This test checks the current time, so we can only reliably test the function behavior
	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)
	// We can't deterministically test this without time injection, but we verify no error
}

// --- New User High Volume trigger ---

func TestAlertTrigger_NewUserHighVolume_OldUser(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	oldTime := time.Now().Add(-48 * time.Hour) // 2 days ago
	triggerRepo := &mockAlertTriggerRepo{
		firstEventTime: &oldTime,
		eventCount:     30,
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeNewUserHighVolume, a.AlertType)
	}
}

func TestAlertTrigger_NewUserHighVolume_NewUserBelowThreshold(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	recentTime := time.Now().Add(-2 * time.Hour) // 2 hours ago
	triggerRepo := &mockAlertTriggerRepo{
		firstEventTime: &recentTime,
		eventCount:     10, // below 20 threshold
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeNewUserHighVolume, a.AlertType)
	}
}

func TestAlertTrigger_NewUserHighVolume_NewUserAboveThreshold(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	recentTime := time.Now().Add(-2 * time.Hour) // 2 hours ago
	triggerRepo := &mockAlertTriggerRepo{
		firstEventTime: &recentTime,
		eventCount:     25, // above 20 threshold
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	var found bool
	for _, a := range alertRepo.alerts {
		if a.AlertType == models.AlertTypeNewUserHighVolume {
			found = true
			assert.Equal(t, models.AlertSeverityWarning, a.Severity)
		}
	}
	assert.True(t, found, "expected new_user_high_volume alert")
}

func TestAlertTrigger_NewUserHighVolume_NoEvents(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{
		firstEventTime: nil, // no events
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeNewUserHighVolume, a.AlertType)
	}
}

// --- Repeated Errors trigger ---

func TestAlertTrigger_RepeatedErrors_BelowThreshold(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{
		errorCount: 2, // below 5 threshold
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	errMsg := "connection refused"
	event.ErrorMessage = &errMsg
	event.WasSuccessful = false

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeRepeatedErrors, a.AlertType)
	}
}

func TestAlertTrigger_RepeatedErrors_AboveThreshold(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{
		errorCount: 7, // above 5 threshold
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	errMsg := "connection refused to database"
	event.ErrorMessage = &errMsg
	event.WasSuccessful = false

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	var found bool
	for _, a := range alertRepo.alerts {
		if a.AlertType == models.AlertTypeRepeatedErrors {
			found = true
			assert.Equal(t, models.AlertSeverityWarning, a.Severity)
			assert.Contains(t, a.Title, "7 occurrences")
		}
	}
	assert.True(t, found, "expected repeated_errors alert")
}

func TestAlertTrigger_RepeatedErrors_NoErrorMessage(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{errorCount: 10}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.ErrorMessage = nil

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeRepeatedErrors, a.AlertType)
	}
}

func TestAlertTrigger_RepeatedErrors_EmptyErrorMessage(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{errorCount: 10}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	emptyMsg := ""
	event.ErrorMessage = &emptyMsg

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	for _, a := range alertRepo.alerts {
		assert.NotEqual(t, models.AlertTypeRepeatedErrors, a.AlertType)
	}
}

// --- Alert creation details ---

func TestAlertTrigger_AlertHasRelatedAuditIDs(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.SecurityFlags = []string{"sql_injection_attempt"}
	event.SecurityLevel = models.MCPSecurityCritical

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	alert := findAlertByType(alertRepo.alerts, models.AlertTypeSQLInjection)
	require.NotNil(t, alert)
	assert.Contains(t, alert.RelatedAuditIDs, event.ID)
}

func TestAlertTrigger_AlertHasAffectedUserID(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.SecurityFlags = []string{"sql_injection_attempt"}
	event.SecurityLevel = models.MCPSecurityCritical

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	alert := findAlertByType(alertRepo.alerts, models.AlertTypeSQLInjection)
	require.NotNil(t, alert)
	require.NotNil(t, alert.AffectedUserID)
	assert.Equal(t, event.UserID, *alert.AffectedUserID)
}

func TestAlertTrigger_AlertHasCorrectProjectID(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	triggerRepo := &mockAlertTriggerRepo{}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.SecurityFlags = []string{"sql_injection_attempt"}
	event.SecurityLevel = models.MCPSecurityCritical

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	alert := findAlertByType(alertRepo.alerts, models.AlertTypeSQLInjection)
	require.NotNil(t, alert)
	assert.Equal(t, event.ProjectID, alert.ProjectID)
}

// --- Helper function tests ---

func TestExtractRowCountFromSummary_Int(t *testing.T) {
	summary := map[string]any{"row_count": 100}
	assert.Equal(t, 100, extractRowCountFromSummary(summary))
}

func TestExtractRowCountFromSummary_Float64(t *testing.T) {
	summary := map[string]any{"row_count": float64(250)}
	assert.Equal(t, 250, extractRowCountFromSummary(summary))
}

func TestExtractRowCountFromSummary_Int64(t *testing.T) {
	summary := map[string]any{"row_count": int64(999)}
	assert.Equal(t, 999, extractRowCountFromSummary(summary))
}

func TestExtractRowCountFromSummary_Nil(t *testing.T) {
	assert.Equal(t, 0, extractRowCountFromSummary(nil))
}

func TestExtractRowCountFromSummary_Missing(t *testing.T) {
	summary := map[string]any{"other": "value"}
	assert.Equal(t, 0, extractRowCountFromSummary(summary))
}

func TestTruncateString_Short(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
}

func TestTruncateString_Long(t *testing.T) {
	assert.Equal(t, "hel...", truncate("hello world", 3))
}

func TestHasInjectionFlag_Positive(t *testing.T) {
	event := &models.MCPAuditEvent{
		SecurityFlags: []string{"sql_injection_attempt"},
	}
	assert.True(t, hasInjectionFlag(event))
}

func TestHasInjectionFlag_Negative(t *testing.T) {
	event := &models.MCPAuditEvent{
		SecurityFlags: []string{"auth_failure"},
	}
	assert.False(t, hasInjectionFlag(event))
}

func TestHasInjectionFlag_Empty(t *testing.T) {
	event := &models.MCPAuditEvent{}
	assert.False(t, hasInjectionFlag(event))
}

func TestDescribeSecurityEvent(t *testing.T) {
	toolName := "query"
	errMsg := "suspicious input detected"
	event := &models.MCPAuditEvent{
		UserID:        "user@test.com",
		ToolName:      &toolName,
		SecurityFlags: []string{"injection", "suspicious"},
		ErrorMessage:  &errMsg,
	}

	desc := describeSecurityEvent(event)
	assert.Contains(t, desc, "user@test.com")
	assert.Contains(t, desc, "query")
	assert.Contains(t, desc, "injection, suspicious")
	assert.Contains(t, desc, "suspicious input detected")
}

// --- Multiple triggers fire independently ---

func TestAlertTrigger_MultipleTriggersFire(t *testing.T) {
	alertRepo := &mockAlertRepo{}
	recentTime := time.Now().Add(-1 * time.Hour)
	triggerRepo := &mockAlertTriggerRepo{
		eventCount:     55, // triggers unusual_query_volume
		firstEventTime: &recentTime,
	}
	svc := newTestTriggerService(alertRepo, triggerRepo)

	event := newBaseEvent()
	event.SecurityFlags = []string{"sql_injection_attempt"}
	event.SecurityLevel = models.MCPSecurityCritical
	event.ResultSummary = map[string]any{"row_count": 20000}

	err := svc.EvaluateEvent(context.Background(), event)
	require.NoError(t, err)

	// Should have alerts for: sql_injection, unusual_query_volume, large_data_export, new_user_high_volume
	alertTypes := make(map[string]bool)
	for _, a := range alertRepo.alerts {
		alertTypes[a.AlertType] = true
	}
	assert.True(t, alertTypes[models.AlertTypeSQLInjection], "expected sql_injection alert")
	assert.True(t, alertTypes[models.AlertTypeUnusualQueryVolume], "expected unusual_query_volume alert")
	assert.True(t, alertTypes[models.AlertTypeLargeDataExport], "expected large_data_export alert")
	assert.True(t, alertTypes[models.AlertTypeNewUserHighVolume], "expected new_user_high_volume alert")
}
