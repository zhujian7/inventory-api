ROLE_NAME="app-admin-role"
PERMISSIONS=("create_deployment")
RELATIONS_PORT=9000
TUPLES=""
for i in "${!PERMISSIONS[@]}"; do
    PERM="${PERMISSIONS[$i]}"
    TUPLE="{\"resource\":{\"id\":\"${ROLE_NAME}\",\"type\":{\"name\":\"role\",\"namespace\":\"rbac\"}},\"relation\":\"${PERM}\",\"subject\":{\"subject\":{\"id\":\"*\",\"type\":{\"name\":\"principal\",\"namespace\":\"rbac\"}}}}"
    if [ $i -gt 0 ]; then
        TUPLES="${TUPLES},"
    fi
    TUPLES="${TUPLES}${TUPLE}"
done

MESSAGE="{\"tuples\":[${TUPLES}]}"

echo "Creating role '${ROLE_NAME}' with permissions: ${PERMISSIONS[*]}"
grpcurl -plaintext -d "${MESSAGE}" \
    "localhost:${RELATIONS_PORT}" \
    kessel.relations.v1beta1.KesselTupleService.CreateTuples
