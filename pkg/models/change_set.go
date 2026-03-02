package models

import (
	"time"

	"github.com/google/uuid"
)

// ChangeSet represents the set of schema changes since the last successful ontology extraction.
// When nil, nodes perform full extraction. When present, nodes scope their work to only
// the affected items, preserving existing user-curated metadata.
type ChangeSet struct {
	// BuiltAt is the completed_at timestamp from the last successful DAG.
	BuiltAt time.Time

	// Tables
	AddedTables    []SchemaTable // created_at > built_at
	ModifiedTables []SchemaTable // updated_at > built_at, created_at <= built_at
	DeletedTables  []SchemaTable // deleted_at > built_at

	// Columns
	AddedColumns    []SchemaColumn // created_at > built_at
	ModifiedColumns []SchemaColumn // updated_at > built_at, created_at <= built_at
	DeletedColumns  []SchemaColumn // deleted_at > built_at

	// Computed helpers
	AffectedTableIDs map[uuid.UUID]bool // union of all table IDs that need re-processing
	UserEditedIDs    map[uuid.UUID]bool // column/table metadata IDs to skip (last_edit_source != NULL)
}

// IsEmpty returns true if no schema changes were detected.
func (cs *ChangeSet) IsEmpty() bool {
	if cs == nil {
		return true
	}
	return len(cs.AddedTables) == 0 &&
		len(cs.ModifiedTables) == 0 &&
		len(cs.DeletedTables) == 0 &&
		len(cs.AddedColumns) == 0 &&
		len(cs.ModifiedColumns) == 0 &&
		len(cs.DeletedColumns) == 0
}

// HasChangedColumns returns true if any columns were added or modified.
func (cs *ChangeSet) HasChangedColumns() bool {
	if cs == nil {
		return false
	}
	return len(cs.AddedColumns) > 0 || len(cs.ModifiedColumns) > 0
}

// HasChangedTables returns true if any tables were added or modified.
func (cs *ChangeSet) HasChangedTables() bool {
	if cs == nil {
		return false
	}
	return len(cs.AddedTables) > 0 || len(cs.ModifiedTables) > 0
}

// HasDeletedItems returns true if any tables or columns were deleted.
func (cs *ChangeSet) HasDeletedItems() bool {
	if cs == nil {
		return false
	}
	return len(cs.DeletedTables) > 0 || len(cs.DeletedColumns) > 0
}

// ShouldSkipColumn returns true if a column should be skipped during re-extraction.
// A column is skipped if it has been user-edited AND is not a newly added column.
func (cs *ChangeSet) ShouldSkipColumn(schemaColumnID uuid.UUID) bool {
	if cs == nil {
		return false
	}
	if !cs.UserEditedIDs[schemaColumnID] {
		return false
	}
	// New columns always get extracted, even if they have metadata from a previous edit
	for _, col := range cs.AddedColumns {
		if col.ID == schemaColumnID {
			return false
		}
	}
	return true
}

// ShouldSkipTable returns true if a table should be skipped during re-extraction.
// A table is skipped if it has been user-edited AND is not a newly added table.
func (cs *ChangeSet) ShouldSkipTable(schemaTableID uuid.UUID) bool {
	if cs == nil {
		return false
	}
	if !cs.UserEditedIDs[schemaTableID] {
		return false
	}
	// New tables always get extracted
	for _, tbl := range cs.AddedTables {
		if tbl.ID == schemaTableID {
			return false
		}
	}
	return true
}

// IsAffectedTable returns true if the given table ID is in the affected set.
func (cs *ChangeSet) IsAffectedTable(tableID uuid.UUID) bool {
	if cs == nil {
		return true // nil ChangeSet means full extraction — all tables affected
	}
	return cs.AffectedTableIDs[tableID]
}

// ChangedColumnIDs returns the set of schema column IDs that were added or modified.
func (cs *ChangeSet) ChangedColumnIDs() map[uuid.UUID]bool {
	result := make(map[uuid.UUID]bool)
	if cs == nil {
		return result
	}
	for _, col := range cs.AddedColumns {
		result[col.ID] = true
	}
	for _, col := range cs.ModifiedColumns {
		result[col.ID] = true
	}
	return result
}

// AffectedTableNames returns the names of tables in the affected set.
// Used by nodes that accept table name filters (e.g., ColumnEnrichment).
func (cs *ChangeSet) AffectedTableNames() []string {
	if cs == nil {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	for _, t := range cs.AddedTables {
		if !seen[t.TableName] {
			seen[t.TableName] = true
			names = append(names, t.TableName)
		}
	}
	for _, t := range cs.ModifiedTables {
		if !seen[t.TableName] {
			seen[t.TableName] = true
			names = append(names, t.TableName)
		}
	}
	return names
}

// ChangeSummary is a JSON-serializable summary of what changed in an incremental extraction.
type ChangeSummary struct {
	TablesAdded     int `json:"tables_added"`
	TablesModified  int `json:"tables_modified"`
	TablesDeleted   int `json:"tables_deleted"`
	ColumnsAdded    int `json:"columns_added"`
	ColumnsModified int `json:"columns_modified"`
	ColumnsDeleted  int `json:"columns_deleted"`
}

// ToSummary creates a ChangeSummary from a ChangeSet.
func (cs *ChangeSet) ToSummary() *ChangeSummary {
	if cs == nil {
		return nil
	}
	return &ChangeSummary{
		TablesAdded:     len(cs.AddedTables),
		TablesModified:  len(cs.ModifiedTables),
		TablesDeleted:   len(cs.DeletedTables),
		ColumnsAdded:    len(cs.AddedColumns),
		ColumnsModified: len(cs.ModifiedColumns),
		ColumnsDeleted:  len(cs.DeletedColumns),
	}
}
