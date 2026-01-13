package middleware_test

import (
	"os"
	"testing"

	"github.com/project-kessel/inventory-api/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestK8sNamespaceSchemaLoading(t *testing.T) {
	// Get the current working directory
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Build the path to the schema directory
	schemaDir := cwd + "/../../data/schema/resources"

	// Preload schemas from filesystem
	err = middleware.PreloadAllSchemasFromFilesystem(schemaDir)
	require.NoError(t, err, "Failed to preload schemas")

	// Verify k8s_namespace common schema is loaded
	commonSchema, ok := middleware.SchemaCache.Load("common:k8s_namespace")
	assert.True(t, ok, "k8s_namespace common schema should be loaded")
	assert.NotNil(t, commonSchema, "k8s_namespace common schema should not be nil")

	// Verify k8s_namespace ACM reporter schema is loaded
	acmSchema, ok := middleware.SchemaCache.Load("k8s_namespace:acm")
	assert.True(t, ok, "k8s_namespace:acm reporter schema should be loaded")
	assert.NotNil(t, acmSchema, "k8s_namespace:acm reporter schema should not be nil")

	// Verify k8s_namespace config is loaded
	config, ok := middleware.SchemaCache.Load("config:k8s_namespace")
	assert.True(t, ok, "k8s_namespace config should be loaded")
	assert.NotNil(t, config, "k8s_namespace config should not be nil")
}

func TestK8sNamespaceValidation(t *testing.T) {
	// Setup schemas
	commonSchema := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"properties": {
			"workspace_id": { "type": "string" }
		},
		"required": ["workspace_id"]
	}`

	reporterSchema := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"properties": {
			"namespace_name": { "type": "string" },
			"cluster_id": { "type": "string" },
			"cluster_reporter": { "type": "string" },
			"status": {
				"type": "string",
				"enum": ["NAMESPACE_STATUS_UNSPECIFIED", "NAMESPACE_STATUS_OTHER", "ACTIVE", "TERMINATING"]
			},
			"created_at": { "type": "string" },
			"labels": {
				"type": "object",
				"additionalProperties": { "type": "string" }
			},
			"annotations": {
				"type": "object",
				"additionalProperties": { "type": "string" }
			}
		},
		"required": ["namespace_name", "cluster_id", "cluster_reporter", "status"]
	}`

	// Store schemas in cache
	middleware.SchemaCache.Store("common:k8s_namespace", commonSchema)
	middleware.SchemaCache.Store("k8s_namespace:acm", reporterSchema)

	t.Run("Valid k8s_namespace data", func(t *testing.T) {
		commonData := map[string]interface{}{
			"workspace_id": "ws-12345",
		}

		reporterData := map[string]interface{}{
			"namespace_name":   "default",
			"cluster_id":       "cluster-abc",
			"cluster_reporter": "acm",
			"status":           "ACTIVE",
			"created_at":       "2024-01-15T10:30:00Z",
			"labels": map[string]interface{}{
				"env": "prod",
			},
		}

		err := middleware.ValidateCommonRepresentation("k8s_namespace", commonData)
		assert.NoError(t, err)

		err = middleware.ValidateReporterRepresentation("k8s_namespace", "acm", reporterData)
		assert.NoError(t, err)
	})

	t.Run("Missing required field - namespace_name", func(t *testing.T) {
		reporterData := map[string]interface{}{
			"cluster_id":       "cluster-abc",
			"cluster_reporter": "acm",
			"status":           "ACTIVE",
		}

		err := middleware.ValidateReporterRepresentation("k8s_namespace", "acm", reporterData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "namespace_name")
	})

	t.Run("Missing required field - cluster_id", func(t *testing.T) {
		reporterData := map[string]interface{}{
			"namespace_name":   "default",
			"cluster_reporter": "acm",
			"status":           "ACTIVE",
		}

		err := middleware.ValidateReporterRepresentation("k8s_namespace", "acm", reporterData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cluster_id")
	})

	t.Run("Invalid status value", func(t *testing.T) {
		reporterData := map[string]interface{}{
			"namespace_name":   "default",
			"cluster_id":       "cluster-abc",
			"cluster_reporter": "acm",
			"status":           "INVALID_STATUS",
		}

		err := middleware.ValidateReporterRepresentation("k8s_namespace", "acm", reporterData)
		assert.Error(t, err)
	})

	t.Run("Valid status - TERMINATING", func(t *testing.T) {
		reporterData := map[string]interface{}{
			"namespace_name":   "test-ns",
			"cluster_id":       "cluster-xyz",
			"cluster_reporter": "acm",
			"status":           "TERMINATING",
		}

		err := middleware.ValidateReporterRepresentation("k8s_namespace", "acm", reporterData)
		assert.NoError(t, err)
	})

	t.Run("With labels and annotations", func(t *testing.T) {
		reporterData := map[string]interface{}{
			"namespace_name":   "production",
			"cluster_id":       "cluster-prod-01",
			"cluster_reporter": "acm",
			"status":           "ACTIVE",
			"labels": map[string]interface{}{
				"environment": "production",
				"team":        "platform",
			},
			"annotations": map[string]interface{}{
				"managed-by":  "acm",
				"description": "Production namespace",
			},
		}

		err := middleware.ValidateReporterRepresentation("k8s_namespace", "acm", reporterData)
		assert.NoError(t, err)
	})
}

func TestK8sNamespaceReporterCombinationValidation(t *testing.T) {
	// Get the current working directory
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Build the path to the schema directory
	schemaDir := cwd + "/../../data/schema/resources"

	// Preload schemas from filesystem
	err = middleware.PreloadAllSchemasFromFilesystem(schemaDir)
	require.NoError(t, err, "Failed to preload schemas")

	t.Run("Valid combination - k8s_namespace with ACM", func(t *testing.T) {
		err := middleware.ValidateResourceReporterCombination("k8s_namespace", "ACM")
		assert.NoError(t, err)
	})

	t.Run("Invalid combination - k8s_namespace with unknown reporter", func(t *testing.T) {
		err := middleware.ValidateResourceReporterCombination("k8s_namespace", "UNKNOWN")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid reporter_type: UNKNOWN for resource_type: k8s_namespace")
	})
}
