# ACM Kessel Experiment Guide

This guide demonstrates how to experiment with Kessel authorization using ACM (Advanced Cluster Management) resources,
specifically `k8s_cluster` and `k8s_namespace` resources with hierarchical relationships and RBAC.

## Overview

This experiment showcases:
- Resource schema definition for Kubernetes clusters and namespaces
- Kessel authorization schema configuration
- Resource reporting through the inventory API
- Role-based access control (RBAC) setup
- Permission inheritance through resource hierarchy

## Prerequisites

- Kessel inventory API running on `localhost:8081`
- Kessel relations service running on `localhost:9000`
- `curl` for API requests
- `grpcurl` for gRPC calls

## Step 1: Resource Schema Definition

The resource schemas are defined at:
- [data/schema/resources/k8s_cluster](../../data/schema/resources/k8s_cluster)
- [data/schema/resources/k8s_namespace](../../data/schema/resources/k8s_namespace)

Each schema includes:
- `common_representation.json` - Common fields across all reporters
- `config.yaml` - Resource configuration and metadata
- `reporters/` - Reporter-specific schema definitions (e.g., ACM)

## Step 2: Define ACM Kessel Schema

The Kessel authorization schema is defined in [deploy/kessel](../../deploy/kessel):

- `acm.ksl` - ACM resource definitions (k8s_cluster, k8s_namespace)
- `rbac.ksl` - RBAC definitions (role, role_binding, principal)
- `schema.zed` - Compiled authorization schema

To regenerate the schema after making changes:

```bash
# Convert .ksl files to schema.zed
ksl kessel.ksl rbac.ksl acm.ksl
```

## Step 3: Report Resources

Report a Kubernetes cluster and namespace to the inventory API.

### Report K8s Cluster

```bash
curl -X POST http://localhost:8081/api/inventory/v1beta2/resources \
  -H "Content-Type: application/json" \
  -d @data/testData/v1beta2/k8s-cluster.json
```

**Sample cluster data** ([k8s-cluster.json](../../data/testData/v1beta2/k8s-cluster.json)):
- Local Resource ID: `ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d`
- Workspace ID: `aee8f698-9d43-49a1-b458-680a7c9dc046`
- Reporter: ACM

### Report K8s Namespace

```bash
curl -X POST http://localhost:8081/api/inventory/v1beta2/resources \
  -H "Content-Type: application/json" \
  -d @data/testData/v1beta2/k8s-namespace.json
```

**Sample namespace data** ([k8s-namespace.json](../../data/testData/v1beta2/k8s-namespace.json)):
- Local Resource ID: `namespace-12345-67890-abcdef`
- Cluster ID: `ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d` (links to cluster above)
- Workspace ID: `aee8f698-9d43-49a1-b458-680a7c9dc046`
- Reporter: ACM

## Step 4: Create RBAC Role

Create an application admin role with specific permissions.

```bash
./test/acm/create-role.sh
```

This script ([create-role.sh](../../test/acm/create-role.sh)) creates a role named `app-admin-role` with permissions:
- `create_deployment`
- `create_secret`

The role is stored in the Kessel relations service at `localhost:9000`.

## Step 5: Create Role Bindings

Role bindings grant users access to roles for specific resources. We demonstrate three levels of access:

### Workspace Admin Binding

```bash
./test/acm/create-workspace-admin-binding.sh
```

This creates a binding ([create-workspace-admin-binding.sh](../../test/acm/create-workspace-admin-binding.sh)):
- User: `workspace-admin`
- Resource: `aee8f698-9d43-49a1-b458-680a7c9dc046` (workspace)
- Role: `app-admin-role`

**Permissions inherited**: Access to all clusters and namespaces in the workspace.

### Cluster Admin Binding

```bash
./test/acm/create-cluster-admin-binding.sh
```

This creates a binding ([create-cluster-admin-binding.sh](../../test/acm/create-cluster-admin-binding.sh)):
- User: `cluster-admin`
- Resource: `ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d` (k8s_cluster)
- Role: `app-admin-role`

**Permissions inherited**: Access to all namespaces in the cluster.

### Namespace Admin Binding

```bash
./test/acm/create-namespace-admin-binding.sh
```

This creates a binding ([create-namespace-admin-binding.sh](../../test/acm/create-namespace-admin-binding.sh)):
- User: `namespace-12345-67890-abcdef-admin`
- Resource: `namespace-12345-67890-abcdef` (k8s_namespace)
- Role: `app-admin-role`

**Permissions inherited**: Access only to the specific namespace.

## Step 6: Check Permissions

Verify authorization using the check API endpoint.

### Example 1: Unauthorized User (Expected: ALLOWED_FALSE)

```bash
CHECK_MESSAGE='{"object": {"resource_type": "k8s_namespace", "resource_id": "namespace-12345-67890-abcdef",
"reporter": {"type": "acm"}}, "relation": "create_deployment", "subject": {"resource": {"resource_type":
"principal", "resource_id": "namespace-12345-67890-abcdef-admin-1", "reporter": {"type": "rbac"}}}}'

curl -H "Content-Type: application/json" -X POST -d "$CHECK_MESSAGE" \
  "http://localhost:8081/api/inventory/v1beta2/check"
```

