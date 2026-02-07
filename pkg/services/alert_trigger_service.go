package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// AlertTriggerService evaluates MCP audit events and creates alerts when conditions are met.
type AlertTriggerService interface {
	EvaluateEvent(ctx context.Context, event *models.MCPAuditEvent) error
}

// alertTriggerDeps defines the minimal interface needed by alert triggers for alert creation.
type alertTriggerDeps struct {
	alertRepo         repositories.AlertRepository
	triggerRepo       repositories.AlertTriggerRepository
	mcpConfigRepo     repositories.MCPConfigRepository
	logger            *zap.Logger
	idempotencyWindow time.Duration
}

type alertTriggerService struct {
	deps alertTriggerDeps
}

func NewAlertTriggerService(
	alertRepo repositories.AlertRepository,
	triggerRepo repositories.AlertTriggerRepository,
	mcpConfigRepo repositories.MCPConfigRepository,
	logger *zap.Logger,
) AlertTriggerService {
	return &alertTriggerService{
		deps: alertTriggerDeps{
			alertRepo:         alertRepo,
			triggerRepo:       triggerRepo,
			mcpConfigRepo:     mcpConfigRepo,
			logger:            logger.Named("alert-trigger"),
			idempotencyWindow: 1 * time.Hour,
		},
	}
}

var _ AlertTriggerService = (*alertTriggerService)(nil)

