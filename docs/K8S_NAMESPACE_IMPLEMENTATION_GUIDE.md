# Guide: Adding k8s_namespace Resource with k8s_cluster Relation

## Overview
This guide explains how to add a new reportable resource (`k8s_namespace`) that has a parent relationship to another resource (`k8s_cluster`) in the Kessel Inventory API, and how to test it in a kind cluster.

## Part 1: Define the Resource Schema

### 1.1 Create Resource Schema Files

Create the following files under `data/schema/resources/k8s_namespace/`:

**`config.yaml`** - Defines the resource type and valid reporters:
```yaml
resourceType: k8s_namespace
reporters:
  - ACM
```

**`common_representation.json`** - Defines common fields (workspace):
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "workspace_id": { "type": "string" }
  },
  "required": ["workspace_id"]
}
```

**`reporters/acm/k8s_namespace.json`** - Defines reporter-specific schema:
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "namespace_name": {
      "type": "string",
      "description": "Name of the Kubernetes namespace"
    },
    "cluster_id": {
      "type": "string",
      "description": "The ID of the cluster this namespace belongs to"
    },
    "cluster_reporter": {
      "type": "string",
      "description": "The reporter type for the cluster (e.g., 'acm')"
    },
    "status": {
      "type": "string",
      "enum": ["NAMESPACE_STATUS_UNSPECIFIED", "NAMESPACE_STATUS_OTHER", "ACTIVE", "TERMINATING"]
    },
    "created_at": { "type": "string", "format": "date-time" },
    "labels": { "type": "object" },
    "annotations": { "type": "object" }
  },
  "required": ["namespace_name", "cluster_id", "cluster_reporter", "status"]
}
```

**Key Point**: The `cluster_id` and `cluster_reporter` fields establish the parent-child relationship in PostgreSQL.

### 1.2 Build Schema Tarball
```bash
make build-schemas
```
This creates `resources.tar.gz` containing all resource schemas.

---

## Part 2: Add SpiceDB Authorization Schema

### 2.1 Update `deploy/schema.zed`

Add the k8s_namespace definition with **both** workspace and cluster relations:

```zed
definition acm/k8s_namespace {
  relation t_workspace: rbac/workspace
  relation t_cluster: acm/k8s_cluster
}
```

**Why both relations?**
- `t_workspace`: For multi-tenant authorization (who can access this namespace)
- `t_cluster`: For hierarchical authorization (namespaces inherit from cluster)

---

## Part 3: Implement Tuple Generation for Cluster Relationship

### 3.1 Add `ClusterInfo()` Method to Representations

**File**: `internal/biz/model/representations.go`

```go
// ClusterInfo returns the cluster_id and cluster_reporter from the reporter representation data.
// Returns empty strings if not present or if reporter representation is not available.
// This is used for k8s_namespace resources to establish the parent cluster relationship.
func (r *Representations) ClusterInfo() (string, string) {
	if r != nil && len(r.reporterData) > 0 {
		clusterID, idOk := r.reporterData["cluster_id"].(string)
		clusterReporter, reporterOk := r.reporterData["cluster_reporter"].(string)
		if idOk && reporterOk {
			return clusterID, clusterReporter
		}
	}
	return "", ""
}
```

### 3.2 Add `NewClusterRelationsTuple()` Function

**File**: `internal/biz/model/relations_tuple.go`

```go
const (
	WorkspaceRelation = "workspace"
	ClusterRelation   = "cluster"  // Add this
	RbacNamespace     = "rbac"
)

func NewClusterRelationsTuple(clusterID string, clusterReporter string, key ReporterResourceKey) RelationsTuple {
	resourceId := key.LocalResourceId()
	resourceType := key.ResourceType()
	reporterType := key.ReporterType()

	namespace := strings.ToLower(reporterType.String())

	resourceObjectType := NewRelationsObjectType(
		strings.ToLower(resourceType.String()),
		namespace,
	)
	resource := NewRelationsResource(resourceId, resourceObjectType)

	clusterSubjectId, _ := NewLocalResourceId(clusterID)
	clusterNamespace := strings.ToLower(clusterReporter)
	clusterObjectType := NewRelationsObjectType("k8s_cluster", clusterNamespace)
	clusterSubject := NewRelationsResource(clusterSubjectId, clusterObjectType)
	subject := NewRelationsSubject(clusterSubject)

	return NewRelationsTuple(resource, ClusterRelation, subject)
}
```

