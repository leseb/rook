#!/usr/bin/env bash
set -e


#############
# FUNCTIONS #
#############

function test_run {
  # looking for the vagrant command is probably enough to make sure we run this on the host
  if ! command -v vagrant &> /dev/null ; then
    echo "Run me from your host, NOT within your Kubernetes virtual machine!"
    exit 1
  fi
}

function purge_ceph {
  instances=$(vagrant global-status | awk '/k8s-/ { print $1 }')
  for i in $instances; do
    # assuming /var/lib/rook is not ideal but it should work most of the time
    vagrant ssh "$i" -c "cat << 'EOF' > /tmp/purge-ceph.sh
    sudo rm -rf /var/lib/rook
    for disk in \$(sudo blkid | awk '/ROOK/ {print \$1}' | sed 's/[0-9]://' | uniq); do
    sudo dd if=/dev/zero of=\$disk bs=1M count=20 oflag=direct
    done
EOF"
    vagrant ssh "$i" -c "bash /tmp/purge-ceph.sh"
  done
}


########
# MAIN #
########

test_run
purge_ceph
