package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestChangeSet_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		cs       *ChangeSet
		expected bool
	}{
		{"nil changeset", nil, true},
		{"empty changeset", &ChangeSet{}, true},
		{"has added tables", &ChangeSet{AddedTables: []SchemaTable{{}}}, false},
		{"has modified tables", &ChangeSet{ModifiedTables: []SchemaTable{{}}}, false},
		{"has deleted tables", &ChangeSet{DeletedTables: []SchemaTable{{}}}, false},
		{"has added columns", &ChangeSet{AddedColumns: []SchemaColumn{{}}}, false},
		{"has modified columns", &ChangeSet{ModifiedColumns: []SchemaColumn{{}}}, false},
		{"has deleted columns", &ChangeSet{DeletedColumns: []SchemaColumn{{}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cs.IsEmpty(); got != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestChangeSet_HasChangedColumns(t *testing.T) {
	tests := []struct {
		name     string
		cs       *ChangeSet
		expected bool
	}{
		{"nil", nil, false},
		{"empty", &ChangeSet{}, false},
		{"added columns", &ChangeSet{AddedColumns: []SchemaColumn{{}}}, true},
		{"modified columns", &ChangeSet{ModifiedColumns: []SchemaColumn{{}}}, true},
		{"deleted only", &ChangeSet{DeletedColumns: []SchemaColumn{{}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cs.HasChangedColumns(); got != tt.expected {
				t.Errorf("HasChangedColumns() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestChangeSet_HasChangedTables(t *testing.T) {
	tests := []struct {
		name     string
		cs       *ChangeSet
		expected bool
	}{
		{"nil", nil, false},
		{"empty", &ChangeSet{}, false},
		{"added tables", &ChangeSet{AddedTables: []SchemaTable{{}}}, true},
		{"modified tables", &ChangeSet{ModifiedTables: []SchemaTable{{}}}, true},
		{"deleted only", &ChangeSet{DeletedTables: []SchemaTable{{}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cs.HasChangedTables(); got != tt.expected {
				t.Errorf("HasChangedTables() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestChangeSet_HasDeletedItems(t *testing.T) {
	tests := []struct {
		name     string
		cs       *ChangeSet
		expected bool
	}{
		{"nil", nil, false},
		{"empty", &ChangeSet{}, false},
		{"deleted tables", &ChangeSet{DeletedTables: []SchemaTable{{}}}, true},
		{"deleted columns", &ChangeSet{DeletedColumns: []SchemaColumn{{}}}, true},
		{"added only", &ChangeSet{AddedTables: []SchemaTable{{}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cs.HasDeletedItems(); got != tt.expected {
				t.Errorf("HasDeletedItems() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestChangeSet_ShouldSkipColumn(t *testing.T) {
	colID := uuid.New()
	newColID := uuid.New()

	cs := &ChangeSet{
		UserEditedIDs: map[uuid.UUID]bool{colID: true},
		AddedColumns:  []SchemaColumn{{ID: newColID}},
	}

	tests := []struct {
		name     string
		id       uuid.UUID
		expected bool
	}{
		{"user-edited column", colID, true},
		{"non-edited column", uuid.New(), false},
		{"new column (even if edited)", newColID, false},
	}

	// Make newColID user-edited too
	cs.UserEditedIDs[newColID] = true

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cs.ShouldSkipColumn(tt.id); got != tt.expected {
				t.Errorf("ShouldSkipColumn(%s) = %v, want %v", tt.id, got, tt.expected)
			}
		})
	}

	// nil ChangeSet should not skip
	if (*ChangeSet)(nil).ShouldSkipColumn(colID) {
		t.Error("nil ChangeSet should not skip any column")
	}
}

func TestChangeSet_ShouldSkipTable(t *testing.T) {
	tableID := uuid.New()
	newTableID := uuid.New()

	cs := &ChangeSet{
		UserEditedIDs: map[uuid.UUID]bool{tableID: true, newTableID: true},
		AddedTables:   []SchemaTable{{ID: newTableID}},
	}

	tests := []struct {
		name     string
		id       uuid.UUID
		expected bool
	}{
		{"user-edited table", tableID, true},
		{"non-edited table", uuid.New(), false},
		{"new table (even if edited)", newTableID, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cs.ShouldSkipTable(tt.id); got != tt.expected {
				t.Errorf("ShouldSkipTable(%s) = %v, want %v", tt.id, got, tt.expected)
			}
		})
	}
}

func TestChangeSet_IsAffectedTable(t *testing.T) {
	tableID := uuid.New()
	cs := &ChangeSet{
		AffectedTableIDs: map[uuid.UUID]bool{tableID: true},
	}

	if !cs.IsAffectedTable(tableID) {
		t.Error("expected table to be affected")
	}
	if cs.IsAffectedTable(uuid.New()) {
		t.Error("expected table to not be affected")
	}

	// nil ChangeSet means full extraction — all tables affected
	if !(*ChangeSet)(nil).IsAffectedTable(uuid.New()) {
		t.Error("nil ChangeSet should treat all tables as affected")
	}
}

func TestChangeSet_ChangedColumnIDs(t *testing.T) {
	addedID := uuid.New()
	modifiedID := uuid.New()

	cs := &ChangeSet{
		AddedColumns:    []SchemaColumn{{ID: addedID}},
		ModifiedColumns: []SchemaColumn{{ID: modifiedID}},
	}

	ids := cs.ChangedColumnIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if !ids[addedID] || !ids[modifiedID] {
		t.Error("expected both added and modified IDs")
	}

	nilIDs := (*ChangeSet)(nil).ChangedColumnIDs()
	if len(nilIDs) != 0 {
		t.Error("nil ChangeSet should return empty map")
	}
}

func TestChangeSet_AffectedTableNames(t *testing.T) {
	cs := &ChangeSet{
		AddedTables:    []SchemaTable{{TableName: "new_table"}},
		ModifiedTables: []SchemaTable{{TableName: "existing_table"}, {TableName: "new_table"}}, // duplicate
	}

	names := cs.AffectedTableNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 unique names, got %d: %v", len(names), names)
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["new_table"] || !nameSet["existing_table"] {
		t.Errorf("unexpected names: %v", names)
	}

	// nil ChangeSet returns nil
	if got := (*ChangeSet)(nil).AffectedTableNames(); got != nil {
		t.Errorf("nil ChangeSet should return nil, got %v", got)
	}
}

func TestChangeSet_ToSummary(t *testing.T) {
	cs := &ChangeSet{
		AddedTables:     []SchemaTable{{}, {}},
		ModifiedTables:  []SchemaTable{{}},
		DeletedTables:   []SchemaTable{{}},
		AddedColumns:    []SchemaColumn{{}, {}, {}},
		ModifiedColumns: []SchemaColumn{{}, {}},
		DeletedColumns:  []SchemaColumn{{}},
	}

	summary := cs.ToSummary()
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.TablesAdded != 2 || summary.TablesModified != 1 || summary.TablesDeleted != 1 {
		t.Errorf("table counts: added=%d modified=%d deleted=%d",
			summary.TablesAdded, summary.TablesModified, summary.TablesDeleted)
	}
	if summary.ColumnsAdded != 3 || summary.ColumnsModified != 2 || summary.ColumnsDeleted != 1 {
		t.Errorf("column counts: added=%d modified=%d deleted=%d",
			summary.ColumnsAdded, summary.ColumnsModified, summary.ColumnsDeleted)
	}

	if (*ChangeSet)(nil).ToSummary() != nil {
		t.Error("nil ChangeSet should return nil summary")
	}
}

func TestChangeSet_BuiltAt(t *testing.T) {
	now := time.Now()
	cs := &ChangeSet{BuiltAt: now}
	if !cs.BuiltAt.Equal(now) {
		t.Errorf("BuiltAt mismatch")
	}
}
