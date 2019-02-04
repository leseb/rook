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
package clusterd

import (
	"testing"

	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
)

const (
	cephVolumeInventoryOutput = `[{"available": true, "rejected_reasons": [], "sys_api": {"scheduler_mode": "mq-deadline", "rotational": "0", "vendor": "", "human_readable_size": "1024.00 MB", "sectors": 0, "sas_device_handle": "", "partitions": {}, "rev": "", "sas_address": "", "locked": 0, "sectorsize": "512", "removable": "0", "path": "/dev/rbd0", "support_discard": "", "model": "", "ro": "0", "nr_requests": "256", "size": 1073741824.0}, "lvs": [], "path": "/dev/rbd0"}, {"available": true, "rejected_reasons": [], "sys_api": {"scheduler_mode": "deadline", "rotational": "1", "vendor": "ATA", "human_readable_size": "50.00 GB", "sectors": 0, "sas_device_handle": "", "partitions": {}, "rev": "2.5+", "sas_address": "", "locked": 0, "sectorsize": "512", "removable": "0", "path": "/dev/sda", "support_discard": "", "model": "QEMU HARDDISK", "ro": "0", "nr_requests": "128", "size": 53687091200.0}, "lvs": [], "path": "/dev/sda"}, {"available": false, "rejected_reasons": ["locked"], "sys_api": {"scheduler_mode": "deadline", "rotational": "1", "vendor": "ATA", "human_readable_size": "50.00 GB", "sectors": 0, "sas_device_handle": "", "partitions": {}, "rev": "2.5+", "sas_address": "", "locked": 1, "sectorsize": "512", "removable": "0", "path": "/dev/sdb", "support_discard": "", "model": "QEMU HARDDISK", "ro": "0", "nr_requests": "128", "size": 53687091200.0}, "lvs": [{"cluster_name": "ceph", "name": "data-lv1", "osd_id": "0", "cluster_fsid": "7e264fe8-902a-4cfb-ad76-8930806a2846", "type": "block", "block_uuid": "EEmPso-KKaa-9xHP-XQcJ-8Tna-d4nB-hxmHpC", "osd_fsid": "4a3e79bd-c168-4f4f-bf50-db11a49031d9"}, {"cluster_name": "ceph", "name": "data-lv2", "osd_id": "1", "cluster_fsid": "7e264fe8-902a-4cfb-ad76-8930806a2846", "type": "block", "block_uuid": "Yk7faD-OWxp-MxEm-4OeT-elsP-1yZ1-J2dCGK", "osd_fsid": "4020a077-80a8-413e-805d-4c6086caca8f"}], "path": "/dev/sdb"}, {"available": false, "rejected_reasons": ["locked"], "sys_api": {"scheduler_mode": "deadline", "rotational": "1", "vendor": "ATA", "human_readable_size": "50.00 GB", "sectors": 0, "sas_device_handle": "", "partitions": {"sdc1": {"start": "2048", "holders": [], "sectorsize": 512, "sectors": "52426752", "size": "25.00 GB"}, "sdc2": {"start": "52428800", "holders": ["dm-3"], "sectorsize": 512, "sectors": "52426752", "size": "25.00 GB"}}, "rev": "2.5+", "sas_address": "", "locked": 1, "sectorsize": "512", "removable": "0", "path": "/dev/sdc", "support_discard": "", "model": "QEMU HARDDISK", "ro": "0", "nr_requests": "128", "size": 53687091200.0}, "lvs": [{"cluster_name": "ceph", "name": "journal1", "osd_id": "1", "cluster_fsid": "7e264fe8-902a-4cfb-ad76-8930806a2846", "db_uuid": "kztB84-BE0F-zA0S-1Yp8-aRRy-8jVL-0qy2Kp", "type": "db", "osd_fsid": "4020a077-80a8-413e-805d-4c6086caca8f"}], "path": "/dev/sdc"}, {"available": false, "rejected_reasons": ["locked"], "sys_api": {"scheduler_mode": "mq-deadline", "rotational": "1", "vendor": "0x1af4", "human_readable_size": "11.00 GB", "sectors": 0, "sas_device_handle": "", "partitions": {"vda1": {"start": "2048", "holders": [], "sectorsize": 512, "sectors": "614400", "size": "300.00 MB"}, "vda2": {"start": "616448", "holders": ["dm-0"], "sectorsize": 512, "sectors": "20355072", "size": "9.71 GB"}}, "rev": "", "sas_address": "", "locked": 1, "sectorsize": "512", "removable": "0", "path": "/dev/vda", "support_discard": "", "model": "", "ro": "0", "nr_requests": "256", "size": 11811160064.0}, "lvs": [{"comment": "not used by ceph", "name": "root"}], "path": "/dev/vda"}]
`
)

