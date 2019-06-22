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

package mon

import (
	"testing"

	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/stretchr/testify/assert"
)

func TestMonFlattening(t *testing.T) {

	// single endpoint
	mons := map[string]*cephconfig.MonInfo{
		"foo": {Name: "foo", Endpoint: "1.2.3.4:5000"},
	}
	flattened := FlattenMonEndpoints(mons)
	assert.Equal(t, "foo=1.2.3.4:5000", flattened)
	parsed := ParseMonEndpoints(flattened)
	assert.Equal(t, 1, len(parsed))
	assert.Equal(t, "foo", parsed["foo"].Name)
	assert.Equal(t, "1.2.3.4:5000", parsed["foo"].Endpoint)

	// multiple endpoints
	mons["bar"] = &cephconfig.MonInfo{Name: "bar", Endpoint: "2.3.4.5:6000"}
	flattened = FlattenMonEndpoints(mons)
	parsed = ParseMonEndpoints(flattened)
	assert.Equal(t, 2, len(parsed))
	assert.Equal(t, "foo", parsed["foo"].Name)
	assert.Equal(t, "1.2.3.4:5000", parsed["foo"].Endpoint)
	assert.Equal(t, "bar", parsed["bar"].Name)
	assert.Equal(t, "2.3.4.5:6000", parsed["bar"].Endpoint)
}

func TestUpdateCsiClusterConfig(t *testing.T) {
	// initialize an empty list & add a simple mons list
	mons := map[string]*cephconfig.MonInfo{
		"foo": {Name: "foo", Endpoint: "1.2.3.4:5000"},
	}
	s, err := UpdateCsiClusterConfig("[]", "alpha", mons)
	assert.NoError(t, err)
	assert.Equal(t, s,
		`[{"clusterID":"alpha","monitors":["1.2.3.4:5000"]}]`)

	// add a 2nd mon to the current cluster
	mons["bar"] = &cephconfig.MonInfo{
		Name: "bar", Endpoint: "10.11.12.13:5000"}
	s, err = UpdateCsiClusterConfig(s, "alpha", mons)
	assert.NoError(t, err)
	cc, err := parseCsiClusterConfig(s)
	assert.NoError(t, err)
	assert.Equal(t, len(cc), 1)
	assert.Equal(t, cc[0].ClusterID, "alpha")
	assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
	assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
	assert.Equal(t, len(cc[0].Monitors), 2)

	// add a 2nd cluster with 3 mons
	mons2 := map[string]*cephconfig.MonInfo{
		"flim": {Name: "flim", Endpoint: "20.1.1.1:5000"},
		"flam": {Name: "flam", Endpoint: "20.1.1.2:5000"},
		"blam": {Name: "blam", Endpoint: "20.1.1.3:5000"},
	}
	s, err = UpdateCsiClusterConfig(s, "beta", mons2)
	assert.NoError(t, err)
	cc, err = parseCsiClusterConfig(s)
	assert.NoError(t, err)
	assert.Equal(t, len(cc), 2)
	assert.Equal(t, cc[0].ClusterID, "alpha")
	assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
	assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
	assert.Equal(t, len(cc[0].Monitors), 2)
	assert.Equal(t, cc[1].ClusterID, "beta")
	assert.Contains(t, cc[1].Monitors, "20.1.1.1:5000")
	assert.Contains(t, cc[1].Monitors, "20.1.1.2:5000")
	assert.Contains(t, cc[1].Monitors, "20.1.1.3:5000")
	assert.Equal(t, len(cc[1].Monitors), 3)

	// remove a mon from the 2nd cluster
	delete(mons2, "blam")
	s, err = UpdateCsiClusterConfig(s, "beta", mons2)
	assert.NoError(t, err)
	cc, err = parseCsiClusterConfig(s)
	assert.NoError(t, err)
	assert.Equal(t, len(cc), 2)
	assert.Equal(t, cc[0].ClusterID, "alpha")
	assert.Contains(t, cc[0].Monitors, "1.2.3.4:5000")
	assert.Contains(t, cc[0].Monitors, "10.11.12.13:5000")
	assert.Equal(t, len(cc[0].Monitors), 2)
	assert.Equal(t, cc[1].ClusterID, "beta")
	assert.Contains(t, cc[1].Monitors, "20.1.1.1:5000")
	assert.Contains(t, cc[1].Monitors, "20.1.1.2:5000")
	assert.Equal(t, len(cc[1].Monitors), 2)

	// does it return error on garbage input?
	_, err = UpdateCsiClusterConfig("qqq", "beta", mons2)
	assert.Error(t, err)
}
