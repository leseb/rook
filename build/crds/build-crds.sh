#!/usr/bin/env bash

# Copyright 2021 The Rook Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )/../.." && pwd -P)
CONTROLLER_GEN_BIN_PATH=$1
# allowDangerousTypes is used to accept float64
CRD_OPTIONS="crd:trivialVersions=true,allowDangerousTypes=true"
OLM_CATALOG_DIR="${SCRIPT_ROOT}/cluster/olm/ceph/deploy/crds"
CRDS_FILE_PATH="${SCRIPT_ROOT}/cluster/examples/kubernetes/ceph/crds.yaml"
HELM_CRDS_FILE_PATH="${SCRIPT_ROOT}/cluster/charts/rook-ceph/templates/resources.yaml"
HELM_CRDS_BEFORE_1_16_FILE_APTH=build/crds/helm-resources-before-1.16

########
# MAIN #
########
# ensures the vendor dir has the right deps, e,g. code-generator
echo "vendoring project"
go mod vendor

echo "generating crds.yaml"
"$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./pkg/apis/ceph.rook.io/v1" output:crd:artifacts:config="$OLM_CATALOG_DIR"
"$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./pkg/apis/rook.io/v1alpha2" output:crd:artifacts:config="$OLM_CATALOG_DIR"
"$CONTROLLER_GEN_BIN_PATH" "$CRD_OPTIONS" paths="./vendor/github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1" output:crd:artifacts:config="$OLM_CATALOG_DIR"

true > "$CRDS_FILE_PATH"
cat <<EOF > "$CRDS_FILE_PATH"
##############################################################################
# Create the CRDs that are necessary before creating your Rook cluster.
# These resources *must* be created before the cluster.yaml or their variants.
##############################################################################
EOF

true > "$HELM_CRDS_FILE_PATH"
cat <<EOF > "$HELM_CRDS_FILE_PATH"
{{- if .Values.crds.enabled }}
{{- if semverCompare ">=1.16.0" .Capabilities.KubeVersion.GitVersion }}
EOF

for crd in "$OLM_CATALOG_DIR/"*.yaml; do
  cat "$crd" >> "$CRDS_FILE_PATH"
  cat "$crd" >> "$HELM_CRDS_FILE_PATH"
done

echo "generating helm resources.yaml"
cat "$HELM_CRDS_BEFORE_1_16_FILE_APTH" >> "$HELM_CRDS_FILE_PATH"