### 3.3 Update `CalculateTuples()` to Generate Cluster Tuples

**File**: `internal/biz/usecase/resources/schema_service.go`

```go
func (sc *SchemaUsecase) CalculateTuples(currentRepresentation, previousRepresentation *model.Representations, key model.ReporterResourceKey) (model.TuplesToReplicate, error) {
	// Extract workspace IDs from representations
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
```

### 3.4 Update Repository to Fetch Reporter Data

**Important Enhancement**: The repository functions must fetch **both** common and reporter representations to support parent-child relationships.

**File**: `internal/data/resource_repository.go`

#### 3.4.1 Update `FindCurrentAndPreviousVersionedRepresentations`

```go
func (r *resourceRepository) FindCurrentAndPreviousVersionedRepresentations(tx *gorm.DB, key bizmodel.ReporterResourceKey, currentCommonVersion *uint, operationType biz.EventOperationType) (*bizmodel.Representations, *bizmodel.Representations, error) {
	if currentCommonVersion == nil {
		return nil, nil, nil
	}

	// Change the struct to include both common and reporter data
	type representationRow struct {
		CommonData                 internal.JsonObject
		CommonVersion              uint
		ReporterData               internal.JsonObject  // Add this
		ReporterVersion            uint                 // Add this
		ResourceId                 uuid.UUID
		ReportedByReporterType     string
		ReportedByReporterInstance string
		TransactionId              string
	}

	var results []representationRow

	db := r.getDBSession(tx)

	// Update the query to JOIN with reporter_representations
	query := db.Table("reporter_resources rr").
		Select("cr.data as common_data, cr.version as common_version, rr2.data as reporter_data, rr2.version as reporter_version, cr.resource_id, cr.reported_by_reporter_type, cr.reported_by_reporter_instance, cr.transaction_id").
		Joins("JOIN common_representations cr ON rr.resource_id = cr.resource_id").
		Joins("LEFT JOIN reporter_representations rr2 ON rr.id = rr2.reporter_resource_id AND cr.version = rr2.common_version")

	query = r.buildReporterResourceKeyQuery(query, key)

	if operationType.OperationType() == biz.OperationTypeCreated {
		query = query.Where("cr.version = ?", *currentCommonVersion)
	} else {
		query = query.Where("(cr.version = ? OR cr.version = ?)", *currentCommonVersion, *currentCommonVersion-1)
	}

	err := query.Find(&results).Error
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find common representations by version: %w", err)
	}

	var current, previous *bizmodel.Representations
	for _, row := range results {
		// Update NewRepresentations call to include reporter data
		rep, err := bizmodel.NewRepresentations(
			bizmodel.Representation(row.CommonData),
			&row.CommonVersion,
			bizmodel.Representation(row.ReporterData),  // Add this
			&row.ReporterVersion,                       // Add this
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create representation: %w", err)
		}

		if row.CommonVersion == *currentCommonVersion {
			current = rep
		} else if *currentCommonVersion > 0 && row.CommonVersion == *currentCommonVersion-1 {
			previous = rep
		}
	}

	return current, previous, nil
}
```

#### 3.4.2 Update `FindLatestRepresentations` Similarly

