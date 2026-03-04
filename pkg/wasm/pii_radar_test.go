package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// --- Test helpers for PII Radar ---

// schemaResponse builds a mock get_schema JSON response.
func schemaResponse(tables ...mockTable) []byte {
	type col struct {
		Name            string `json:"name"`
		DataType        string `json:"data_type"`
		IsPrimaryKey    bool   `json:"is_primary_key"`
		IsNullable      bool   `json:"is_nullable"`
		OrdinalPosition int    `json:"ordinal_position"`
	}
	type tbl struct {
		Schema   string `json:"schema"`
		Name     string `json:"name"`
		RowCount int    `json:"row_count"`
		Columns  []col  `json:"columns"`
	}

	tbls := make([]tbl, 0)
	for _, t := range tables {
		var cols []col
		for i, c := range t.columns {
			cols = append(cols, col{
				Name:            c.name,
				DataType:        c.dataType,
				IsPrimaryKey:    c.isPK,
				IsNullable:      true,
				OrdinalPosition: i + 1,
			})
		}
		tbls = append(tbls, tbl{
			Schema:   t.schema,
			Name:     t.name,
			RowCount: t.rowCount,
			Columns:  cols,
		})
	}

	result, _ := json.Marshal(map[string]any{
		"tables":        tbls,
		"relationships": []any{},
		"table_count":   len(tbls),
	})
	return result
}

type mockTable struct {
	schema   string
	name     string
	rowCount int
	columns  []mockColumn
}

type mockColumn struct {
	name     string
	dataType string
	isPK     bool
}

// queryResponse builds a mock query JSON response.
func queryResponse(columns []string, rows []map[string]any) []byte {
	type qCol struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	var qCols []qCol
	for _, c := range columns {
		qCols = append(qCols, qCol{Name: c, Type: "varchar"})
	}

	result, _ := json.Marshal(map[string]any{
		"columns":   qCols,
		"rows":      rows,
		"row_count": len(rows),
		"truncated": false,
	})
	return result
}

// emptyQueryResponse returns a query response with no rows.
func emptyQueryResponse(columns []string) []byte {
	return queryResponse(columns, []map[string]any{})
}

// piiRadarInvoker creates a MapToolInvoker with get_schema and query handlers.
func piiRadarInvoker(schemaData []byte, queryHandler func(ctx context.Context, args map[string]any) ([]byte, bool, error)) *MapToolInvoker {
	return &MapToolInvoker{
		Handlers: map[string]func(ctx context.Context, arguments map[string]any) ([]byte, bool, error){
			"get_schema": func(_ context.Context, _ map[string]any) ([]byte, bool, error) {
				return schemaData, false, nil
			},
			"query": queryHandler,
		},
	}
}

func loadPiiRadarWasm(t *testing.T) []byte {
	t.Helper()
	wasmBytes, err := os.ReadFile(testdataPath("pii_radar_guest.wasm"))
	if err != nil {
		t.Fatalf("failed to read pii_radar_guest.wasm: %v", err)
	}
	return wasmBytes
}

func runPiiRadar(t *testing.T, invoker *MapToolInvoker, store *MemoryStateStore) map[string]any {
	t.Helper()
	return runPiiRadarWithInput(t, invoker, store, `{"now":"2026-03-04T12:00:00Z"}`)
}

func runPiiRadarWithInput(t *testing.T, invoker *MapToolInvoker, store *MemoryStateStore, input string) map[string]any {
	t.Helper()
	wasmBytes := loadPiiRadarWasm(t)

	hostFuncs := append(
		[]HostFunc{ToolInvokeHostFunc(invoker)},
		StateHostFuncs(store, "pii-radar")...,
	)

	rt := NewRuntime()
	out, err := rt.LoadAndRun(context.Background(), wasmBytes, "run", []byte(input), hostFuncs)
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}

	var report map[string]any
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("failed to unmarshal report: %v\nraw output: %s", err, string(out))
	}
	return report
}

// --- Tests ---