func TestAvailableDisks(t *testing.T) {

	// no disks discovered for a node is an error
	disks := GetAvailableDevices([]*sys.LocalDisk{})
	assert.Equal(t, 0, len(disks))

	// no available disks because of the formatting
	d1 := &sys.LocalDisk{Name: "sda", UUID: "myuuid1", Size: 123, Rotational: true, Readonly: false, Filesystem: "btrfs", Type: sys.DiskType, HasChildren: true}
	disks = GetAvailableDevices([]*sys.LocalDisk{d1})
	assert.Equal(t, 0, len(disks))

	// multiple available disks
	d2 := &sys.LocalDisk{Name: "sdb", UUID: "myuuid2", Size: 123, Rotational: true, Readonly: false, Type: sys.DiskType, HasChildren: true}
	d3 := &sys.LocalDisk{Name: "sdc", UUID: "myuuid3", Size: 123, Rotational: true, Readonly: false, Type: sys.DiskType, HasChildren: true}
	disks = GetAvailableDevices([]*sys.LocalDisk{d1, d2, d3})

	assert.Equal(t, 2, len(disks))
	assert.Equal(t, "sdb", disks[0])
	assert.Equal(t, "sdc", disks[1])

	// partitions don't result in more available devices
	d4 := &sys.LocalDisk{Name: "sdb1", UUID: "myuuid4", Size: 123, Rotational: true, Readonly: false, Type: sys.PartType, HasChildren: true}
	d5 := &sys.LocalDisk{Name: "sdb2", UUID: "myuuid5", Size: 123, Rotational: true, Readonly: false, Type: sys.PartType, HasChildren: true}
	disks = GetAvailableDevices([]*sys.LocalDisk{d1, d2, d3, d4, d5})
	assert.Equal(t, 2, len(disks))
	assert.Equal(t, "sdb", disks[0])
	assert.Equal(t, "sdc", disks[1])

	// Crypt disk type results in available disk
	d6 := &sys.LocalDisk{Name: "sdd", UUID: "myuuid2", Size: 123, Rotational: true, Readonly: false, Type: sys.CryptType, HasChildren: true}
	disks = GetAvailableDevices([]*sys.LocalDisk{d6})
	assert.Equal(t, 1, len(disks))

}

func TestDiscoverDevices(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommand: func(debug bool, actionName string, command string, arg ...string) error {
			logger.Infof("mock execute. %s. %s", actionName, command)
			return nil
		},
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, arg ...string) (string, error) {
			logger.Infof("mock execute with output. %s. %s", actionName, command)
			return "", nil
		},
	}
	devices, err := DiscoverDevices(executor)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(devices))
}

func TestIgnoreDevice(t *testing.T) {
	cases := map[string]bool{
		"rbd0":    true,
		"rbd2":    true,
		"rbd9913": true,
		"rbd32p1": true,
		"rbd0a2":  false,
		"rbd":     false,
		"arbd0":   false,
		"rbd0x":   false,
	}
	for dev, expected := range cases {
		assert.Equal(t, expected, ignoreDevice(dev), dev)
	}
}

func TestGetAvailableDevicesCephVolume(t *testing.T) {
	fakeExecutor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, arg ...string) (string, error) {
			return cephVolumeInventoryOutput, nil
		},
	}

	expectedAvailableDevices := []string{"/dev/rbd0", "/dev/sda"}

	d, err := GetAvailableDevicesCephVolume(fakeExecutor)
	assert.Nil(t, err)
	assert.Equal(t, expectedDevList, d)
}