```go
func (r *resourceRepository) FindLatestRepresentations(tx *gorm.DB, key bizmodel.ReporterResourceKey) (*bizmodel.Representations, error) {
	var result struct {
		CommonData      internal.JsonObject
		CommonVersion   uint
		ReporterData    internal.JsonObject  // Add this
		ReporterVersion uint                 // Add this
	}

	db := r.getDBSession(tx)

	query := db.Table("reporter_resources rr").
		Select("cr.data as common_data, cr.version as common_version, rr2.data as reporter_data, rr2.version as reporter_version").
		Joins("JOIN common_representations cr ON rr.resource_id = cr.resource_id").
		Joins("LEFT JOIN reporter_representations rr2 ON rr.id = rr2.reporter_resource_id AND cr.version = rr2.common_version")

	query = r.buildReporterResourceKeyQuery(query, key)

	err := query.Order("cr.version DESC").Limit(1).Scan(&result).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find latest representations: %w", err)
	}

	// Convert to Representations
	rep, err := bizmodel.NewRepresentations(
		bizmodel.Representation(result.CommonData),
		&result.CommonVersion,
		bizmodel.Representation(result.ReporterData),  // Add this
		&result.ReporterVersion,                       // Add this
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create representation: %w", err)
	}
	return rep, nil
}
```

---

## Part 4: Testing in Kind Cluster

### 4.1 Start Kind Cluster
```bash
make inventory-up-kind
```

### 4.2 Update ConfigMaps and Deploy

**Update SpiceDB schema:**
```bash
kubectl delete configmap spicedb-schema
kubectl create configmap spicedb-schema --from-file=schema.zed=deploy/schema.zed
kubectl rollout restart deployment/relationships
```

**Update resource schemas:**
```bash
kubectl delete configmap resources-tarball
kubectl create configmap resources-tarball --from-file=resources.tar.gz
kubectl rollout restart deployment/kessel-inventory
```

### 4.3 Build and Push Updated Image

```bash
# Build and push image (requires QUAY_REPO_INVENTORY env var)
./build_push_minimal.sh

# Update deployment with your image
kubectl set image deployment/kessel-inventory api=quay.io/<your-repo>/kessel-inventory:<tag>
kubectl rollout status deployment/kessel-inventory
```

### 4.4 Create Test Data

**File**: `data/testData/v1beta2/k8s-namespace.json`

```json
{
  "type": "k8s_namespace",
  "reporterType": "ACM",
  "reporterInstanceId": "14c6b63e-49b2-4cc2-99de-5d914b657548",
  "representations": {
    "metadata": {
      "localResourceId": "namespace-12345-67890-abcdef",
      "apiHref": "https://apiHref.com/namespaces/default",
      "consoleHref": "https://www.console.com/namespaces/default",
      "reporterVersion": "0.2.0"
    },
    "common": {
      "workspace_id": "aee8f698-9d43-49a1-b458-680a7c9dc046"
    },
    "reporter": {
      "namespace_name": "default",
      "cluster_id": "ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d",
      "cluster_reporter": "acm",
      "status": "ACTIVE",
      "created_at": "2024-01-15T10:30:00Z",
      "labels": {
        "kubernetes.io/metadata.name": "default",
        "environment": "production",
        "team": "platform"
      },
      "annotations": {
        "description": "Default namespace for the cluster",
        "managed-by": "acm"
      }
    }
  }
}
```

### 4.5 Create Resources via API

First, create the parent k8s_cluster, then create k8s_namespace referencing it.

```bash
# Port-forward to access API
kubectl port-forward service/kessel-inventory-service 8081:8081 &

# Create k8s_cluster (parent resource)
curl -X POST http://localhost:8081/api/inventory/v1beta2/resources \
  -H "Content-Type: application/json" \
  -d @data/testData/v1beta2/k8s-cluster.json

# Create k8s_namespace (child resource)
curl -X POST http://localhost:8081/api/inventory/v1beta2/resources \
  -H "Content-Type: application/json" \
  -d @data/testData/v1beta2/k8s-namespace.json
```

---

## Part 5: Verify the Implementation

### 5.1 Check Consumer Logs
```bash
kubectl logs -l app=kessel-inventory --tail=100 | grep -i "k8s_namespace\|cluster"
```

Look for:
- `Creating tuples` messages showing both workspace and cluster relations
- No errors about missing object definitions

Expected log output:
```
INFO: Creating tuples: ... k8s_namespace ... relation:"workspace" ...
INFO: Creating tuples: ... k8s_namespace ... relation:"cluster" ...
```

