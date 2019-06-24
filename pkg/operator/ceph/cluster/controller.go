/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/nfs"
	"github.com/rook/rook/pkg/operator/ceph/object"
	objectuser "github.com/rook/rook/pkg/operator/ceph/object/user"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

const (
	crushConfigMapName       = "rook-crush-config"
	crushmapCreatedKey       = "initialCrushMapCreated"
	clusterCreateInterval    = 6 * time.Second
	clusterCreateTimeout     = 60 * time.Minute
	updateClusterInterval    = 30 * time.Second
	updateClusterTimeout     = 1 * time.Hour
	detectCephVersionTimeout = 15 * time.Minute
)

const (
	// DefaultClusterName states the default name of the rook-cluster if not provided.
	DefaultClusterName         = "rook-ceph"
	clusterDeleteRetryInterval = 2 //seconds
	clusterDeleteMaxRetries    = 15
	disableHotplugEnv          = "ROOK_DISABLE_DEVICE_HOTPLUG"
)

var (
	logger        = capnslog.NewPackageLogger("github.com/rook/rook", "op-cluster")
	finalizerName = fmt.Sprintf("%s.%s", ClusterResource.Name, ClusterResource.Group)
)

var ClusterResource = opkit.CustomResource{
	Name:    "cephcluster",
	Plural:  "cephclusters",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1.CephCluster{}).Name(),
}

// ClusterController controls an instance of a Rook cluster
type ClusterController struct {
	context            *clusterd.Context
	volumeAttachment   attachment.Attachment
	devicesInUse       bool
	rookImage          string
	clusterMap         map[string]*cluster
	addClusterCallback func(bool) error
	csiConfigMutex     *sync.Mutex
}

// NewClusterController create controller for watching cluster custom resources created
func NewClusterController(context *clusterd.Context, rookImage string, volumeAttachment attachment.Attachment, addClusterCallback func(bool) error) *ClusterController {
	return &ClusterController{
		context:            context,
		volumeAttachment:   volumeAttachment,
		rookImage:          rookImage,
		clusterMap:         make(map[string]*cluster),
		addClusterCallback: addClusterCallback,
		csiConfigMutex:     &sync.Mutex{},
	}
}

// Watch watches instances of cluster resources
func (c *ClusterController) StartWatch(namespace string, stopCh chan struct{}) error {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching clusters in all namespaces")
	watcher := opkit.NewWatcher(ClusterResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient())
	go watcher.Watch(&cephv1.CephCluster{}, stopCh)

	// watch for events on new/updated K8s nodes, too

	lwNodes := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return c.context.Clientset.CoreV1().Nodes().List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return c.context.Clientset.CoreV1().Nodes().Watch(options)
		},
	}

	_, nodeController := cache.NewInformer(
		lwNodes,
		&v1.Node{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onK8sNodeAdd,
			UpdateFunc: c.onK8sNodeUpdate,
			DeleteFunc: nil,
		},
	)
	go nodeController.Run(stopCh)

	if disableVal := os.Getenv(disableHotplugEnv); disableVal != "true" {
		// watch for updates to the device discovery configmap
		logger.Infof("Enabling hotplug orchestration: %s=%s", disableHotplugEnv, disableVal)
		operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
		_, deviceCMController := cache.NewInformer(
			cache.NewFilteredListWatchFromClient(c.context.Clientset.CoreV1().RESTClient(),
				"configmaps", operatorNamespace, func(options *metav1.ListOptions) {
					options.LabelSelector = fmt.Sprintf("%s=%s", k8sutil.AppAttr, discoverDaemon.AppName)
				},
			),
			&v1.ConfigMap{},
			0,
			cache.ResourceEventHandlerFuncs{
				AddFunc:    nil,
				UpdateFunc: c.onDeviceCMUpdate,
				DeleteFunc: nil,
			},
		)

		go deviceCMController.Run(stopCh)
	} else {
		logger.Infof("Disabling hotplug orchestration via %s", disableHotplugEnv)
	}

	return nil
}

