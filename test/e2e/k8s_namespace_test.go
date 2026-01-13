package e2e

import (
	"context"
	"testing"

	pbv1beta2 "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta2"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	grpcinsecure "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestInventoryAPI_v1beta2_k8s_namespace_create_and_update(t *testing.T) {
	enableShortMode(t)
	ctx := context.Background()

	workspaceId := "workspace-test-12345"
	namespaceLocalId := "namespace-e2e-test-12345"
	clusterLocalId := "cluster-e2e-test-67890"
	reporterType := "ACM"
	reporterInstanceId := "e2e-test-instance"

	t.Logf("=== Testing k8s_namespace Resource ===")
	t.Logf("Namespace Local ID: %s", namespaceLocalId)
	t.Logf("Cluster Local ID: %s", clusterLocalId)
	t.Logf("Workspace ID: %s", workspaceId)

	// Create gRPC connection
	conn, err := grpc.NewClient(
		inventoryapi_grpc_url,
		grpc.WithTransportCredentials(grpcinsecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerAuth{token: "1234"}),
	)
	assert.NoError(t, err, "Failed to create gRPC client")
	defer func() {
		if connErr := conn.Close(); connErr != nil {
			t.Logf("Failed to close gRPC connection: %v", connErr)
		}
	}()

	conn.Connect()
	client := pbv1beta2.NewKesselInventoryServiceClient(conn)

	// Step 1: Create the parent k8s_cluster first
	t.Logf("1. Creating parent k8s_cluster resource")
	clusterReporterData, err := structpb.NewStruct(map[string]interface{}{
		"external_cluster_id": "external-cluster-123",
		"cluster_status":      "READY",
		"kube_version":        "1.28.0",
	})
	assert.NoError(t, err, "Failed to create cluster reporter data")

	clusterReq := &pbv1beta2.ReportResourceRequest{
		WriteVisibility:    pbv1beta2.WriteVisibility_MINIMIZE_LATENCY,
		Type:               "k8s_cluster",
		ReporterType:       reporterType,
		ReporterInstanceId: reporterInstanceId,
		Representations: &pbv1beta2.ResourceRepresentations{
			Metadata: &pbv1beta2.RepresentationMetadata{
				LocalResourceId: clusterLocalId,
				ApiHref:         "https://api.example.com/clusters/test-cluster",
				ConsoleHref:     proto.String("https://console.example.com/clusters/test-cluster"),
			},
			Common: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"workspace_id": structpb.NewStringValue(workspaceId),
				},
			},
			Reporter: clusterReporterData,
		},
	}

	_, err = client.ReportResource(ctx, clusterReq)
	assert.NoError(t, err, "Failed to create k8s_cluster resource")
	t.Logf("✓ k8s_cluster created successfully")

	// Step 2: Create k8s_namespace resource
	t.Logf("2. Creating k8s_namespace resource")
	namespaceReporterData, err := structpb.NewStruct(map[string]interface{}{
		"namespace_name":   "test-namespace",
		"cluster_id":       clusterLocalId,
		"cluster_reporter": "acm",
		"status":           "ACTIVE",
		"created_at":       "2024-01-15T10:30:00Z",
		"labels": map[string]interface{}{
			"environment": "test",
			"team":        "platform",
		},
		"annotations": map[string]interface{}{
			"description": "E2E test namespace",
			"managed-by":  "acm",
		},
	})
	assert.NoError(t, err, "Failed to create namespace reporter data")

	namespaceReq := &pbv1beta2.ReportResourceRequest{
		WriteVisibility:    pbv1beta2.WriteVisibility_MINIMIZE_LATENCY,
		Type:               "k8s_namespace",
		ReporterType:       reporterType,
		ReporterInstanceId: reporterInstanceId,
		Representations: &pbv1beta2.ResourceRepresentations{
			Metadata: &pbv1beta2.RepresentationMetadata{
				LocalResourceId: namespaceLocalId,
				ApiHref:         "https://api.example.com/namespaces/test-namespace",
				ConsoleHref:     proto.String("https://console.example.com/namespaces/test-namespace"),
			},
			Common: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"workspace_id": structpb.NewStringValue(workspaceId),
				},
			},
			Reporter: namespaceReporterData,
		},
	}

	_, err = client.ReportResource(ctx, namespaceReq)
	assert.NoError(t, err, "Failed to create k8s_namespace resource")
	t.Logf("✓ k8s_namespace created successfully")

	// Step 3: Update k8s_namespace status
	t.Logf("3. Updating k8s_namespace status to TERMINATING")
	updatedNamespaceReporterData, err := structpb.NewStruct(map[string]interface{}{
		"namespace_name":   "test-namespace",
		"cluster_id":       clusterLocalId,
		"cluster_reporter": "acm",
		"status":           "TERMINATING",
		"created_at":       "2024-01-15T10:30:00Z",
		"labels": map[string]interface{}{
			"environment": "test",
			"team":        "platform",
		},
		"annotations": map[string]interface{}{
			"description": "E2E test namespace - being deleted",
			"managed-by":  "acm",
		},
	})
	assert.NoError(t, err, "Failed to create updated namespace reporter data")

	updateReq := &pbv1beta2.ReportResourceRequest{
		WriteVisibility:    pbv1beta2.WriteVisibility_MINIMIZE_LATENCY,
		Type:               "k8s_namespace",
		ReporterType:       reporterType,
		ReporterInstanceId: reporterInstanceId,
		Representations: &pbv1beta2.ResourceRepresentations{
			Metadata: &pbv1beta2.RepresentationMetadata{
				LocalResourceId: namespaceLocalId,
				ApiHref:         "https://api.example.com/namespaces/test-namespace",
				ConsoleHref:     proto.String("https://console.example.com/namespaces/test-namespace"),
			},
			Common: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"workspace_id": structpb.NewStringValue(workspaceId),
				},
			},
			Reporter: updatedNamespaceReporterData,
		},
	}

	_, err = client.ReportResource(ctx, updateReq)
	assert.NoError(t, err, "Failed to update k8s_namespace resource")
	t.Logf("✓ k8s_namespace updated successfully")

	// Step 4: Delete k8s_namespace
	t.Logf("4. Deleting k8s_namespace resource")
	deleteReq := &pbv1beta2.DeleteResourceRequest{
		Reference: &pbv1beta2.ResourceReference{
			ResourceType: "k8s_namespace",
			ResourceId:   namespaceLocalId,
			Reporter: &pbv1beta2.ReporterReference{
				Type:       reporterType,
				InstanceId: proto.String(reporterInstanceId),
			},
		},
	}

	_, err = client.DeleteResource(ctx, deleteReq)
	assert.NoError(t, err, "Failed to delete k8s_namespace resource")
	t.Logf("✓ k8s_namespace deleted successfully")

	// Step 5: Delete k8s_cluster (cleanup)
	t.Logf("5. Deleting k8s_cluster resource (cleanup)")
	deleteClusterReq := &pbv1beta2.DeleteResourceRequest{
		Reference: &pbv1beta2.ResourceReference{
			ResourceType: "k8s_cluster",
			ResourceId:   clusterLocalId,
			Reporter: &pbv1beta2.ReporterReference{
				Type:       reporterType,
				InstanceId: proto.String(reporterInstanceId),
			},
		},
	}

	_, err = client.DeleteResource(ctx, deleteClusterReq)
	assert.NoError(t, err, "Failed to delete k8s_cluster resource")
	t.Logf("✓ k8s_cluster deleted successfully")

	t.Logf("=== All k8s_namespace tests passed! ===")
}