**Response:**
```json
{"allowed":"ALLOWED_FALSE"}
```

The user `namespace-12345-67890-abcdef-admin-1` does not have a role binding, so access is denied.

### Example 2: Authorized User (Expected: ALLOWED_TRUE)

```bash
CHECK_MESSAGE='{"object": {"resource_type": "k8s_namespace", "resource_id": "namespace-12345-67890-abcdef",
"reporter": {"type": "acm"}}, "relation": "create_deployment", "subject": {"resource": {"resource_type":
"principal", "resource_id": "namespace-12345-67890-abcdef-admin", "reporter": {"type": "rbac"}}}}'

curl -H "Content-Type: application/json" -X POST -d "$CHECK_MESSAGE" \
  "http://localhost:8081/api/inventory/v1beta2/check"
```

**Response:**
```json
{"allowed":"ALLOWED_TRUE"}
```

The user `namespace-12345-67890-abcdef-admin` has the `app-admin-role` bound to the namespace, granting
`create_deployment` permission.

### Example 3: Cluster Admin with Inherited Permissions (Expected: ALLOWED_TRUE)

```bash
CHECK_MESSAGE='{"object": {"resource_type": "k8s_namespace", "resource_id": "namespace-12345-67890-abcdef",
"reporter": {"type": "acm"}}, "relation": "create_deployment", "subject": {"resource": {"resource_type":
"principal", "resource_id": "cluster-admin", "reporter": {"type": "rbac"}}}}'

curl -H "Content-Type: application/json" -X POST -d "$CHECK_MESSAGE" \
  "http://localhost:8081/api/inventory/v1beta2/check"
```

**Response:**
```json
{"allowed":"ALLOWED_TRUE"}
```

The user `cluster-admin` has the `app-admin-role` bound to the cluster `ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d`.
Due to hierarchical permission inheritance, this admin can `create_deployment` in all namespaces within that cluster,
including `namespace-12345-67890-abcdef`.

### Example 4: Namespace Admin Without Cluster Permissions (Expected: ALLOWED_FALSE)

```bash
CHECK_MESSAGE='{"object": {"resource_type": "k8s_cluster", "resource_id": "ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d",
"reporter": {"type": "acm"}}, "relation": "create_deployment", "subject": {"resource": {"resource_type":
"principal", "resource_id": "namespace-12345-67890-abcdef-admin", "reporter": {"type": "rbac"}}}}'

curl -H "Content-Type: application/json" -X POST -d "$CHECK_MESSAGE" \
  "http://localhost:8081/api/inventory/v1beta2/check"
```

**Response:**
```json
{"allowed":"ALLOWED_FALSE"}
```

The user `namespace-12345-67890-abcdef-admin` only has permissions for the specific namespace
`namespace-12345-67890-abcdef`. Permissions do not propagate upward in the hierarchy, so this user
cannot perform `create_deployment` on the cluster resource itself.

## Permission Hierarchy

The authorization model supports hierarchical permission inheritance:

```
Workspace (aee8f698-9d43-49a1-b458-680a7c9dc046)
  └─ K8s Cluster (ae5c7a82-cb3b-4591-9b10-3ae1506d4f3d)
      └─ K8s Namespace (namespace-12345-67890-abcdef)
```

- **Workspace admins** inherit permissions to all clusters and namespaces
- **Cluster admins** inherit permissions to all namespaces in the cluster
- **Namespace admins** have permissions only to their specific namespace

## Testing Different Scenarios

You can test additional authorization scenarios:

1. **Workspace admin accessing a namespace**:
   - User: `workspace-admin`
   - Expected: `ALLOWED_TRUE` (inherits from workspace)

2. **Cluster admin accessing a namespace**:
   - User: `cluster-admin`
   - Expected: `ALLOWED_TRUE` (inherits from cluster)

3. **Different permission**:
   - Relation: `create_secret`
   - Expected: `ALLOWED_TRUE` for users with `app-admin-role`

## Troubleshooting

### Common Issues

1. **Connection refused errors**:
   - Ensure inventory API is running on port 8081
   - Ensure relations service is running on port 9000

2. **ALLOWED_FALSE when expecting TRUE**:
   - Verify role bindings were created successfully
   - Check that resource IDs match exactly
   - Confirm the reporter types are correct

3. **Schema validation errors**:
   - Regenerate schema.zed from .ksl files
   - Restart services after schema changes

## Related Files

- Resource schemas: [data/schema/resources/](../../data/schema/resources/)
- Kessel schemas: [deploy/kessel/](../../deploy/kessel/)
- Test data: [data/testData/v1beta2/](../../data/testData/v1beta2/)
- RBAC scripts: [test/acm/](../../test/acm/)

## References

For more information, see:
- [K8S Namespace Implementation Guide](../K8S_NAMESPACE_IMPLEMENTATION_GUIDE.md)
- Feature commit: `1a28c9ed022b407db04d6a088c6252fdd9b856f7`