func (c *ClusterController) StopWatch() {
	for _, cluster := range c.clusterMap {
		close(cluster.stopCh)
	}
	c.clusterMap = make(map[string]*cluster)
}

// ************************************************************************************************
// Add event functions
// ************************************************************************************************
func (c *ClusterController) onK8sNodeAdd(obj interface{}) {
	newNode, ok := obj.(*v1.Node)
	if !ok {
		logger.Warningf("Expected NodeList but handler received %#v", obj)
	}

	if k8sutil.GetNodeSchedulable(*newNode) == false {
		logger.Debugf("Skipping cluster update. Added node %s is unschedulable", newNode.Labels[v1.LabelHostname])
		return
	}

	for _, cluster := range c.clusterMap {
		if k8sutil.NodeIsTolerable(*newNode, cephv1.GetOSDPlacement(cluster.Spec.Placement).Tolerations, false) == false {
			logger.Debugf("Skipping -> Node is not tolerable for cluster %s", cluster.Namespace)
			continue
		}
		if cluster.Spec.Storage.UseAllNodes == false {
			logger.Debugf("Skipping -> Do not use all Nodes in cluster %s", cluster.Namespace)
			continue
		}
		if cluster.Info == nil {
			logger.Infof("Cluster %s is not ready. Skipping orchestration.", cluster.Namespace)
			continue
		}

		if valid, _ := k8sutil.ValidNode(*newNode, cluster.Spec.Placement.All()); valid == true {
			logger.Debugf("Adding %s to cluster %s", newNode.Labels[v1.LabelHostname], cluster.Namespace)
			err := cluster.createInstance(c.rookImage, cluster.Info.CephVersion)
			if err != nil {
				logger.Errorf("Failed to update cluster in namespace %s. Was not able to add %s. %+v", cluster.Namespace, newNode.Labels[v1.LabelHostname], err)
			}
		} else {
			logger.Infof("Could not add host %s . It is not valid", newNode.Labels[v1.LabelHostname])
			continue
		}
		logger.Infof("Added %s to cluster %s", newNode.Labels[v1.LabelHostname], cluster.Namespace)
	}
}

func (c *ClusterController) onAdd(obj interface{}) {
	clusterObj, err := getClusterObject(obj)
	if err != nil {
		logger.Errorf("failed to get cluster object: %+v", err)
		return
	}

	if existing, ok := c.clusterMap[clusterObj.Namespace]; ok {
		logger.Errorf("Failed to add cluster cr %s in namespace %s. Cluster cr %s already exists in this namespace. Only one cluster cr per namespace is supported.",
			clusterObj.Name, clusterObj.Namespace, existing.crdName)
		return
	}

	cluster := newCluster(clusterObj, c.context, c.csiConfigMutex)
	c.clusterMap[cluster.Namespace] = cluster

	logger.Infof("starting cluster in namespace %s", cluster.Namespace)

	// notify the callback that a cluster crd is being added
	if c.addClusterCallback != nil {
		if err := c.addClusterCallback(cluster.Spec.External.Enable); err != nil {
			logger.Errorf("%+v", err)
		}
	}

	c.initializeCluster(cluster, clusterObj)

}

