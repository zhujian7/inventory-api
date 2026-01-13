package resources

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/project-kessel/inventory-api/internal/biz/model"
)

type SchemaUsecase struct {
	Log *log.Helper
}

func NewSchemaUsecase(logger *log.Helper) *SchemaUsecase {
	return &SchemaUsecase{
		Log: logger,
	}
}

func (sc *SchemaUsecase) CalculateTuples(currentRepresentation, previousRepresentation *model.Representations, key model.ReporterResourceKey) (model.TuplesToReplicate, error) {
	// Extract workspace IDs from representations
	// currentRepresentation can be nil for DELETE operations (meaning no current/new state)
	currentWorkspaceID := ""
	if currentRepresentation != nil {
		currentWorkspaceID = currentRepresentation.WorkspaceID()
	}
	previousWorkspaceID := ""
	if previousRepresentation != nil {
		previousWorkspaceID = previousRepresentation.WorkspaceID()
	}

	// Extract cluster information for k8s_namespace resources
	currentClusterID := ""
	currentClusterReporter := ""
	if currentRepresentation != nil && key.ResourceType().String() == "k8s_namespace" {
		currentClusterID, currentClusterReporter = currentRepresentation.ClusterInfo()
	}
	previousClusterID := ""
	previousClusterReporter := ""
	if previousRepresentation != nil && key.ResourceType().String() == "k8s_namespace" {
		previousClusterID, previousClusterReporter = previousRepresentation.ClusterInfo()
	}

	// Handle no-op case where workspace and cluster haven't changed
	workspaceChanged := previousWorkspaceID == "" || previousWorkspaceID != currentWorkspaceID
	clusterChanged := previousClusterID == "" || previousClusterID != currentClusterID || previousClusterReporter != currentClusterReporter
	if !workspaceChanged && !clusterChanged {
		return model.TuplesToReplicate{}, nil
	}

	// Build tuples to create and delete
	var tuplesToCreate, tuplesToDelete []model.RelationsTuple

	// Workspace tuples
	if currentWorkspaceID != "" {
		tuplesToCreate = append(tuplesToCreate, model.NewWorkspaceRelationsTuple(currentWorkspaceID, key))
	}
	if previousWorkspaceID != "" {
		tuplesToDelete = append(tuplesToDelete, model.NewWorkspaceRelationsTuple(previousWorkspaceID, key))
	}

	// Cluster tuples for k8s_namespace
	if currentClusterID != "" && currentClusterReporter != "" {
		tuplesToCreate = append(tuplesToCreate, model.NewClusterRelationsTuple(currentClusterID, currentClusterReporter, key))
	}
	if previousClusterID != "" && previousClusterReporter != "" {
		tuplesToDelete = append(tuplesToDelete, model.NewClusterRelationsTuple(previousClusterID, previousClusterReporter, key))
	}

	return model.NewTuplesToReplicate(tuplesToCreate, tuplesToDelete)
}
