package central

import (
	"encoding/json"
	"testing"
)

func TestProjectInfo_UnmarshalWithApplications(t *testing.T) {
	body := `{
		"project": {
			"id": "test-id",
			"name": "Test Project",
			"applications": [
				{
					"name": "mcp-server",
					"billing": {
						"status": "dormant",
						"freeSeatsLimit": 2
					}
				}
			],
			"urls": {
				"projectsPage": "https://example.com/projects",
				"projectPage": "https://example.com/projects/test-id"
			}
		}
	}`

	var response struct {
		Project ProjectInfo `json:"project"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	p := response.Project
	if p.ID != "test-id" {
		t.Errorf("expected ID %q, got %q", "test-id", p.ID)
	}
	if p.Name != "Test Project" {
		t.Errorf("expected Name %q, got %q", "Test Project", p.Name)
	}
	if len(p.Applications) != 1 {
		t.Fatalf("expected 1 application, got %d", len(p.Applications))
	}

	app := p.Applications[0]
	if app.Name != "mcp-server" {
		t.Errorf("expected application name %q, got %q", "mcp-server", app.Name)
	}
	if app.Billing == nil {
		t.Fatal("expected billing info, got nil")
	}
	if app.Billing.Status != "dormant" {
		t.Errorf("expected billing status %q, got %q", "dormant", app.Billing.Status)
	}
	if app.Billing.FreeSeatsLimit != 2 {
		t.Errorf("expected freeSeatsLimit %d, got %d", 2, app.Billing.FreeSeatsLimit)
	}
}

func TestProjectInfo_UnmarshalWithoutApplications(t *testing.T) {
	body := `{
		"project": {
			"id": "test-id",
			"name": "Test Project",
			"urls": {
				"projectsPage": "https://example.com/projects"
			}
		}
	}`

	var response struct {
		Project ProjectInfo `json:"project"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if response.Project.Applications != nil {
		t.Errorf("expected nil applications, got %v", response.Project.Applications)
	}
}

func TestProjectInfo_UnmarshalMultipleApplications(t *testing.T) {
	body := `{
		"project": {
			"id": "test-id",
			"name": "Test Project",
			"applications": [
				{"name": "mcp-server"},
				{"name": "ai-data-liaison", "billing": {"status": "active", "freeSeatsLimit": 5}}
			]
		}
	}`

	var response struct {
		Project ProjectInfo `json:"project"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(response.Project.Applications) != 2 {
		t.Fatalf("expected 2 applications, got %d", len(response.Project.Applications))
	}

	if response.Project.Applications[0].Name != "mcp-server" {
		t.Errorf("expected first app %q, got %q", "mcp-server", response.Project.Applications[0].Name)
	}
	if response.Project.Applications[0].Billing != nil {
		t.Error("expected nil billing for mcp-server (not provided)")
	}

	if response.Project.Applications[1].Name != "ai-data-liaison" {
		t.Errorf("expected second app %q, got %q", "ai-data-liaison", response.Project.Applications[1].Name)
	}
	if response.Project.Applications[1].Billing == nil {
		t.Fatal("expected billing for ai-data-liaison")
	}
	if response.Project.Applications[1].Billing.Status != "active" {
		t.Errorf("expected billing status %q, got %q", "active", response.Project.Applications[1].Billing.Status)
	}
}