func (c *ClusterController) configureExternalCephCluster(namespace, name string, cluster *cluster) error {
	c.updateClusterStatus(namespace, name, cephv1.ClusterStateConnecting, "")

	// loop until we find the secret necessary to connect to the external cluster
	for {
		var err error
		cluster.Info, _, _, err = mon.LoadClusterInfo(c.context, namespace)
		if err != nil {
			logger.Warningf("waiting for the connection info of the external cluster. %+v", err)
			time.Sleep(10 * time.Second)
			continue
		}
		logger.Infof("found the cluster info to connect to the external cluster. mons=%+v", cluster.Info.Monitors)

		// Let's write connection info (ceph config file and keyring) to the operator for health checks
		err = mon.WriteConnectionConfig(cluster.context, cluster.Info)
		if err != nil {
			return fmt.Errorf("failed to write connection info %+v", err)
		}

		// Get Ceph monitors version on the external cluster
		cephMonVersion, err := client.GetCephMonVersion(c.context, namespace)
		if err != nil {
			return fmt.Errorf("failed to get ceph mon version. %+v", err)
		}

		logger.Infof("detecting the image version provided for the external cluster...")
		specCephVersionImage, err := cluster.detectCephVersion(c.rookImage, cluster.Spec.CephVersion.Image, detectCephVersionTimeout)
		if err != nil {
			return fmt.Errorf("unknown ceph major version. %+v", err)
		}
		specCephVersion := *specCephVersionImage

		// Populate clusterInfo with the external cluster version
		cluster.Info.CephVersion = *cephMonVersion

		// Make sure the external cluster version and the ceph version in the provided image are identical
		if !cephver.IsIdentical(specCephVersion, cluster.Info.CephVersion) {
			return fmt.Errorf("wrong ceph version %s, external cluster version is %s, they must match", specCephVersion.String(), cephMonVersion.String())
		}

		// Everything went well so let's update the CR's status to "connected"
		c.updateClusterStatus(namespace, name, cephv1.ClusterStateConnected, "")

		return nil
	}
}

func (c *ClusterController) configureLocalCephCluster(namespace, name string, cluster *cluster, clusterObj *cephv1.CephCluster) error {
	if c.devicesInUse && cluster.Spec.Storage.AnyUseAllDevices() {
		c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, cephv1.ClusterStateError, "using all devices in more than one namespace is not supported")
		return fmt.Errorf("using all devices in more than one namespace is not supported")
	}

	if cluster.Spec.Storage.AnyUseAllDevices() {
		c.devicesInUse = true
	}

	return nil
}

