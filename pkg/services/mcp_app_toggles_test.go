package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestGetToolAppID_ApprovedQueryCoreToolsBelongToOntologyForge(t *testing.T) {
	assert.Equal(t, models.AppIDOntologyForge, GetToolAppID("create_approved_query", "developer"))
	assert.Equal(t, models.AppIDOntologyForge, GetToolAppID("list_approved_queries", "developer"))
	assert.Equal(t, models.AppIDOntologyForge, GetToolAppID("list_approved_queries", "user"))
	assert.Equal(t, models.AppIDOntologyForge, GetToolAppID("execute_approved_query", "user"))
	assert.Equal(t, models.AppIDAIDataLiaison, GetToolAppID("suggest_approved_query", "user"))
	assert.Equal(t, models.AppIDAIDataLiaison, GetToolAppID("list_query_suggestions", "developer"))
}

func TestGetToolOwningAppIDs_ApprovedQueryCoreToolsHaveSingleOntologyForgeOwner(t *testing.T) {
	assert.Equal(t, []string{models.AppIDOntologyForge}, GetToolOwningAppIDs("create_approved_query"))
	assert.Equal(t, []string{models.AppIDOntologyForge}, GetToolOwningAppIDs("list_approved_queries"))
	assert.Equal(t, []string{models.AppIDOntologyForge}, GetToolOwningAppIDs("execute_approved_query"))
	assert.Equal(t, []string{models.AppIDAIDataLiaison}, GetToolOwningAppIDs("suggest_query_update"))
}