### 5.2 Verify Database - PostgreSQL

**Check resource exists:**
```bash
kubectl exec invdatabase-<pod-id> -- psql -U postgres -d spicedb -c \
  "SELECT type, COUNT(*) FROM resource GROUP BY type;"
```

Expected output:
```
type          | count
--------------+-------
k8s_cluster   |     1
k8s_namespace |     1
```

**Check reporter data includes cluster_id:**
```bash
kubectl exec invdatabase-<pod-id> -- psql -U postgres -d spicedb -c \
  "SELECT rr.data->>'cluster_id' as cluster_id,
          rr.data->>'cluster_reporter' as cluster_reporter,
          rr.data->>'namespace_name' as namespace_name
   FROM reporter_representations rr
   JOIN resource r ON r.id = rr.reporter_resource_id
   WHERE r.type = 'k8s_namespace';"
```

Expected output:
```
cluster_id                           | cluster_reporter | namespace_name
-------------------------------------+------------------+---------------
ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d | acm              | default
```

### 5.3 Verify Tuples - SpiceDB

**Check both tuples exist:**
```bash
kubectl exec postgres-<pod-id> -- psql -U postgres -d spicedb -c \
  "SELECT namespace, object_id, relation, userset_namespace, userset_object_id
   FROM relation_tuple
   WHERE namespace = 'acm/k8s_namespace'
   AND deleted_xid = '9223372036854775807'::xid8;"
```

**Expected output:**
```
namespace         | object_id                    | relation    | userset_namespace | userset_object_id
------------------+------------------------------+-------------+-------------------+--------------------------------------
acm/k8s_namespace | namespace-12345-67890-abcdef | t_workspace | rbac/workspace    | aee8f698-9d43-49a1-b458-680a7c9dc046
acm/k8s_namespace | namespace-12345-67890-abcdef | t_cluster   | acm/k8s_cluster   | ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d
```

✅ **Success**: Both workspace and cluster tuples should be present!

### 5.4 Test Query with JOIN

Verify the relationship works in database queries:

```bash
kubectl exec invdatabase-<pod-id> -- psql -U postgres -d spicedb -c \
  "SELECT
     ns.type as namespace_type,
     ns_rr.data->>'namespace_name' as namespace_name,
     ns_rr.data->>'cluster_id' as cluster_id,
     c.type as cluster_type,
     c_rr.data->>'kube_version' as cluster_version
   FROM resource ns
   JOIN reporter_resources ns_rr_res ON ns.id = ns_rr_res.resource_id
   JOIN reporter_representations ns_rr ON ns_rr_res.id = ns_rr.reporter_resource_id
   LEFT JOIN reporter_resources c_rr_res ON (ns_rr.data->>'cluster_id')::text = c_rr_res.local_resource_id
   LEFT JOIN resource c ON c_rr_res.resource_id = c.id
   LEFT JOIN reporter_representations c_rr ON c_rr_res.id = c_rr.reporter_resource_id
   WHERE ns.type = 'k8s_namespace';"
```

This verifies the PostgreSQL relationship works via the cluster_id field.

### 5.5 Verify Relations Using `zed` Command

The `zed` CLI tool provides a more direct way to inspect SpiceDB schema and relationships.

#### Step 1: Port-forward to SpiceDB service

```bash
kubectl port-forward service/spicedb-cr 50051:50051 &
```

#### Step 2: Read the k8s_namespace schema definition

```bash
zed schema read --endpoint localhost:50051 --insecure | grep -A 5 "acm/k8s_namespace"
```

Expected output:

```
definition acm/k8s_namespace {
  relation t_workspace: rbac/workspace
  relation t_cluster: acm/k8s_cluster
}
```

#### Step 3: Read all relationships for the k8s_namespace resource

```bash
zed relationship read --endpoint localhost:50051 --insecure acm/k8s_namespace:namespace-12345-67890-abcdef
```

Expected output:

```
acm/k8s_namespace:namespace-12345-67890-abcdef t_cluster acm/k8s_cluster:ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d
acm/k8s_namespace:namespace-12345-67890-abcdef t_workspace rbac/workspace:aee8f698-9d43-49a1-b458-680a7c9dc046
```

✅ **Success**: Both `t_cluster` and `t_workspace` relations should be present!

#### Step 4: Find all namespaces belonging to a specific cluster

```bash
zed relationship read --endpoint localhost:50051 --insecure --subject-filter "acm/k8s_cluster:ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d" acm/k8s_namespace
```

Expected output:

```
acm/k8s_namespace:namespace-12345-67890-abcdef t_cluster acm/k8s_cluster:ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d
```

This demonstrates the hierarchical relationship - you can query "which namespaces belong to this cluster".

#### Step 5: Read a specific relation

```bash
zed relationship read --endpoint localhost:50051 --insecure acm/k8s_namespace:namespace-12345-67890-abcdef t_cluster
```

Expected output:

```
acm/k8s_namespace:namespace-12345-67890-abcdef t_cluster acm/k8s_cluster:ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d
```

#### Useful `zed` Commands Summary

```bash
# Read entire schema
zed schema read --endpoint localhost:50051 --insecure

# Read all k8s definitions
zed schema read --endpoint localhost:50051 --insecure | grep -A 10 "acm/k8s"

# Read all relationships for a resource
zed relationship read --endpoint localhost:50051 --insecure acm/k8s_namespace:<resource-id>

# Find resources by parent relationship
zed relationship read --endpoint localhost:50051 --insecure --subject-filter "acm/k8s_cluster:<cluster-id>" acm/k8s_namespace

# Read specific relation type
zed relationship read --endpoint localhost:50051 --insecure acm/k8s_namespace:<resource-id> <relation-name>
```

---

## Key Concepts

### Database Relationships (PostgreSQL)
- **Storage**: JSONB fields in `reporter_representations.data`
- **Fields**: `cluster_id`, `cluster_reporter`
- **Purpose**: Data queries, JOINs, reporting
- **Example**: Find all namespaces in a cluster via JOIN on cluster_id

### Authorization Relationships (SpiceDB)
- **Storage**: Tuples in `relation_tuple` table
- **Two types for k8s_namespace**:
  - **Workspace tuple (`t_workspace`)**: Multi-tenant access control
    - "Who can access this namespace based on workspace membership?"
  - **Cluster tuple (`t_cluster`)**: Hierarchical authorization
    - "Namespaces inherit permissions from their parent cluster"
- **Purpose**: Real-time authorization checks, permission inheritance
- **Generated**: Automatically by consumer from database data

### Data Flow Architecture

```
┌─────────────────┐
│   REST API      │  1. Create k8s_namespace
│  (HTTP POST)    │     with cluster_id field
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   PostgreSQL    │  2. Store in database:
│                 │     - common_representations (workspace_id)
│                 │     - reporter_representations (cluster_id, cluster_reporter)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Outbox Event   │  3. Debezium captures change
│     (Kafka)     │     Publishes to Kafka topic
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│    Consumer     │  4. Reads event, fetches representations
│                 │     Calls CalculateTuples()
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   SpiceDB       │  5. Creates two tuples:
│                 │     - t_workspace tuple
│                 │     - t_cluster tuple
└─────────────────┘
```

---

## Common Issues & Solutions

### Issue 1: "object definition 'acm/k8s_namespace' not found"
**Symptom**: Consumer crashes when processing k8s_namespace events

**Root Cause**: Missing SpiceDB schema definition

**Solution**:
1. Add definition to `deploy/schema.zed`:
   ```zed
   definition acm/k8s_namespace {
     relation t_workspace: rbac/workspace
     relation t_cluster: acm/k8s_cluster
   }
   ```
2. Update ConfigMap:
   ```bash
   kubectl delete configmap spicedb-schema
   kubectl create configmap spicedb-schema --from-file=schema.zed=deploy/schema.zed
   kubectl rollout restart deployment/relationships
   ```