func (c *ClusterController) initializeCluster(cluster *cluster, clusterObj *cephv1.CephCluster) {
	cluster.Spec = &clusterObj.Spec
	if cluster.Spec.Mon.Count == 0 {
		logger.Warningf("mon count should be at least 1, will use default value of %d", mon.DefaultMonCount)
		cluster.Spec.Mon.Count = mon.DefaultMonCount
	}
	if cluster.Spec.Mon.Count%2 == 0 {
		logger.Warningf("mon count is even (given: %d), should be uneven, continuing", cluster.Spec.Mon.Count)
	}

	// Start the Rook cluster components. Retry several times in case of failure.
	failedMessage := ""
	state := cephv1.ClusterStateError

	// Try to load clusterInfo early so we can compare the running version with the one from the spec image
	var err error
	cluster.Info, _, _, err = mon.LoadClusterInfo(c.context, cluster.Namespace)
	if err == nil {
		// Let's write connection info (ceph config file and keyring) to the operator for health checks
		err = mon.WriteConnectionConfig(cluster.context, cluster.Info)
		if err != nil {
			return
		}
	}

	err = wait.Poll(clusterCreateInterval, clusterCreateTimeout,
		func() (bool, error) {
			cephVersion, canRetry, err := c.detectAndValidateCephVersion(cluster, cluster.Spec.CephVersion.Image)
			if err != nil {
				failedMessage = fmt.Sprintf("failed the ceph version check. %+v", err)
				logger.Errorf(failedMessage)
				if !canRetry {
					// it may seem strange to exit true but we don't want to retry if the version is not supported
					return true, nil
				}
				return false, nil
			}

			c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, cephv1.ClusterStateCreating, "")

			err = cluster.createInstance(c.rookImage, *cephVersion)
			if err != nil {
				failedMessage = fmt.Sprintf("failed to create cluster in namespace %s. %+v", cluster.Namespace, err)
				logger.Errorf(failedMessage)
				return false, nil
			}

			state = cephv1.ClusterStateCreated
			failedMessage = ""
			return true, nil
		})

	c.updateClusterStatus(clusterObj.Namespace, clusterObj.Name, state, failedMessage)

	if state == cephv1.ClusterStateError {
		// the cluster could not be initialized
		return
	}

	if !cluster.Spec.External.Enable {
		if err := c.configureLocalCephCluster(clusterObj.Namespace, clusterObj.Name, cluster, clusterObj); err != nil {
			logger.Errorf("failed to configure local ceph cluster. %+v", err)
			return
		}
	} else {
		if err := c.configureExternalCephCluster(clusterObj.Namespace, clusterObj.Name, cluster); err != nil {
			logger.Errorf("failed to configure external ceph cluster. %+v", err)
			return
		}
	}

	// Start pool CRD watcher
	poolController := pool.NewPoolController(c.context, cluster.Spec)
	poolController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Start object store CRD watcher
	objectStoreController := object.NewObjectStoreController(cluster.Info, c.context, cluster.Namespace, c.rookImage, cluster.Spec, cluster.ownerRef, cluster.Spec.DataDirHostPath)
	objectStoreController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Start object store user CRD watcher
	objectStoreUserController := objectuser.NewObjectStoreUserController(c.context, cluster.Spec, cluster.Namespace, cluster.ownerRef)
	objectStoreUserController.StartWatch(cluster.stopCh)

	// Start file system CRD watcher
	fileController := file.NewFilesystemController(cluster.Info, c.context, cluster.Namespace, c.rookImage, cluster.Spec, cluster.ownerRef, cluster.Spec.DataDirHostPath)
	fileController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Start nfs ganesha CRD watcher
	ganeshaController := nfs.NewCephNFSController(cluster.Info, c.context, cluster.Spec.DataDirHostPath, cluster.Namespace, c.rookImage, cluster.Spec, cluster.ownerRef)
	ganeshaController.StartWatch(cluster.Namespace, cluster.stopCh)

	// Populate ClusterInfo
	if cluster.Spec.External.Enable {
		cluster.mons.ClusterInfo = cluster.Info
	}

	// Start mon health checker
	healthChecker := mon.NewHealthChecker(cluster.mons, cluster.Spec)
	go healthChecker.Check(cluster.stopCh)

	if !cluster.Spec.External.Enable {
		// Start the osd health checker only if running OSDs in the local ceph cluster
		osdChecker := osd.NewMonitor(c.context, cluster.Namespace)
		go osdChecker.Start(cluster.stopCh)
	}

	// Start the ceph status checker
	cephChecker := newCephStatusChecker(c.context, cluster.Namespace, clusterObj.Name)
	go cephChecker.checkCephStatus(cluster.stopCh)

	// add the finalizer to the crd
	err = c.addFinalizer(clusterObj.Namespace, clusterObj.Name)
	if err != nil {
		logger.Errorf("failed to add finalizer to cluster crd. %+v", err)
	}
}

// ************************************************************************************************
// Update event functions
// ************************************************************************************************
func (c *ClusterController) onK8sNodeUpdate(oldObj, newObj interface{}) {
	// skip forced resyncs
	if reflect.DeepEqual(oldObj, newObj) {
		return
	}

	// Checking for nodes where NoSchedule-Taint got removed
	newNode, ok := newObj.(*v1.Node)
	if !ok {
		logger.Warningf("Expected Node but handler received %#v", newObj)
		return
	}

	oldNode, ok := oldObj.(*v1.Node)
	if !ok {
		logger.Warningf("Expected Node but handler received %#v", oldObj)
		return
	}

	// set or unset noout depending on whether nodes are schedulable.
	newNodeSchedulable := k8sutil.GetNodeSchedulable(*newNode)
	oldNodeSchedulable := k8sutil.GetNodeSchedulable(*oldNode)

	// Checking for NoSchedule added to storage node
	if oldNodeSchedulable == false && newNodeSchedulable == false {
		logger.Debugf("Skipping cluster update. Updated node %s was and is still unschedulable", newNode.Labels[v1.LabelHostname])
		return
	}
	if oldNodeSchedulable == true && newNodeSchedulable == true {
		logger.Debugf("Skipping cluster update. Updated node %s was and it is still schedulable", oldNode.Labels[v1.LabelHostname])
		return
	}

	for _, cluster := range c.clusterMap {
		if cluster.Info == nil {
			logger.Infof("Cluster %s is not ready. Skipping orchestration.", cluster.Namespace)
			continue
		}
		if valid, _ := k8sutil.ValidNode(*newNode, cephv1.GetOSDPlacement(cluster.Spec.Placement)); valid == true {
			logger.Debugf("Adding %s to cluster %s", newNode.Labels[v1.LabelHostname], cluster.Namespace)
			err := cluster.createInstance(c.rookImage, cluster.Info.CephVersion)
			if err != nil {
				logger.Errorf("Failed adding the updated node %s to cluster in namespace %s. %+v", newNode.Labels[v1.LabelHostname], cluster.Namespace, err)
				continue
			}
		} else {
			logger.Infof("Updated node %s is not valid and could not get added to cluster in namespace %s.", newNode.Labels[v1.LabelHostname], cluster.Namespace)
			continue
		}
		logger.Infof("Added updated node %s to cluster %s", newNode.Labels[v1.LabelHostname], cluster.Namespace)
	}
}

