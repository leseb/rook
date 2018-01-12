#!/usr/bin/env bash
set -e


#############
# VARIABLES #
#############

rook_git_root=$(git rev-parse --show-toplevel || echo ".")
rook_kube_templates_dir="$rook_git_root/cluster/examples/kubernetes/"


#############
# FUNCTIONS #
#############

function fail_if_wrong_node {
  if [[ "$(hostname)" != "k8s-01" ]]; then
    echo "You must run the script from k8s-01"
    exit 1
  fi
}

function purge_rook_pods {
  cd "$rook_kube_templates_dir"
  kubectl delete -f wordpress.yaml || true
  kubectl delete -f mysql.yaml || true
  kubectl delete -n rook pool replicapool || true
  kubectl delete storageclass rook-block || true
  kubectl -n kube-system delete secret rook-admin || true
  kubectl delete -f kube-registry.yaml || true
  kubectl delete -n rook cluster rook || true
  kubectl delete thirdpartyresources cluster.rook.io pool.rook.io objectstore.rook.io filesystem.rook.io volumeattachment.rook.io || true # ignore errors if on K8s 1.7+
  kubectl delete crd clusters.rook.io pools.rook.io objectstores.rook.io filesystems.rook.io volumeattachments.rook.io || true # ignore errors if on K8s 1.5 and 1.6
  kubectl delete -n rook-system daemonset rook-agent || true
  kubectl delete -f rook-operator.yaml || true
  kubectl delete clusterroles rook-agent || true
  kubectl delete clusterrolebindings rook-agent || true
  kubectl delete namespace rook || true
  cd "$rook_git_root"
}

function install_rpm_dep {
  sudo yum install -y go git perl-Digest-SHA
}

function add_user_to_docker_group {
  sudo groupadd docker || true
  sudo gpasswd -a vagrant docker || true
  newgrp docker <<EONG
EONG
  #if [[ $(id -Gn) =~ docker ]]; then
  #  exec sg docker "$0 $*"
  #fi
}

function run_docker_registry {
  if ! docker ps | grep -sq registry; then
    docker run -d -p 5000:5000 --restart=always --name registry registry:2
  fi
}

function docker_import {
  img=$(docker images | grep -Eo '^build-[a-z0-9]{8}/rook-[a-z0-9]+\s')
  # shellcheck disable=SC2086
  docker tag $img 172.17.8.1:5000/rook/rook:latest
  docker --debug push 172.17.8.1:5000/rook/rook:latest
  # shellcheck disable=SC2086
  docker rmi $img
}

function make_rook {
  # go to the repository root dir
  cd "$rook_git_root"
  # build rook
  make
}

# NOTE (leseb): I'm letting this unsued once I figured out how to purge all the nodes  from k8s-01
function run_rook {
  cd "$rook_kube_templates_dir"
  kubectl create -f rook-operator.yaml
  kubectl create -f rook-cluster.yaml
  cd -
}


function edit_rook_cluster_template {
  cd "$rook_kube_templates_dir"
  sed -i 's|image: .*$|image: 172.17.8.1:5000/rook/rook:latest|' rook-operator.yaml
  echo "rook-operator.yml has been edited with the new image 172.17.8.1:5000/rook/rook:latest"
  echo "Now RUN purge-ceph.sh from your host."
  cd -
}


########
# MAIN #
########

fail_if_wrong_node
install_rpm_dep
add_user_to_docker_group
run_docker_registry
# we purge rook otherwise make fails for 'use-use' image
purge_rook_pods
make_rook
docker_import
edit_rook_cluster_template