func TestPiiRadar_ColumnNameDetection(t *testing.T) {
	schema := schemaResponse(mockTable{
		schema:   "public",
		name:     "users",
		rowCount: 100,
		columns: []mockColumn{
			{name: "id", dataType: "integer", isPK: true},
			{name: "user_password", dataType: "varchar"},
			{name: "api_key", dataType: "varchar"},
			{name: "email", dataType: "varchar"},
			{name: "first_name", dataType: "varchar"},
			{name: "order_total", dataType: "numeric"},
		},
	})

	invoker := piiRadarInvoker(schema, func(_ context.Context, args map[string]any) ([]byte, bool, error) {
		return emptyQueryResponse([]string{"id", "user_password", "api_key", "email", "first_name", "order_total"}), false, nil
	})
	store := NewMemoryStateStore()
	report := runPiiRadar(t, invoker, store)

	if report["status"] != "completed" {
		t.Fatalf("expected status=completed, got %v", report["status"])
	}

	newFindings, ok := report["new_findings"].([]any)
	if !ok {
		t.Fatalf("new_findings is not an array: %T", report["new_findings"])
	}

	// Should detect: user_password (password), api_key, email
	foundPatterns := map[string]bool{}
	for _, f := range newFindings {
		finding := f.(map[string]any)
		foundPatterns[finding["pattern_name"].(string)] = true
		// Column findings should have detection_type=column
		if finding["detection_type"] != "column" {
			t.Errorf("expected detection_type=column for %s, got %s", finding["pattern_name"], finding["detection_type"])
		}
	}

	if !foundPatterns["password"] {
		t.Error("expected finding for password column")
	}
	if !foundPatterns["api_key"] {
		t.Error("expected finding for api_key column")
	}
	if !foundPatterns["email"] {
		t.Error("expected finding for email column")
	}

	// first_name and order_total should NOT be detected
	if foundPatterns["first_name"] {
		t.Error("first_name should not be detected")
	}
	if foundPatterns["order_total"] {
		t.Error("order_total should not be detected")
	}
}