func (c *ClusterController) onUpdate(oldObj, newObj interface{}) {
	oldClust, err := getClusterObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old cluster object: %+v", err)
		return
	}
	newClust, err := getClusterObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new cluster object: %+v", err)
		return
	}

	logger.Debugf("update event for cluster %s", newClust.Namespace)

	if existing, ok := c.clusterMap[newClust.Namespace]; ok && existing.crdName != newClust.Name {
		logger.Errorf("Skipping update of cluster cr %s in namespace %s. Cluster cr %s already exists in this namespace. Only one cluster cr per namespace is supported.",
			newClust.Name, newClust.Namespace, existing.crdName)
		return
	}

	// Check if the cluster is being deleted. This code path is called when a finalizer is specified in the crd.
	// When a cluster is requested for deletion, K8s will only set the deletion timestamp if there are any finalizers in the list.
	// K8s will only delete the crd and child resources when the finalizers have been removed from the crd.
	if newClust.DeletionTimestamp != nil {
		logger.Infof("cluster %s has a deletion timestamp", newClust.Namespace)
		err := c.handleDelete(newClust, time.Duration(clusterDeleteRetryInterval)*time.Second)
		if err != nil {
			logger.Errorf("failed finalizer for cluster. %+v", err)
			return
		}
		// remove the finalizer from the crd, which indicates to k8s that the resource can safely be deleted
		c.removeFinalizer(newClust)
		return
	}
	cluster, ok := c.clusterMap[newClust.Namespace]
	if !ok {
		logger.Errorf("Cannot update cluster %s that does not exist", newClust.Namespace)
		return
	}

	// If the cluster was never initialized during the OnAdd() method due to a failure, we must
	// treat the cluster as if it was just created.
	if !cluster.initialized() {
		logger.Infof("Update event for uninitialized cluster %s. Initializing...", newClust.Namespace)
		c.initializeCluster(cluster, newClust)
		return
	}

	changed, _ := clusterChanged(oldClust.Spec, newClust.Spec, cluster)
	if !changed {
		logger.Debugf("update event for cluster %s is not supported", newClust.Namespace)
		return
	}

	logger.Infof("update event for cluster %s is supported, orchestrating update now", newClust.Namespace)

	// if the image changed, we need to detect the new image version
	versionChanged := false
	if oldClust.Spec.CephVersion.Image != newClust.Spec.CephVersion.Image {
		logger.Infof("the ceph version changed")
		version, _, err := c.detectAndValidateCephVersion(cluster, newClust.Spec.CephVersion.Image)
		if err != nil {
			logger.Errorf("unknown ceph major version. %+v", err)
			return
		}
		cluster.Info.CephVersion = *version
	}

	logger.Debugf("old cluster: %+v", oldClust.Spec)
	logger.Debugf("new cluster: %+v", newClust.Spec)

	cluster.Spec = &newClust.Spec

	// Get cluster running versions
	versions, err := client.GetAllCephDaemonVersions(c.context, cluster.Namespace)
	if err != nil {
		logger.Errorf("failed to get ceph daemons versions. %+v", err)
		return
	}
	runningVersions := *versions

	// If the image version changed let's make sure we can safely upgrade
	if versionChanged {
		updateOrNot, err := diffImageSpecAndClusterRunningVersion(cluster.Info.CephVersion, runningVersions)
		if err != nil {
			logger.Errorf("failed to determine if we should upgrade or not. %+v", err)
			return
		}

		if updateOrNot {
			// If the image version changed let's make sure we can safely upgrade
			// check ceph's status, if not healthy we fail
			cephStatus := client.IsCephHealthy(c.context, cluster.Namespace)
			if !cephStatus {
				logger.Errorf("ceph status in namespace %s is not healthy, refusing to upgrade. fix the cluster and re-edit the cluster CR to trigger a new orchestation update", cluster.Namespace)
				return
			}
		}
	} else {
		logger.Infof("ceph daemons running versions are: %+v", runningVersions)
	}

	// attempt to update the cluster.  note this is done outside of wait.Poll because that function
	// will wait for the retry interval before trying for the first time.
	done, _ := c.handleUpdate(newClust.Name, cluster)
	if done {
		return
	}

	err = wait.Poll(updateClusterInterval, updateClusterTimeout, func() (bool, error) {
		return c.handleUpdate(newClust.Name, cluster)
	})
	if err != nil {
		c.updateClusterStatus(newClust.Namespace, newClust.Name, cephv1.ClusterStateError,
			fmt.Sprintf("giving up trying to update cluster in namespace %s after %s. %+v", cluster.Namespace, updateClusterTimeout, err))
		return
	}

	// Display success after upgrade
	if versionChanged {
		printOverallCephVersion(c.context, cluster.Namespace)
		// TODO: Update all the crd controllers that there is a new version of the ceph image to deploy
	}
}

