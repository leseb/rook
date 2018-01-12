---
title: Multi-Node Test Environment
weight: 91
indent: true
---

# Rook development workflow

## Setup expectation

There are a bunch of pre-requisites to be able to deploy the following environment. Such as:

* A Linux workstation (CentOS or Fedora)
* KVM/QEMU installation

For other Linux distribution, there is no guarantee the following will work.
However adapting commands (apt/yum/dnf) could just work.

## Prerequisites installation

Execute `tests/scripts/multi-node/redhat-system-prerequisites.sh`.

## Deploy Kubernetes with Kubespray

In order to successfully deploy Kubernetes with Kubespray, you must have this code: https://github.com/kubernetes-incubator/kubespray/pull/2153 and https://github.com/kubernetes-incubator/kubespray/pull/2172.

Edit `inventory/group_vars/k8s-cluster.yml` with:

```bash
docker_options: "--insecure-registry=172.17.8.1:5000 --insecure-registry={{ kube_service_addresses }} --graph={{ docker_daemon_graph }}  {{ docker_log_opts }}"
```

FYI: `172.17.8.1` is the libvirt bridge IP, so it's reachable from all your virtual machines.
If you use k8s-01 as a registry, you should change it, either by a subnet of the IP of that machine in the `172.17.8.0/24` subnet.

And `inventory/group_vars/all.yml`:

```
etcd_peer_client_auth: false
```

Create Vagrant's variable directory:

```bash
mkdir vagrant/
```

Create `vagrant/config.rb` and use the content from : `tests/scripts/multi-node/config.rb`. You can adapt it at will.

Deploy!

```bash
vagrant up --no-provision ; vagrant provision
```

Go grab a coffee:

```
PLAY RECAP *********************************************************************
k8s-01                     : ok=351  changed=111  unreachable=0    failed=0
k8s-02                     : ok=230  changed=65   unreachable=0    failed=0
k8s-03                     : ok=230  changed=65   unreachable=0    failed=0
k8s-04                     : ok=229  changed=65   unreachable=0    failed=0
k8s-05                     : ok=229  changed=65   unreachable=0    failed=0
k8s-06                     : ok=229  changed=65   unreachable=0    failed=0
k8s-07                     : ok=229  changed=65   unreachable=0    failed=0
k8s-08                     : ok=229  changed=65   unreachable=0    failed=0
k8s-09                     : ok=229  changed=65   unreachable=0    failed=0

Friday 12 January 2018  10:25:45 +0100 (0:00:00.017)       0:17:24.413 ********
===============================================================================
download : container_download | Download containers if pull is required or told to always pull (all nodes) - 192.44s
kubernetes/preinstall : Update package management cache (YUM) --------- 178.26s
download : container_download | Download containers if pull is required or told to always pull (all nodes) - 102.24s
docker : ensure docker packages are installed -------------------------- 57.20s
download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 52.33s
kubernetes/preinstall : Install packages requirements ------------------ 25.18s
download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 23.74s
download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 18.90s
download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 15.39s
kubernetes/master : Master | wait for the apiserver to be running ------ 12.44s
download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 11.83s
download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 11.66s
kubernetes/node : install | Copy kubelet from hyperkube container ------ 11.44s
download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 11.41s
download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 11.00s
docker : Docker | pause while Docker restarts -------------------------- 10.22s
kubernetes/secrets : Check certs | check if a cert already exists on node --- 6.05s
kubernetes-apps/network_plugin/flannel : Flannel | Wait for flannel subnet.env file presence --- 5.33s
kubernetes/master : Master | wait for kube-scheduler -------------------- 5.30s
kubernetes/master : Copy kubectl from hyperkube container --------------- 4.77s
[leseb@tarox kubespray]$
[leseb@tarox kubespray]$
[leseb@tarox kubespray]$ vagrant ssh k8s-01
Last login: Fri Jan 12 09:22:18 2018 from 192.168.121.1

[vagrant@k8s-01 ~]$ kubectl get nodes
NAME      STATUS    ROLES         AGE       VERSION
k8s-01    Ready     master,node   2m        v1.9.0+coreos.0
k8s-02    Ready     node          2m        v1.9.0+coreos.0
k8s-03    Ready     node          2m        v1.9.0+coreos.0
k8s-04    Ready     node          2m        v1.9.0+coreos.0
k8s-05    Ready     node          2m        v1.9.0+coreos.0
k8s-06    Ready     node          2m        v1.9.0+coreos.0
k8s-07    Ready     node          2m        v1.9.0+coreos.0
k8s-08    Ready     node          2m        v1.9.0+coreos.0
k8s-09    Ready     node          2m        v1.9.0+coreos.0
```

