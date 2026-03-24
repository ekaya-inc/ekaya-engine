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
	projectParamOntologyCompletionStates     = "ontology_completion_states"
	projectParamOntologyCompletionProvenance = "ontology_completion_provenance"
	projectParamOntologyCompletedAt          = "ontology_completed_at"
	projectParamOntologyCompletedAtFormat    = time.RFC3339Nano
)

type ontologyCompletionState struct {
	Provenance  models.OntologyCompletionProvenance
	CompletedAt *time.Time
}

type ontologyCompletionStatePayload struct {
	Provenance  string `json:"provenance,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type projectSettingsExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func loadOntologyCompletionState(ctx context.Context, execer projectSettingsExecer, projectID, datasourceID uuid.UUID) (*ontologyCompletionState, error) {
	parameters, err := loadProjectParameters(ctx, execer, projectID)
	if err != nil {
		return nil, err
	}

	states, err := loadOntologyCompletionStatePayloads(parameters)
	if err != nil {
		return nil, err
	}

	payload, ok := states[datasourceID.String()]
	if !ok {
		return &ontologyCompletionState{}, nil
	}

	state := &ontologyCompletionState{
		Provenance: models.OntologyCompletionProvenance(payload.Provenance),
	}
	if payload.CompletedAt == "" {
		return state, nil
	}

	parsed, err := time.Parse(projectParamOntologyCompletedAtFormat, payload.CompletedAt)
	if err != nil {
		return nil, fmt.Errorf("parse ontology completion time for datasource %s: %w", datasourceID, err)
	}
	parsed = parsed.UTC()
	state.CompletedAt = &parsed

	return state, nil
}

func storeOntologyCompletionState(
	ctx context.Context,
	execer projectSettingsExecer,
	projectID uuid.UUID,
	datasourceID uuid.UUID,
	provenance models.OntologyCompletionProvenance,
	completedAt time.Time,
) error {
	parameters, err := loadProjectParameters(ctx, execer, projectID)
	if err != nil {
		return err
	}

	if err := setOntologyCompletionState(parameters, datasourceID, provenance, completedAt); err != nil {
		return err
	}

	return updateProjectParameters(ctx, execer, projectID, parameters)
}

func clearOntologyCompletionState(ctx context.Context, execer projectSettingsExecer, projectID, datasourceID uuid.UUID) error {
	parameters, err := loadProjectParameters(ctx, execer, projectID)
	if err != nil {
		return err
	}

	if datasourceID == uuid.Nil {
		delete(parameters, projectParamOntologyCompletionStates)
		delete(parameters, projectParamOntologyCompletionProvenance)
		delete(parameters, projectParamOntologyCompletedAt)
		return updateProjectParameters(ctx, execer, projectID, parameters)
	}

	states, err := loadOntologyCompletionStatePayloads(parameters)
	if err != nil {
		return err
	}
	delete(states, datasourceID.String())
	if len(states) == 0 {
		delete(parameters, projectParamOntologyCompletionStates)
	} else {
		parameters[projectParamOntologyCompletionStates] = states
	}

	delete(parameters, projectParamOntologyCompletionProvenance)
	delete(parameters, projectParamOntologyCompletedAt)

	return updateProjectParameters(ctx, execer, projectID, parameters)
}

func loadOntologyCompletionStatePayloads(parameters map[string]any) (map[string]ontologyCompletionStatePayload, error) {
	rawStates, ok := parameters[projectParamOntologyCompletionStates]
	if !ok || rawStates == nil {
		return map[string]ontologyCompletionStatePayload{}, nil
	}

	payload, err := json.Marshal(rawStates)
	if err != nil {
		return nil, fmt.Errorf("encode ontology completion states: %w", err)
	}

	var states map[string]ontologyCompletionStatePayload
	if err := json.Unmarshal(payload, &states); err != nil {
		return nil, fmt.Errorf("decode ontology completion states: %w", err)
	}
	if states == nil {
		return map[string]ontologyCompletionStatePayload{}, nil
	}

	return states, nil
}

func setOntologyCompletionState(
	parameters map[string]any,
	datasourceID uuid.UUID,
	provenance models.OntologyCompletionProvenance,
	completedAt time.Time,
) error {
	if datasourceID == uuid.Nil {
		return fmt.Errorf("datasource id is required for ontology completion state")
	}

	states, err := loadOntologyCompletionStatePayloads(parameters)
	if err != nil {
		return err
	}

	delete(parameters, projectParamOntologyCompletionProvenance)
	delete(parameters, projectParamOntologyCompletedAt)

	states[datasourceID.String()] = ontologyCompletionStatePayload{
		Provenance:  string(provenance),
		CompletedAt: completedAt.UTC().Format(projectParamOntologyCompletedAtFormat),
	}
	parameters[projectParamOntologyCompletionStates] = states

	return nil
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