func (c *ClusterController) detectAndValidateCephVersion(cluster *cluster, image string) (*cephver.CephVersion, bool, error) {
	version, err := cluster.detectCephVersion(c.rookImage, image, detectCephVersionTimeout)
	if err != nil {
		return nil, true, err
	}
	if err := cluster.validateCephVersion(version); err != nil {
		return nil, false, err
	}
	return version, false, nil
}

func (c *ClusterController) handleUpdate(crdName string, cluster *cluster) (bool, error) {
	c.updateClusterStatus(cluster.Namespace, crdName, cephv1.ClusterStateUpdating, "")

	if err := cluster.createInstance(c.rookImage, cluster.Info.CephVersion); err != nil {
		logger.Errorf("failed to update cluster in namespace %s. %+v", cluster.Namespace, err)
		return false, nil
	}

	c.updateClusterStatus(cluster.Namespace, crdName, cephv1.ClusterStateCreated, "")

	logger.Infof("succeeded updating cluster in namespace %s", cluster.Namespace)
	return true, nil
}

func (c *ClusterController) onDeviceCMUpdate(oldObj, newObj interface{}) {
	oldCm, ok := oldObj.(*v1.ConfigMap)
	if !ok {
		logger.Warningf("Expected ConfigMap but handler received %#v", oldObj)
		return
	}
	logger.Debugf("onDeviceCMUpdate old device cm: %+v", oldCm)

	newCm, ok := newObj.(*v1.ConfigMap)
	if !ok {
		logger.Warningf("Expected ConfigMap but handler received %#v", newObj)
		return
	}
	logger.Debugf("onDeviceCMUpdate new device cm: %+v", newCm)

	oldDevStr, ok := oldCm.Data[discoverDaemon.LocalDiskCMData]
	if !ok {
		logger.Warningf("unexpected configmap data")
		return
	}

	newDevStr, ok := newCm.Data[discoverDaemon.LocalDiskCMData]
	if !ok {
		logger.Warningf("unexpected configmap data")
		return
	}

	devicesEqual, err := discoverDaemon.DeviceListsEqual(oldDevStr, newDevStr)
	if err != nil {
		logger.Warningf("failed to compare device lists: %v", err)
		return
	}

	if devicesEqual {
		logger.Debugf("device lists are equal. skipping orchestration")
		return
	}

	for _, cluster := range c.clusterMap {
		if cluster.Info == nil {
			logger.Info("Cluster %s is not ready. Skipping orchestration on device change", cluster.Namespace)
			continue
		}
		logger.Infof("Running orchestration for namespace %s after device change", cluster.Namespace)
		err := cluster.createInstance(c.rookImage, cluster.Info.CephVersion)
		if err != nil {
			logger.Errorf("Failed orchestration after device change in namespace %s. %+v", cluster.Namespace, err)
			continue
		}
	}
}

