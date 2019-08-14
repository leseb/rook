/*
Copyright The Kubernetes Authors.

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

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"time"

	v1alpha1 "github.com/rook/rook/pkg/apis/yugabytedb.rook.io/v1alpha1"
	scheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// YBClustersGetter has a method to return a YBClusterInterface.
// A group's client should implement this interface.
type YBClustersGetter interface {
	YBClusters(namespace string) YBClusterInterface
}

// YBClusterInterface has methods to work with YBCluster resources.
type YBClusterInterface interface {
	Create(*v1alpha1.YBCluster) (*v1alpha1.YBCluster, error)
	Update(*v1alpha1.YBCluster) (*v1alpha1.YBCluster, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.YBCluster, error)
	List(opts v1.ListOptions) (*v1alpha1.YBClusterList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.YBCluster, err error)
	YBClusterExpansion
}

// yBClusters implements YBClusterInterface
type yBClusters struct {
	client rest.Interface
	ns     string
}

// newYBClusters returns a YBClusters
func newYBClusters(c *YugabytedbV1alpha1Client, namespace string) *yBClusters {
	return &yBClusters{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the yBCluster, and returns the corresponding yBCluster object, and an error if there is any.
func (c *yBClusters) Get(name string, options v1.GetOptions) (result *v1alpha1.YBCluster, err error) {
	result = &v1alpha1.YBCluster{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("ybclusters").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of YBClusters that match those selectors.
func (c *yBClusters) List(opts v1.ListOptions) (result *v1alpha1.YBClusterList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.YBClusterList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("ybclusters").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested yBClusters.
func (c *yBClusters) Watch(opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("ybclusters").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch()
}

// Create takes the representation of a yBCluster and creates it.  Returns the server's representation of the yBCluster, and an error, if there is any.
func (c *yBClusters) Create(yBCluster *v1alpha1.YBCluster) (result *v1alpha1.YBCluster, err error) {
	result = &v1alpha1.YBCluster{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("ybclusters").
		Body(yBCluster).
		Do().
		Into(result)
	return
}

// Update takes the representation of a yBCluster and updates it. Returns the server's representation of the yBCluster, and an error, if there is any.
func (c *yBClusters) Update(yBCluster *v1alpha1.YBCluster) (result *v1alpha1.YBCluster, err error) {
	result = &v1alpha1.YBCluster{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("ybclusters").
		Name(yBCluster.Name).
		Body(yBCluster).
		Do().
		Into(result)
	return
}

// Delete takes name of the yBCluster and deletes it. Returns an error if one occurs.
func (c *yBClusters) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("ybclusters").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *yBClusters) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("ybclusters").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched yBCluster.
func (c *yBClusters) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.YBCluster, err error) {
	result = &v1alpha1.YBCluster{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("ybclusters").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