### Issue 2: Cluster tuple not created (only workspace tuple exists)
**Symptom**: Query shows only 1 tuple instead of 2

**Root Cause**: Repository functions not fetching reporter data

**Solution**:
Ensure `FindCurrentAndPreviousVersionedRepresentations` and `FindLatestRepresentations` include:
1. JOIN with `reporter_representations` table
2. Fetch both `CommonData` and `ReporterData`
3. Pass both to `NewRepresentations()`

**Verification**:
```sql
-- Check if reporter data includes cluster_id
SELECT rr.data
FROM reporter_representations rr
JOIN resource r ON r.id = rr.reporter_resource_id
WHERE r.type = 'k8s_namespace';
```

### Issue 3: Consumer crashes on startup after adding new resource
**Symptom**: Consumer pod in CrashLoopBackOff

**Common Causes**:
1. Resource schemas not in Docker image/ConfigMap
2. SpiceDB schema mismatch
3. Missing database migrations

**Solution**:
1. Rebuild schemas: `make build-schemas`
2. Update ConfigMaps before deploying code
3. Check logs: `kubectl logs -l app=kessel-inventory`

### Issue 4: ClusterInfo() returns empty strings
**Symptom**: Cluster tuple not generated even though cluster_id exists in database

**Root Cause**: `ClusterInfo()` reading from `reporterData` but it's nil

**Solution**: Verify repository JOINs include reporter_representations:
```go
Joins("LEFT JOIN reporter_representations rr2 ON rr.id = rr2.reporter_resource_id AND cr.version = rr2.common_version")
```

### Issue 5: Port-forward fails with "service not found"
**Symptom**: Cannot connect to API at localhost:8081

**Solution**: Use correct service name:
```bash
kubectl port-forward service/kessel-inventory-service 8081:8081
```

---

## Testing Checklist

Before considering the implementation complete, verify:

- [ ] **Schema files created**: config.yaml, common_representation.json, reporter schema
- [ ] **Schema tarball built**: `make build-schemas` succeeds
- [ ] **SpiceDB schema updated**: `deploy/schema.zed` includes k8s_namespace definition
- [ ] **ClusterInfo() implemented**: Returns cluster_id and cluster_reporter
- [ ] **NewClusterRelationsTuple() added**: Creates cluster relationship tuple
- [ ] **CalculateTuples() updated**: Generates both workspace and cluster tuples
- [ ] **Repository JOINs updated**: Fetches reporter representations
- [ ] **Unit tests pass**: Schema validation tests work
- [ ] **E2E tests created**: Full CRUD operations tested
- [ ] **ConfigMaps updated**: Both spicedb-schema and resources-tarball
- [ ] **Image rebuilt**: With all code changes
- [ ] **Deployment successful**: Pod running without crashes
- [ ] **Resources created**: Both cluster and namespace via API
- [ ] **Database verified**: cluster_id present in reporter_representations
- [ ] **Tuples verified**: Both t_workspace and t_cluster in SpiceDB
- [ ] **Consumer logs clean**: No errors processing namespace events

---

## Files Changed Summary

### Schema Files (New)
1. `data/schema/resources/k8s_namespace/config.yaml`
2. `data/schema/resources/k8s_namespace/common_representation.json`
3. `data/schema/resources/k8s_namespace/reporters/acm/k8s_namespace.json`

### Code Changes (Modified)
1. **deploy/schema.zed**
   - Added `definition acm/k8s_namespace` with t_workspace and t_cluster relations

2. **internal/biz/model/representations.go**
   - Added `ClusterInfo()` method to extract cluster_id and cluster_reporter

3. **internal/biz/model/relations_tuple.go**
   - Added `ClusterRelation` constant
   - Added `NewClusterRelationsTuple()` function

4. **internal/biz/usecase/resources/schema_service.go**
   - Enhanced `CalculateTuples()` to generate cluster tuples for k8s_namespace

5. **internal/data/resource_repository.go**
   - Updated `FindCurrentAndPreviousVersionedRepresentations()` to fetch reporter data
   - Updated `FindLatestRepresentations()` to fetch reporter data