// ************************************************************************************************
// Delete event functions
// ************************************************************************************************
func (c *ClusterController) onDelete(obj interface{}) {
	clust, err := getClusterObject(obj)
	if err != nil {
		logger.Errorf("failed to get cluster object: %+v", err)
		return
	}

	if existing, ok := c.clusterMap[clust.Namespace]; ok && existing.crdName != clust.Name {
		logger.Errorf("Skipping deletion of cluster cr %s in namespace %s. Cluster cr %s already exists in this namespace. Only one cluster cr per namespace is supported.",
			clust.Name, clust.Namespace, existing.crdName)
		return
	}

	logger.Infof("delete event for cluster %s in namespace %s", clust.Name, clust.Namespace)

	err = c.handleDelete(clust, time.Duration(clusterDeleteRetryInterval)*time.Second)
	if err != nil {
		logger.Errorf("failed to delete cluster. %+v", err)
	}
	if cluster, ok := c.clusterMap[clust.Namespace]; ok {
		close(cluster.stopCh)
		delete(c.clusterMap, clust.Namespace)
	}
	if clust.Spec.Storage.AnyUseAllDevices() {
		c.devicesInUse = false
	}
}

func (c *ClusterController) handleDelete(cluster *cephv1.CephCluster, retryInterval time.Duration) error {

	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	retryCount := 0
	for {
		// TODO: filter this List operation by cluster namespace on the server side
		vols, err := c.volumeAttachment.List(operatorNamespace)
		if err != nil {
			return fmt.Errorf("failed to get volume attachments for operator namespace %s: %+v", operatorNamespace, err)
		}

		// find volume attachments in the deleted cluster
		attachmentsExist := false
	AttachmentLoop:
		for _, vol := range vols.Items {
			for _, a := range vol.Attachments {
				if a.ClusterName == cluster.Namespace {
					// there is still an outstanding volume attachment in the cluster that is being deleted.
					attachmentsExist = true
					break AttachmentLoop
				}
			}
		}

		if !attachmentsExist {
			logger.Infof("no volume attachments for cluster %s to clean up.", cluster.Namespace)
			break
		}

		retryCount++
		if retryCount == clusterDeleteMaxRetries {
			logger.Warningf(
				"exceeded retry count while waiting for volume attachments for cluster %s to be cleaned up. vols: %+v",
				cluster.Namespace,
				vols.Items)
			break
		}

		logger.Infof("waiting for volume attachments in cluster %s to be cleaned up. Retrying in %s.",
			cluster.Namespace, retryInterval)
		<-time.After(retryInterval)
	}

	return nil
}

// ************************************************************************************************
// Finalizer functions
// ************************************************************************************************
func (c *ClusterController) addFinalizer(namespace, name string) error {

	// get the latest cluster object since we probably updated it before we got to this point (e.g. by updating its status)
	clust, err := c.context.RookClientset.CephV1().CephClusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// add the finalizer (cephcluster.ceph.rook.io) if it is not yet defined on the cluster CRD
	for _, finalizer := range clust.Finalizers {
		if finalizer == finalizerName {
			logger.Infof("finalizer already set on cluster %s", clust.Namespace)
			return nil
		}
	}

	// adding finalizer to the cluster crd
	clust.Finalizers = append(clust.Finalizers, finalizerName)

	// update the crd
	_, err = c.context.RookClientset.CephV1().CephClusters(clust.Namespace).Update(clust)
	if err != nil {
		return fmt.Errorf("failed to add finalizer to cluster. %+v", err)
	}

	logger.Infof("added finalizer to cluster %s", clust.Name)
	return nil
}