## Deploy Rook:

Clone Rook:

```
[vagrant@k8s-01 ~]$ sudo yum install git -y
[vagrant@k8s-01 ~]$ git clone https://github.com/rook/rook
[vagrant@k8s-01 kubernetes]$ cd rook/cluster/examples/kubernetes/
```

Deploy the Rook operator:

```
[vagrant@k8s-01 kubernetes]$ kubectl create -f rook-operator.yaml
```

Wait a bit:

```
[vagrant@k8s-01 kubernetes]$ kubectl get pod -n rook-system
NAME                             READY     STATUS    RESTARTS   AGE
rook-agent-6jksm                 1/1       Running   0          6h
rook-agent-8shtx                 1/1       Running   0          6h
rook-agent-btmhs                 1/1       Running   0          6h
rook-agent-gx5rg                 1/1       Running   0          6h
rook-agent-jhmtj                 1/1       Running   4          6h
rook-agent-s4mkd                 1/1       Running   0          6h
rook-agent-vt765                 1/1       Running   0          6h
rook-agent-wd6hc                 1/1       Running   0          6h
rook-agent-z75ph                 1/1       Running   0          6h
rook-operator-77cf655476-svhpv   1/1       Running   0          6h
```

Deploy your first cluster:

```
[vagrant@k8s-01 kubernetes]$ kubectl create -f rook-cluster.yaml
```

Wait a bit:

```
[vagrant@k8s-01 kubernetes]$ kubectl get pods -o wide -n rook
NAME                             READY     STATUS    RESTARTS   AGE       IP              NODE
rook-api-848df956bf-bxnxx        1/1       Running   0          6h        10.233.68.5     k8s-03
rook-ceph-mgr0-cfccfd6b8-h7ks5   1/1       Running   0          6h        10.233.70.4     k8s-04
rook-ceph-mon0-cpscb             1/1       Running   2          6h        10.233.69.121   k8s-01
rook-ceph-mon1-br9vf             1/1       Running   0          6h        10.233.72.5     k8s-02
rook-ceph-mon2-bkkbj             1/1       Running   0          6h        10.233.68.4     k8s-03
rook-ceph-osd-24dpq              1/1       Running   0          6h        10.233.66.4     k8s-05
rook-ceph-osd-2dn2t              1/1       Running   2          6h        10.233.69.120   k8s-01
rook-ceph-osd-6tbwn              1/1       Running   0          6h        10.233.68.6     k8s-03
rook-ceph-osd-c76w2              1/1       Running   0          6h        10.233.72.6     k8s-02
rook-ceph-osd-cqq8h              1/1       Running   0          6h        10.233.67.6     k8s-08
rook-ceph-osd-fjbx9              1/1       Running   0          6h        10.233.70.5     k8s-04
rook-ceph-osd-k7hfc              1/1       Running   0          6h        10.233.65.4     k8s-09
rook-ceph-osd-n6kl8              1/1       Running   0          6h        10.233.64.4     k8s-06
rook-ceph-osd-xpqwx              1/1       Running   0          6h        10.233.71.4     k8s-07
```


## Development workflow

It is expected that the following steps will be run from the Kubernetes master node k8s-01.

Now, please refer to [https://rook.io/docs/rook/master/development-flow.html](https://rook.io/docs/rook/master/development-flow.html) to setup your development environment (go, git etc).

At this stage, Rook should be cloned on k8s-01.

From your Rook repository (should be $GOPATH/src/github.com/rook) location execute `bash tests/scripts/multi-node/build-rook.sh`.
During its execution `build-rook.sh` also purge your Rook pod so Rook container images can be build.
Now that Rook pods are gone with have to purge the Ceph data dir and disks.
For this, place `tests/scripts/multi-node/purge-ceph.sh` in your Kubespray git root directory. and execute it after `build-rook.sh`.

Deploy again, virtual machines (k8s-0X) will pull the new container image and run your new Rook code.
You can run `bash tests/scripts/multi-node/build-rook.sh` as many times as you want to rebuild your new rook image, then flush and re-deploy the operator to test your code.

From here, resume your dev, change your code and test it by running `bash tests/scripts/multi-node/build-rook.sh`.