func TestInventoryAPI_v1beta2_k8s_namespace_validation(t *testing.T) {
	enableShortMode(t)
	ctx := context.Background()

	t.Logf("=== Testing k8s_namespace Validation ===")

	// Create gRPC connection
	conn, err := grpc.NewClient(
		inventoryapi_grpc_url,
		grpc.WithTransportCredentials(grpcinsecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&bearerAuth{token: "1234"}),
	)
	assert.NoError(t, err, "Failed to create gRPC client")
	defer func() {
		if connErr := conn.Close(); connErr != nil {
			t.Logf("Failed to close gRPC connection: %v", connErr)
		}
	}()

	conn.Connect()
	client := pbv1beta2.NewKesselInventoryServiceClient(conn)

	// Test 1: Missing required field - namespace_name
	t.Logf("1. Testing validation with missing namespace_name (should fail)")
	invalidReporterData, err := structpb.NewStruct(map[string]interface{}{
		"cluster_id":       "cluster-123",
		"cluster_reporter": "acm",
		"status":           "ACTIVE",
	})
	assert.NoError(t, err)

	invalidReq := &pbv1beta2.ReportResourceRequest{
		WriteVisibility:    pbv1beta2.WriteVisibility_MINIMIZE_LATENCY,
		Type:               "k8s_namespace",
		ReporterType:       "ACM",
		ReporterInstanceId: "test-instance",
		Representations: &pbv1beta2.ResourceRepresentations{
			Metadata: &pbv1beta2.RepresentationMetadata{
				LocalResourceId: "invalid-namespace-1",
				ApiHref:         "https://api.example.com/namespaces/invalid",
			},
			Common: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"workspace_id": structpb.NewStringValue("workspace-123"),
				},
			},
			Reporter: invalidReporterData,
		},
	}

	_, err = client.ReportResource(ctx, invalidReq)
	assert.Error(t, err, "Expected validation error for missing namespace_name")
	t.Logf("✓ Validation correctly rejected missing namespace_name: %v", err)

	// Test 2: Invalid status value
	t.Logf("2. Testing validation with invalid status value (should fail)")
	invalidStatusData, err := structpb.NewStruct(map[string]interface{}{
		"namespace_name":   "test-ns",
		"cluster_id":       "cluster-123",
		"cluster_reporter": "acm",
		"status":           "INVALID_STATUS",
	})
	assert.NoError(t, err)

	invalidStatusReq := &pbv1beta2.ReportResourceRequest{
		WriteVisibility:    pbv1beta2.WriteVisibility_MINIMIZE_LATENCY,
		Type:               "k8s_namespace",
		ReporterType:       "ACM",
		ReporterInstanceId: "test-instance",
		Representations: &pbv1beta2.ResourceRepresentations{
			Metadata: &pbv1beta2.RepresentationMetadata{
				LocalResourceId: "invalid-namespace-2",
				ApiHref:         "https://api.example.com/namespaces/invalid",
			},
			Common: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"workspace_id": structpb.NewStringValue("workspace-123"),
				},
			},
			Reporter: invalidStatusData,
		},
	}

	_, err = client.ReportResource(ctx, invalidStatusReq)
	assert.Error(t, err, "Expected validation error for invalid status")
	t.Logf("✓ Validation correctly rejected invalid status: %v", err)

	t.Logf("=== All k8s_namespace validation tests passed! ===")
}