func (c *ClusterController) removeFinalizer(obj interface{}) {
	var fname string
	var objectMeta *metav1.ObjectMeta

	// first determine what type/version of cluster we are dealing with
	if cl, ok := obj.(*cephv1.CephCluster); ok {
		fname = finalizerName
		objectMeta = &cl.ObjectMeta
	} else {
		logger.Warningf("cannot remove finalizer from object that is not a cluster: %+v", obj)
		return
	}

	// remove the finalizer from the slice if it exists
	found := false
	for i, finalizer := range objectMeta.Finalizers {
		if finalizer == fname {
			objectMeta.Finalizers = append(objectMeta.Finalizers[:i], objectMeta.Finalizers[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		logger.Infof("finalizer %s not found in the cluster crd '%s'", fname, objectMeta.Name)
		return
	}

	// update the crd to remove the finalizer for good. retry several times in case of intermittent failures.
	maxRetries := 5
	retrySeconds := 5 * time.Second
	for i := 0; i < maxRetries; i++ {
		var err error
		if cluster, ok := obj.(*cephv1.CephCluster); ok {
			_, err = c.context.RookClientset.CephV1().CephClusters(cluster.Namespace).Update(cluster)
		}

		if err != nil {
			logger.Errorf("failed to remove finalizer %s from cluster %s. %+v", fname, objectMeta.Name, err)
			time.Sleep(retrySeconds)
			continue
		}
		logger.Infof("removed finalizer %s from cluster %s", fname, objectMeta.Name)
		return
	}

	logger.Warningf("giving up from removing the %s cluster finalizer", fname)
}

// updateClusterStatus updates the status of the cluster custom resource, whether it is being updated or is completed
func (c *ClusterController) updateClusterStatus(namespace, name string, state cephv1.ClusterState, message string) {
	logger.Infof("CephCluster %s status: %s. %s", namespace, state, message)

	// get the most recent cluster CRD object
	cluster, err := c.context.RookClientset.CephV1().CephClusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to get cluster from namespace %s prior to updating its status to %s. %+v", namespace, state, err)
	}

	// update the status on the retrieved cluster object
	// do not overwrite the ceph status that is updated in a separate goroutine
	cluster.Status.State = state
	cluster.Status.Message = message
	if _, err := c.context.RookClientset.CephV1().CephClusters(namespace).Update(cluster); err != nil {
		logger.Errorf("failed to update cluster %s status: %+v", namespace, err)
	}
}

func ClusterOwnerRef(clusterName, clusterID string) metav1.OwnerReference {
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         fmt.Sprintf("%s/%s", ClusterResource.Group, ClusterResource.Version),
		Kind:               ClusterResource.Kind,
		Name:               clusterName,
		UID:                types.UID(clusterID),
		BlockOwnerDeletion: &blockOwner,
	}
}

func printOverallCephVersion(context *clusterd.Context, namespace string) {
	versions, err := client.GetAllCephDaemonVersions(context, namespace)
	if err != nil {
		logger.Errorf("failed to get ceph daemons versions. %+v", err)
		return
	}

	if len(versions.Overall) == 1 {
		for v := range versions.Overall {
			version, err := cephver.ExtractCephVersion(v)
			if err != nil {
				logger.Errorf("failed to extract ceph version. %+v", err)
				return
			}
			vv := *version
			logger.Infof("successfully upgraded cluster to version: %s", vv.String())
		}
	} else {
		// This shouldn't happen, but let's log just in case
		logger.Warningf("upgrade orchestration completed but somehow we still have more than one Ceph version running. %+v:", versions.Overall)
	}
}