func TestPiiRadar_ContentDetection(t *testing.T) {
	schema := schemaResponse(mockTable{
		schema:   "public",
		name:     "data",
		rowCount: 3,
		columns: []mockColumn{
			{name: "id", dataType: "integer", isPK: true},
			{name: "col_a", dataType: "varchar"},
			{name: "col_b", dataType: "varchar"},
			{name: "col_c", dataType: "varchar"},
		},
	})

	invoker := piiRadarInvoker(schema, func(_ context.Context, args map[string]any) ([]byte, bool, error) {
		return queryResponse(
			[]string{"id", "col_a", "col_b", "col_c"},
			[]map[string]any{
				{"id": 1, "col_a": "123-45-6789", "col_b": "test@example.com", "col_c": "nothing"},
				{"id": 2, "col_a": "safe data", "col_b": "more safe", "col_c": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
			},
		), false, nil
	})
	store := NewMemoryStateStore()
	report := runPiiRadar(t, invoker, store)

	if report["status"] != "completed" {
		t.Fatalf("expected status=completed, got %v", report["status"])
	}

	newFindings := report["new_findings"].([]any)

	foundPatterns := map[string]string{}
	for _, f := range newFindings {
		finding := f.(map[string]any)
		if finding["detection_type"] == "content" {
			foundPatterns[finding["pattern_name"].(string)] = finding["category"].(string)
		}
	}

	if foundPatterns["ssn"] != "pii_identity" {
		t.Errorf("expected SSN finding with pii_identity category, got %q", foundPatterns["ssn"])
	}
	if foundPatterns["email_address"] != "pii_contact" {
		t.Errorf("expected email finding with pii_contact category, got %q", foundPatterns["email_address"])
	}
	if foundPatterns["jwt_token"] != "secrets" {
		t.Errorf("expected JWT finding with secrets category, got %q", foundPatterns["jwt_token"])
	}
}

func TestPiiRadar_CreditCardLuhn(t *testing.T) {
	schema := schemaResponse(mockTable{
		schema:   "public",
		name:     "payments",
		rowCount: 2,
		columns: []mockColumn{
			{name: "id", dataType: "integer", isPK: true},
			{name: "card_data", dataType: "varchar"},
		},
	})

	invoker := piiRadarInvoker(schema, func(_ context.Context, args map[string]any) ([]byte, bool, error) {
		return queryResponse(
			[]string{"id", "card_data"},
			[]map[string]any{
				{"id": 1, "card_data": "4111111111111111"}, // Valid Luhn
				{"id": 2, "card_data": "4111111111111112"}, // Invalid Luhn
			},
		), false, nil
	})
	store := NewMemoryStateStore()
	report := runPiiRadar(t, invoker, store)

	newFindings := report["new_findings"].([]any)

	ccFound := false
	for _, f := range newFindings {
		finding := f.(map[string]any)
		if finding["pattern_name"] == "credit_card_number" {
			ccFound = true
			// Verify the redacted sample doesn't contain the full number
			redacted := finding["redacted_sample"].(string)
			if strings.Contains(redacted, "4111111111111111") {
				t.Error("redacted sample contains full credit card number")
			}
		}
	}

	if !ccFound {
		t.Error("expected credit card finding for valid Luhn number")
	}
}

func TestPiiRadar_HighWatermark(t *testing.T) {
	schema := schemaResponse(mockTable{
		schema:   "public",
		name:     "events",
		rowCount: 2000,
		columns: []mockColumn{
			{name: "id", dataType: "integer", isPK: true},
			{name: "data", dataType: "varchar"},
		},
	})

	callCount := 0
	var capturedSQL []string

	invoker := piiRadarInvoker(schema, func(_ context.Context, args map[string]any) ([]byte, bool, error) {
		callCount++
		sql, _ := args["sql"].(string)
		capturedSQL = append(capturedSQL, sql)

		// First call: return some rows
		if callCount == 1 {
			return queryResponse(
				[]string{"id", "data"},
				[]map[string]any{
					{"id": 100, "data": "safe1"},
					{"id": 200, "data": "safe2"},
				},
			), false, nil
		}
		// Second call: return more rows
		return queryResponse(
			[]string{"id", "data"},
			[]map[string]any{
				{"id": 300, "data": "safe3"},
			},
		), false, nil
	})
	store := NewMemoryStateStore()

	// First run
	runPiiRadar(t, invoker, store)

	// Verify HWM was stored
	data, _, err := store.Get(context.Background(), "pii-radar")
	if err != nil {
		t.Fatalf("store.Get failed: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state failed: %v", err)
	}
	hwms, ok := state["high_watermarks"].(map[string]any)
	if !ok {
		t.Fatalf("high_watermarks not found in state")
	}
	hwm, ok := hwms["public.events"].(map[string]any)
	if !ok {
		t.Fatalf("public.events HWM not found")
	}
	if hwm["value"] != "200" {
		t.Errorf("expected HWM value=200, got %v", hwm["value"])
	}

	// Second run
	capturedSQL = nil
	callCount = 0
	runPiiRadar(t, invoker, store)

	// Second query should use HWM
	if len(capturedSQL) == 0 {
		t.Fatal("no queries captured on second run")
	}
	if !strings.Contains(capturedSQL[0], "200") {
		t.Errorf("expected second run query to use HWM 200, got: %s", capturedSQL[0])
	}
}

func TestPiiRadar_EmptySchema(t *testing.T) {
	schema := schemaResponse() // No tables
	invoker := piiRadarInvoker(schema, func(_ context.Context, _ map[string]any) ([]byte, bool, error) {
		return nil, false, fmt.Errorf("should not be called")
	})
	store := NewMemoryStateStore()
	report := runPiiRadar(t, invoker, store)

	if report["status"] != "completed" {
		t.Fatalf("expected status=completed, got %v", report["status"])
	}
	if report["tables_total"] != float64(0) {
		t.Errorf("expected tables_total=0, got %v", report["tables_total"])
	}

	newFindings := report["new_findings"].([]any)
	if len(newFindings) != 0 {
		t.Errorf("expected no findings, got %d", len(newFindings))
	}

	errList := report["errors"].([]any)
	if len(errList) != 0 {
		t.Errorf("expected no errors, got %v", errList)
	}
}

func TestPiiRadar_NoPIIFound(t *testing.T) {
	schema := schemaResponse(mockTable{
		schema:   "public",
		name:     "orders",
		rowCount: 3,
		columns: []mockColumn{
			{name: "id", dataType: "integer", isPK: true},
			{name: "created_at", dataType: "timestamp"},
			{name: "amount", dataType: "numeric"},
		},
	})

	invoker := piiRadarInvoker(schema, func(_ context.Context, _ map[string]any) ([]byte, bool, error) {
		return queryResponse(
			[]string{"id", "created_at", "amount"},
			[]map[string]any{
				{"id": 1, "created_at": "2026-01-01", "amount": 99.99},
				{"id": 2, "created_at": "2026-01-02", "amount": 150.00},
				{"id": 3, "created_at": "2026-01-03", "amount": 200.50},
			},
		), false, nil
	})
	store := NewMemoryStateStore()
	report := runPiiRadar(t, invoker, store)

	newFindings := report["new_findings"].([]any)
	if len(newFindings) != 0 {
		t.Errorf("expected no findings for clean data, got %d: %v", len(newFindings), newFindings)
	}
}

func TestPiiRadar_JSONBSecrets(t *testing.T) {
	schema := schemaResponse(mockTable{
		schema:   "public",
		name:     "users",
		rowCount: 1,
		columns: []mockColumn{
			{name: "id", dataType: "integer", isPK: true},
			{name: "agent_data", dataType: "jsonb"},
		},
	})

	invoker := piiRadarInvoker(schema, func(_ context.Context, _ map[string]any) ([]byte, bool, error) {
		return queryResponse(
			[]string{"id", "agent_data"},
			[]map[string]any{
				{"id": 1, "agent_data": `{"livekit_api_key": "sk_live_abc123def456"}`},
			},
		), false, nil
	})
	store := NewMemoryStateStore()
	report := runPiiRadar(t, invoker, store)

	newFindings := report["new_findings"].([]any)

	foundSecrets := false
	for _, f := range newFindings {
		finding := f.(map[string]any)
		if finding["category"] == "secrets" && finding["detection_type"] == "content" {
			foundSecrets = true
		}
	}

	if !foundSecrets {
		t.Error("expected secrets finding for JSONB containing API key")
	}
}

func TestPiiRadar_RedactionInFindings(t *testing.T) {
	schema := schemaResponse(mockTable{
		schema:   "public",
		name:     "sensitive",
		rowCount: 2,
		columns: []mockColumn{
			{name: "id", dataType: "integer", isPK: true},
			{name: "ssn_col", dataType: "varchar"},
			{name: "cc_col", dataType: "varchar"},
			{name: "email_col", dataType: "varchar"},
		},
	})

	ssn := "123-45-6789"
	cc := "4111111111111111"
	email := "secret@company.com"

	invoker := piiRadarInvoker(schema, func(_ context.Context, _ map[string]any) ([]byte, bool, error) {
		return queryResponse(
			[]string{"id", "ssn_col", "cc_col", "email_col"},
			[]map[string]any{
				{"id": 1, "ssn_col": ssn, "cc_col": cc, "email_col": email},
			},
		), false, nil
	})
	store := NewMemoryStateStore()

	// Get the raw output to check for sensitive data
	wasmBytes := loadPiiRadarWasm(t)
	hostFuncs := append(
		[]HostFunc{ToolInvokeHostFunc(invoker)},
		StateHostFuncs(store, "pii-radar")...,
	)
	rt := NewRuntime()
	out, err := rt.LoadAndRun(context.Background(), wasmBytes, "run", []byte(`{"now":"2026-03-04T12:00:00Z"}`), hostFuncs)
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}

	// Check report output doesn't contain full sensitive values
	outStr := string(out)
	if strings.Contains(outStr, ssn) {
		t.Error("report output contains full SSN")
	}
	if strings.Contains(outStr, cc) {
		t.Error("report output contains full credit card number")
	}
	if strings.Contains(outStr, "secret@") {
		t.Error("report output contains full email local part")
	}

	// Check persisted state doesn't contain full sensitive values
	stateData, _, err := store.Get(context.Background(), "pii-radar")
	if err != nil {
		t.Fatalf("store.Get failed: %v", err)
	}
	stateStr := string(stateData)
	if strings.Contains(stateStr, ssn) {
		t.Error("persisted state contains full SSN")
	}
	if strings.Contains(stateStr, cc) {
		t.Error("persisted state contains full credit card number")
	}
	if strings.Contains(stateStr, "secret@") {
		t.Error("persisted state contains full email local part")
	}
}