### Test Files (New)
1. **internal/middleware/k8s_namespace_schema_test.go**
   - Schema loading and validation tests

2. **test/e2e/k8s_namespace_test.go**
   - End-to-end CRUD operation tests

3. **data/testData/v1beta2/k8s-namespace.json**
   - Sample test data for k8s_namespace

---

## Architecture Decisions

### Why Two Relations in SpiceDB?

**Decision**: Include both `t_workspace` and `t_cluster` relations in the SpiceDB schema.

**Rationale**:
1. **Workspace relation**: Required for multi-tenant access control
   - "Can user X access namespace Y based on their workspace membership?"

2. **Cluster relation**: Enables hierarchical authorization patterns
   - "Grant cluster-admin permissions to all namespaces in the cluster"
   - "Inherit cluster-level policies to child namespaces"
   - Matches real-world Kubernetes RBAC patterns

**Alternative Considered**: Only `t_workspace` relation
- **Pros**: Simpler
- **Cons**: Cannot implement cluster-level permissions, loses hierarchical relationship in authorization layer

**Conclusion**: Both relations provide flexibility for future authorization requirements while maintaining proper separation between data relationships (PostgreSQL) and authorization relationships (SpiceDB).

### Why Separate Database and Authorization Relationships?

**PostgreSQL** (`cluster_id` field):
- **Purpose**: Data integrity, queries, reporting
- **Example**: "Show me all namespaces in cluster X"
- **Performance**: Indexed, optimized for queries

**SpiceDB** (`t_cluster` tuple):
- **Purpose**: Real-time authorization decisions
- **Example**: "Does this user have permission to access this namespace?"
- **Performance**: Graph-based, optimized for permission checks

**Benefit**: Each system optimized for its specific use case while maintaining consistency through the event-driven consumer.

---

## Additional Resources

- **Kessel Inventory API Documentation**: [internal docs]
- **SpiceDB Documentation**: https://authzed.com/docs
- **ReBAC Concepts**: Relationship-Based Access Control patterns
- **Debezium CDC**: Change Data Capture for event-driven architecture

---

## Appendix: Complete Example Session

```bash
# 1. Start kind cluster
make inventory-up-kind

# 2. Build schemas
make build-schemas

# 3. Update ConfigMaps
kubectl delete configmap spicedb-schema
kubectl create configmap spicedb-schema --from-file=schema.zed=deploy/schema.zed
kubectl rollout restart deployment/relationships

kubectl delete configmap resources-tarball
kubectl create configmap resources-tarball --from-file=resources.tar.gz
kubectl rollout restart deployment/kessel-inventory

# 4. Build and deploy code
export QUAY_REPO_INVENTORY=quay.io/youruser/kessel-inventory
./build_push_minimal.sh
kubectl set image deployment/kessel-inventory api=$QUAY_REPO_INVENTORY:$(git rev-parse --short=7 HEAD)
kubectl rollout status deployment/kessel-inventory

# 5. Port-forward
kubectl port-forward service/kessel-inventory-service 8081:8081 &

# 6. Create cluster (parent)
curl -X POST http://localhost:8081/api/inventory/v1beta2/resources \
  -H "Content-Type: application/json" \
  -d @data/testData/v1beta2/k8s-cluster.json

# 7. Create namespace (child)
curl -X POST http://localhost:8081/api/inventory/v1beta2/resources \
  -H "Content-Type: application/json" \
  -d @data/testData/v1beta2/k8s-namespace.json

# 8. Verify tuples
kubectl exec postgres-<pod> -- psql -U postgres -d spicedb -c \
  "SELECT namespace, object_id, relation, userset_namespace, userset_object_id
   FROM relation_tuple
   WHERE namespace = 'acm/k8s_namespace'
   AND deleted_xid = '9223372036854775807'::xid8;"

# Expected: 2 rows (t_workspace and t_cluster)
```

---

**Document Version**: 1.0
**Last Updated**: 2026-01-14
**Author**: Generated from k8s_namespace implementation session
