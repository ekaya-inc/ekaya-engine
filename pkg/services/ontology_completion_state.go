package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

const (
	projectParamOntologyCompletionProvenance = "ontology_completion_provenance"
	projectParamOntologyCompletedAt          = "ontology_completed_at"
	projectParamOntologyCompletedAtFormat    = time.RFC3339Nano
)

type ontologyCompletionState struct {
	Provenance  models.OntologyCompletionProvenance
	CompletedAt *time.Time
}

type projectSettingsExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func loadOntologyCompletionState(ctx context.Context, execer projectSettingsExecer, projectID uuid.UUID) (*ontologyCompletionState, error) {
	var raw []byte
	if err := execer.QueryRow(ctx, `SELECT parameters FROM engine_projects WHERE id = $1`, projectID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("load project parameters: %w", err)
	}

	parameters := make(map[string]any)
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &parameters); err != nil {
			return nil, fmt.Errorf("decode project parameters: %w", err)
		}
	}

	state := &ontologyCompletionState{}
	if value, ok := parameters[projectParamOntologyCompletionProvenance].(string); ok {
		state.Provenance = models.OntologyCompletionProvenance(value)
	}
	if value, ok := parameters[projectParamOntologyCompletedAt].(string); ok && value != "" {
		parsed, err := time.Parse(projectParamOntologyCompletedAtFormat, value)
		if err != nil {
			return nil, fmt.Errorf("parse ontology completion time: %w", err)
		}
		parsed = parsed.UTC()
		state.CompletedAt = &parsed
	}

	return state, nil
}

func storeOntologyCompletionState(
	ctx context.Context,
	execer projectSettingsExecer,
	projectID uuid.UUID,
	provenance models.OntologyCompletionProvenance,
	completedAt time.Time,
) error {
	parameters, err := loadProjectParameters(ctx, execer, projectID)
	if err != nil {
		return err
	}

	parameters[projectParamOntologyCompletionProvenance] = string(provenance)
	parameters[projectParamOntologyCompletedAt] = completedAt.UTC().Format(projectParamOntologyCompletedAtFormat)

	return updateProjectParameters(ctx, execer, projectID, parameters)
}

func clearOntologyCompletionState(ctx context.Context, execer projectSettingsExecer, projectID uuid.UUID) error {
	parameters, err := loadProjectParameters(ctx, execer, projectID)
	if err != nil {
		return err
	}

	delete(parameters, projectParamOntologyCompletionProvenance)
	delete(parameters, projectParamOntologyCompletedAt)

	return updateProjectParameters(ctx, execer, projectID, parameters)
}

func loadProjectParameters(ctx context.Context, execer projectSettingsExecer, projectID uuid.UUID) (map[string]any, error) {
	var raw []byte
	if err := execer.QueryRow(ctx, `SELECT parameters FROM engine_projects WHERE id = $1`, projectID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("load project parameters: %w", err)
	}

	parameters := make(map[string]any)
	if len(raw) == 0 || string(raw) == "null" {
		return parameters, nil
	}

	if err := json.Unmarshal(raw, &parameters); err != nil {
		return nil, fmt.Errorf("decode project parameters: %w", err)
	}

	return parameters, nil
}

func updateProjectParameters(ctx context.Context, execer projectSettingsExecer, projectID uuid.UUID, parameters map[string]any) error {
	payload, err := json.Marshal(parameters)
	if err != nil {
		return fmt.Errorf("encode project parameters: %w", err)
	}

	if _, err := execer.Exec(ctx, `
		UPDATE engine_projects
		SET parameters = $2, updated_at = $3
		WHERE id = $1
	`, projectID, payload, time.Now().UTC()); err != nil {
		return fmt.Errorf("update project parameters: %w", err)
	}

	return nil
}
