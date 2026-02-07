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
	logger            *zap.Logger
	idempotencyWindow time.Duration
}

type alertTriggerService struct {
	deps alertTriggerDeps
}

func NewAlertTriggerService(
	alertRepo repositories.AlertRepository,
	triggerRepo repositories.AlertTriggerRepository,
	logger *zap.Logger,
) AlertTriggerService {
	return &alertTriggerService{
		deps: alertTriggerDeps{
			alertRepo:         alertRepo,
			triggerRepo:       triggerRepo,
			logger:            logger.Named("alert-trigger"),
			idempotencyWindow: 1 * time.Hour,
		},
	}
}

var _ AlertTriggerService = (*alertTriggerService)(nil)

func (s *alertTriggerService) EvaluateEvent(ctx context.Context, event *models.MCPAuditEvent) error {
	if event == nil || event.ProjectID == uuid.Nil {
		return nil
	}

	// Each trigger runs independently; collect errors but don't stop on first failure
	var errs []string

	triggers := []func(context.Context, *models.MCPAuditEvent) error{
		s.checkSQLInjection,
		s.checkUnusualQueryVolume,
		s.checkLargeDataExport,
		s.checkAfterHoursAccess,
		s.checkNewUserHighVolume,
		s.checkRepeatedErrors,
	}

	for _, trigger := range triggers {
		if err := trigger(ctx, event); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("alert trigger errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// checkSQLInjection creates a critical alert when security_flags indicate injection or security_level is critical.
func (s *alertTriggerService) checkSQLInjection(ctx context.Context, event *models.MCPAuditEvent) error {
	if !hasInjectionFlag(event) && event.SecurityLevel != models.MCPSecurityCritical {
		return nil
	}

	return s.createAlertIfNew(ctx, event, models.AlertTypeSQLInjection, models.AlertSeverityCritical,
		fmt.Sprintf("SQL injection pattern detected from %s", event.UserID),
		describeSecurityEvent(event),
	)
}

// checkUnusualQueryVolume alerts when a user's query count in the last hour exceeds 50.
// Hardcoded threshold; will be configurable via alert config in task 4.4.
func (s *alertTriggerService) checkUnusualQueryVolume(ctx context.Context, event *models.MCPAuditEvent) error {
	if event.UserID == "" {
		return nil
	}

	since := time.Now().Add(-1 * time.Hour)
	count, err := s.deps.triggerRepo.CountUserEventsInWindow(ctx, event.ProjectID, event.UserID, since)
	if err != nil {
		return fmt.Errorf("unusual_query_volume check: %w", err)
	}

	const threshold = 50
	if count < threshold {
		return nil
	}

	return s.createAlertIfNew(ctx, event, models.AlertTypeUnusualQueryVolume, models.AlertSeverityWarning,
		fmt.Sprintf("Unusual query volume from %s (%d queries in last hour)", event.UserID, count),
		fmt.Sprintf("User %s has executed %d queries in the last hour, exceeding threshold of %d.", event.UserID, count, threshold),
	)
}

// checkLargeDataExport alerts when a query returns more than 10,000 rows.
func (s *alertTriggerService) checkLargeDataExport(ctx context.Context, event *models.MCPAuditEvent) error {
	rowCount := extractRowCountFromSummary(event.ResultSummary)
	const threshold = 10000
	if rowCount < threshold {
		return nil
	}

	return s.createAlertIfNew(ctx, event, models.AlertTypeLargeDataExport, models.AlertSeverityInfo,
		fmt.Sprintf("Large data export by %s (%d rows)", event.UserID, rowCount),
		fmt.Sprintf("User %s executed a query returning %d rows (threshold: %d).", event.UserID, rowCount, threshold),
	)
}

// checkAfterHoursAccess alerts when events occur outside business hours (6am-10pm UTC).
func (s *alertTriggerService) checkAfterHoursAccess(ctx context.Context, event *models.MCPAuditEvent) error {
	hour := time.Now().UTC().Hour()
	if hour >= 6 && hour < 22 {
		return nil
	}

	return s.createAlertIfNew(ctx, event, models.AlertTypeAfterHoursAccess, models.AlertSeverityInfo,
		fmt.Sprintf("After-hours access by %s", event.UserID),
		fmt.Sprintf("User %s accessed the system at %s UTC, outside business hours (06:00-22:00).", event.UserID, time.Now().UTC().Format("15:04")),
	)
}

// checkNewUserHighVolume alerts when a user's first event was <24h ago and they've run >20 queries.
func (s *alertTriggerService) checkNewUserHighVolume(ctx context.Context, event *models.MCPAuditEvent) error {
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

	const threshold = 20
	if count < threshold {
		return nil
	}

	return s.createAlertIfNew(ctx, event, models.AlertTypeNewUserHighVolume, models.AlertSeverityWarning,
		fmt.Sprintf("New user %s with high query volume (%d queries)", event.UserID, count),
		fmt.Sprintf("User %s (first seen %s) has executed %d queries within their first 24 hours.",
			event.UserID, firstTime.Format(time.RFC3339), count),
	)
}

// checkRepeatedErrors alerts when the same error message appears >5 times from same user in 10 minutes.
func (s *alertTriggerService) checkRepeatedErrors(ctx context.Context, event *models.MCPAuditEvent) error {
	if event.ErrorMessage == nil || *event.ErrorMessage == "" || event.UserID == "" {
		return nil
	}

	since := time.Now().Add(-10 * time.Minute)

	// Use a truncated version of the error for matching (first 100 chars)
	errSubstring := *event.ErrorMessage
	if len(errSubstring) > 100 {
		errSubstring = errSubstring[:100]
	}

	count, err := s.deps.triggerRepo.CountUserErrorsWithMessage(ctx, event.ProjectID, event.UserID, errSubstring, since)
	if err != nil {
		return fmt.Errorf("repeated_errors check: %w", err)
	}

	const threshold = 5
	if count < threshold {
		return nil
	}

	return s.createAlertIfNew(ctx, event, models.AlertTypeRepeatedErrors, models.AlertSeverityWarning,
		fmt.Sprintf("Repeated errors from %s (%d occurrences in 10 min)", event.UserID, count),
		fmt.Sprintf("User %s has encountered the same error %d times in the last 10 minutes: %s",
			event.UserID, count, truncate(*event.ErrorMessage, 200)),
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