func TestPiiRadar_IncrementalScan(t *testing.T) {
	schema := schemaResponse(mockTable{
		schema:   "public",
		name:     "users",
		rowCount: 100,
		columns: []mockColumn{
			{name: "id", dataType: "integer", isPK: true},
			{name: "data", dataType: "varchar"},
		},
	})

	// Pre-seed state with existing findings and HWM
	seedState := map[string]any{
		"high_watermarks": map[string]any{
			"public.users": map[string]any{
				"pk_column": "id",
				"value":     "50",
				"completed": false,
			},
		},
		"findings": []any{
			map[string]any{
				"table":             "public.users",
				"column":            "data",
				"category":          "pii_contact",
				"pattern_name":      "email_address",
				"confidence":        "high",
				"detection_type":    "content",
				"redacted_sample":   "****@example.com",
				"first_detected_at": "2026-03-03T10:00:00Z",
				"last_seen_at":      "2026-03-03T10:00:00Z",
				"occurrence_count":  5,
			},
		},
		"last_scan_at": "2026-03-03T10:00:00Z",
		"scan_count":   2,
	}

	store := NewMemoryStateStore()
	seedBytes, _ := json.Marshal(seedState)
	_, err := store.Set(context.Background(), "pii-radar", seedBytes, 0)
	if err != nil {
		t.Fatalf("failed to seed state: %v", err)
	}

	var capturedSQL string
	invoker := piiRadarInvoker(schema, func(_ context.Context, args map[string]any) ([]byte, bool, error) {
		sql, _ := args["sql"].(string)
		capturedSQL = sql

		// Return rows with email content (same pattern as seeded)
		return queryResponse(
			[]string{"id", "data"},
			[]map[string]any{
				{"id": 75, "data": "newuser@example.com"},
			},
		), false, nil
	})

	report := runPiiRadar(t, invoker, store)

	// Verify query used the HWM
	if !strings.Contains(capturedSQL, "50") {
		t.Errorf("expected query to use HWM 50, got: %s", capturedSQL)
	}

	// Verify findings were merged (not duplicated)
	stateData, _, err := store.Get(context.Background(), "pii-radar")
	if err != nil {
		t.Fatalf("store.Get failed: %v", err)
	}
	var finalState map[string]any
	if err := json.Unmarshal(stateData, &finalState); err != nil {
		t.Fatalf("unmarshal state failed: %v", err)
	}

	findings := finalState["findings"].([]any)
	emailFindings := 0
	for _, f := range findings {
		finding := f.(map[string]any)
		if finding["pattern_name"] == "email_address" && finding["table"] == "public.users" {
			emailFindings++
			// Occurrence count should be incremented
			count := finding["occurrence_count"].(float64)
			if count != 6 {
				t.Errorf("expected occurrence_count=6, got %v", count)
			}
			// last_seen_at should be updated
			if finding["last_seen_at"] != "2026-03-04T12:00:00Z" {
				t.Errorf("expected last_seen_at to be updated, got %v", finding["last_seen_at"])
			}
			// first_detected_at should be preserved
			if finding["first_detected_at"] != "2026-03-03T10:00:00Z" {
				t.Errorf("expected first_detected_at to be preserved, got %v", finding["first_detected_at"])
			}
		}
	}

	if emailFindings != 1 {
		t.Errorf("expected exactly 1 email finding (merged), got %d", emailFindings)
	}

	// scan_count should be incremented
	scanCount := finalState["scan_count"].(float64)
	if scanCount != 3 {
		t.Errorf("expected scan_count=3, got %v", scanCount)
	}

	// Verify new_findings should be empty (email was already known)
	newFindings := report["new_findings"].([]any)
	for _, f := range newFindings {
		finding := f.(map[string]any)
		if finding["pattern_name"] == "email_address" && finding["detection_type"] == "content" {
			t.Error("email_address should NOT be in new_findings since it was already known")
		}
	}
}