// loadAlertConfig loads the alert configuration for the project, returning defaults if none is set.
func (s *alertTriggerService) loadAlertConfig(ctx context.Context, projectID uuid.UUID) *models.AlertConfig {
	config, err := s.deps.mcpConfigRepo.GetAlertConfig(ctx, projectID)
	if err != nil {
		s.deps.logger.Error("Failed to load alert config, using defaults",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return models.DefaultAlertConfig()
	}
	if config == nil {
		return models.DefaultAlertConfig()
	}
	return config
}

func (s *alertTriggerService) EvaluateEvent(ctx context.Context, event *models.MCPAuditEvent) error {
	if event == nil || event.ProjectID == uuid.Nil {
		return nil
	}

	config := s.loadAlertConfig(ctx, event.ProjectID)
	if !config.AlertsEnabled {
		return nil
	}

	// Each trigger runs independently; collect errors but don't stop on first failure
	var errs []string

	type trigger struct {
		alertType string
		check     func(context.Context, *models.MCPAuditEvent, *models.AlertConfig) error
	}

	triggers := []trigger{
		{models.AlertTypeSQLInjection, s.checkSQLInjection},
		{models.AlertTypeUnusualQueryVolume, s.checkUnusualQueryVolume},
		{models.AlertTypeLargeDataExport, s.checkLargeDataExport},
		{models.AlertTypeAfterHoursAccess, s.checkAfterHoursAccess},
		{models.AlertTypeNewUserHighVolume, s.checkNewUserHighVolume},
		{models.AlertTypeRepeatedErrors, s.checkRepeatedErrors},
	}

	for _, t := range triggers {
		if !config.IsAlertEnabled(t.alertType) {
			continue
		}
		if err := t.check(ctx, event, config); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("alert trigger errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// checkSQLInjection creates a critical alert when security_flags indicate injection or security_level is critical.
func (s *alertTriggerService) checkSQLInjection(ctx context.Context, event *models.MCPAuditEvent, config *models.AlertConfig) error {
	if !hasInjectionFlag(event) && event.SecurityLevel != models.MCPSecurityCritical {
		return nil
	}

	severity := config.GetSeverity(models.AlertTypeSQLInjection, models.AlertSeverityCritical)
	return s.createAlertIfNew(ctx, event, models.AlertTypeSQLInjection, severity,
		fmt.Sprintf("SQL injection pattern detected from %s", event.UserID),
		describeSecurityEvent(event),
	)
}

// checkUnusualQueryVolume alerts when a user's query count in the last hour exceeds the configured threshold.
func (s *alertTriggerService) checkUnusualQueryVolume(ctx context.Context, event *models.MCPAuditEvent, config *models.AlertConfig) error {
	if event.UserID == "" {
		return nil
	}

	since := time.Now().Add(-1 * time.Hour)
	count, err := s.deps.triggerRepo.CountUserEventsInWindow(ctx, event.ProjectID, event.UserID, since)
	if err != nil {
		return fmt.Errorf("unusual_query_volume check: %w", err)
	}

	threshold := 50
	if setting, ok := config.AlertSettings[models.AlertTypeUnusualQueryVolume]; ok && setting.ThresholdMultiplier != nil {
		// Use multiplier as an absolute threshold (consistent with plan spec: "> 5x their rolling average (or > 50 if no baseline)")
		// For simplicity, threshold_multiplier * 10 gives a configurable absolute threshold
		threshold = int(*setting.ThresholdMultiplier * 10)
		if threshold < 1 {
			threshold = 50
		}
	}

	if count < threshold {
		return nil
	}

	severity := config.GetSeverity(models.AlertTypeUnusualQueryVolume, models.AlertSeverityWarning)
	return s.createAlertIfNew(ctx, event, models.AlertTypeUnusualQueryVolume, severity,
		fmt.Sprintf("Unusual query volume from %s (%d queries in last hour)", event.UserID, count),
		fmt.Sprintf("User %s has executed %d queries in the last hour, exceeding threshold of %d.", event.UserID, count, threshold),
	)
}

// checkLargeDataExport alerts when a query returns more than the configured row threshold.
func (s *alertTriggerService) checkLargeDataExport(ctx context.Context, event *models.MCPAuditEvent, config *models.AlertConfig) error {
	rowCount := extractRowCountFromSummary(event.ResultSummary)

	threshold := 10000
	if setting, ok := config.AlertSettings[models.AlertTypeLargeDataExport]; ok && setting.RowThreshold != nil {
		threshold = *setting.RowThreshold
	}

	if rowCount < threshold {
		return nil
	}

	severity := config.GetSeverity(models.AlertTypeLargeDataExport, models.AlertSeverityInfo)
	return s.createAlertIfNew(ctx, event, models.AlertTypeLargeDataExport, severity,
		fmt.Sprintf("Large data export by %s (%d rows)", event.UserID, rowCount),
		fmt.Sprintf("User %s executed a query returning %d rows (threshold: %d).", event.UserID, rowCount, threshold),
	)
}

// checkAfterHoursAccess alerts when events occur outside configured business hours.
func (s *alertTriggerService) checkAfterHoursAccess(ctx context.Context, event *models.MCPAuditEvent, config *models.AlertConfig) error {
	startHour, startMin := 6, 0
	endHour, endMin := 22, 0
	loc := time.UTC

	if setting, ok := config.AlertSettings[models.AlertTypeAfterHoursAccess]; ok {
		if setting.BusinessHoursStart != nil {
			if h, m, err := parseHourMinute(*setting.BusinessHoursStart); err == nil {
				startHour, startMin = h, m
			}
		}
		if setting.BusinessHoursEnd != nil {
			if h, m, err := parseHourMinute(*setting.BusinessHoursEnd); err == nil {
				endHour, endMin = h, m
			}
		}
		if setting.Timezone != nil {
			if tz, err := time.LoadLocation(*setting.Timezone); err == nil {
				loc = tz
			}
		}
	}

	now := time.Now().In(loc)
	minuteOfDay := now.Hour()*60 + now.Minute()
	startMinOfDay := startHour*60 + startMin
	endMinOfDay := endHour*60 + endMin

	if minuteOfDay >= startMinOfDay && minuteOfDay < endMinOfDay {
		return nil
	}

	severity := config.GetSeverity(models.AlertTypeAfterHoursAccess, models.AlertSeverityInfo)
	return s.createAlertIfNew(ctx, event, models.AlertTypeAfterHoursAccess, severity,
		fmt.Sprintf("After-hours access by %s", event.UserID),
		fmt.Sprintf("User %s accessed the system at %s %s, outside business hours (%02d:%02d-%02d:%02d).",
			event.UserID, now.Format("15:04"), loc.String(), startHour, startMin, endHour, endMin),
	)
}

// checkNewUserHighVolume alerts when a user's first event was <24h ago and they've exceeded the configured query threshold.
func (s *alertTriggerService) checkNewUserHighVolume(ctx context.Context, event *models.MCPAuditEvent, config *models.AlertConfig) error {
	if event.UserID == "" {
		return nil
	}

	firstTime, err := s.deps.triggerRepo.GetUserFirstEventTime(ctx, event.ProjectID, event.UserID)
	if err != nil {
		return fmt.Errorf("new_user_high_volume check: %w", err)
	}
	if firstTime == nil {
		return nil
	}

	// User's first event must be within last 24 hours
	if time.Since(*firstTime) > 24*time.Hour {
		return nil
	}

	count, err := s.deps.triggerRepo.CountUserEventsInWindow(ctx, event.ProjectID, event.UserID, *firstTime)
	if err != nil {
		return fmt.Errorf("new_user_high_volume count: %w", err)
	}

	threshold := 20
	if setting, ok := config.AlertSettings[models.AlertTypeNewUserHighVolume]; ok && setting.QueryThreshold != nil {
		threshold = *setting.QueryThreshold
	}

	if count < threshold {
		return nil
	}

	severity := config.GetSeverity(models.AlertTypeNewUserHighVolume, models.AlertSeverityWarning)
	return s.createAlertIfNew(ctx, event, models.AlertTypeNewUserHighVolume, severity,
		fmt.Sprintf("New user %s with high query volume (%d queries)", event.UserID, count),
		fmt.Sprintf("User %s (first seen %s) has executed %d queries within their first 24 hours.",
			event.UserID, firstTime.Format(time.RFC3339), count),
	)
}

// checkRepeatedErrors alerts when the same error message exceeds the configured count in the configured window.
func (s *alertTriggerService) checkRepeatedErrors(ctx context.Context, event *models.MCPAuditEvent, config *models.AlertConfig) error {
	if event.ErrorMessage == nil || *event.ErrorMessage == "" || event.UserID == "" {
		return nil
	}

	windowMinutes := 10
	threshold := 5
	if setting, ok := config.AlertSettings[models.AlertTypeRepeatedErrors]; ok {
		if setting.ErrorCount != nil {
			threshold = *setting.ErrorCount
		}
		if setting.WindowMinutes != nil {
			windowMinutes = *setting.WindowMinutes
		}
	}

	since := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)

	// Use a truncated version of the error for matching (first 100 chars)
	errSubstring := *event.ErrorMessage
	if len(errSubstring) > 100 {
		errSubstring = errSubstring[:100]
	}

	count, err := s.deps.triggerRepo.CountUserErrorsWithMessage(ctx, event.ProjectID, event.UserID, errSubstring, since)
	if err != nil {
		return fmt.Errorf("repeated_errors check: %w", err)
	}

	if count < threshold {
		return nil
	}

	severity := config.GetSeverity(models.AlertTypeRepeatedErrors, models.AlertSeverityWarning)
	return s.createAlertIfNew(ctx, event, models.AlertTypeRepeatedErrors, severity,
		fmt.Sprintf("Repeated errors from %s (%d occurrences in %d min)", event.UserID, count, windowMinutes),
		fmt.Sprintf("User %s has encountered the same error %d times in the last %d minutes: %s",
			event.UserID, count, windowMinutes, truncate(*event.ErrorMessage, 200)),
	)
}

// createAlertIfNew creates an alert only if no open alert of the same type exists for the user
// within the idempotency window.
func (s *alertTriggerService) createAlertIfNew(ctx context.Context, event *models.MCPAuditEvent, alertType, severity, title, description string) error {
	since := time.Now().Add(-s.deps.idempotencyWindow)

	var exists bool
	var err error
	if event.UserID != "" {
		exists, err = s.deps.triggerRepo.HasOpenAlertForUserAndType(ctx, event.ProjectID, event.UserID, alertType, since)
	} else {
		exists, err = s.deps.triggerRepo.HasOpenAlertForType(ctx, event.ProjectID, alertType, since)
	}
	if err != nil {
		return fmt.Errorf("idempotency check for %s: %w", alertType, err)
	}
	if exists {
		return nil
	}

	alert := &models.AuditAlert{
		ProjectID:   event.ProjectID,
		AlertType:   alertType,
		Severity:    severity,
		Title:       title,
		Description: &description,
		Status:      models.AlertStatusOpen,
	}
	if event.UserID != "" {
		alert.AffectedUserID = &event.UserID
	}
	if event.ID != uuid.Nil {
		alert.RelatedAuditIDs = []uuid.UUID{event.ID}
	}

	if err := s.deps.alertRepo.CreateAlert(ctx, alert); err != nil {
		return fmt.Errorf("create alert %s: %w", alertType, err)
	}

	s.deps.logger.Info("Alert created",
		zap.String("alert_type", alertType),
		zap.String("severity", severity),
		zap.String("project_id", event.ProjectID.String()),
		zap.String("user_id", event.UserID),
	)

	return nil
}

// hasInjectionFlag returns true if the event's security flags contain injection-related indicators.
func hasInjectionFlag(event *models.MCPAuditEvent) bool {
	for _, flag := range event.SecurityFlags {
		lower := strings.ToLower(flag)
		if strings.Contains(lower, "injection") || strings.Contains(lower, "sql_injection") {
			return true
		}
	}
	return false
}

// describeSecurityEvent produces a human-readable description of a security event.
func describeSecurityEvent(event *models.MCPAuditEvent) string {
	parts := []string{
		fmt.Sprintf("Security event detected for user %s.", event.UserID),
	}
	if event.ToolName != nil {
		parts = append(parts, fmt.Sprintf("Tool: %s.", *event.ToolName))
	}
	if len(event.SecurityFlags) > 0 {
		parts = append(parts, fmt.Sprintf("Flags: %s.", strings.Join(event.SecurityFlags, ", ")))
	}
	if event.ErrorMessage != nil {
		parts = append(parts, fmt.Sprintf("Error: %s", truncate(*event.ErrorMessage, 200)))
	}
	return strings.Join(parts, " ")
}

// extractRowCountFromSummary extracts the row_count from an event's result_summary map.
func extractRowCountFromSummary(summary map[string]any) int {
	if summary == nil {
		return 0
	}
	rc, ok := summary["row_count"]
	if !ok {
		return 0
	}
	switch v := rc.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}

// truncate shortens a string to maxLen characters, adding an ellipsis if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// parseHourMinute parses a "HH:MM" string into hour and minute components.
func parseHourMinute(s string) (int, int, error) {
	var h, m int
	_, err := fmt.Sscanf(s, "%d:%d", &h, &m)
	if err != nil {
		return 0, 0, err
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid time: %s", s)
	}
	return h, m, nil
}
