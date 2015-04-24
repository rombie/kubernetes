/*
Copyright 2014 Google Inc. All rights reserved.

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

package persistent_claim

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/latest"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/testclient"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/types"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/volume"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/volume/gce_pd"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/volume/host_path"
)

func newTestHost(t *testing.T, fakeKubeClient client.Interface) volume.VolumeHost {
	tempDir, err := ioutil.TempDir("/tmp", "persistent_volume_test.")
	if err != nil {
		t.Fatalf("can't make a temp rootdir: %v", err)
	}
	return volume.NewFakeVolumeHost(tempDir, fakeKubeClient, testProbeVolumePlugins())
}

func TestCanSupport(t *testing.T) {
	plugMgr := volume.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volume.NewFakeVolumeHost("/tmp/fake", nil, ProbeVolumePlugins()))

	plug, err := plugMgr.FindPluginByName("kubernetes.io/persistent-claim")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}
	if plug.Name() != "kubernetes.io/persistent-claim" {
		t.Errorf("Wrong name: %s", plug.Name())
	}
	if !plug.CanSupport(&volume.Spec{Name: "foo", VolumeSource: api.VolumeSource{PersistentVolumeClaimVolumeSource: &api.PersistentVolumeClaimVolumeSource{}}}) {
		t.Errorf("Expected true")
	}
	if plug.CanSupport(&volume.Spec{VolumeSource: api.VolumeSource{GitRepo: &api.GitRepoVolumeSource{}}}) {
		t.Errorf("Expected false")
	}
	if plug.CanSupport(&volume.Spec{VolumeSource: api.VolumeSource{}}) {
		t.Errorf("Expected false")
	}
}

func TestNewBuilder(t *testing.T) {
	tests := []struct {
		pv        *api.PersistentVolume
		claim     *api.PersistentVolumeClaim
		plugin    volume.VolumePlugin
		podVolume api.VolumeSource
		testFunc  func(builder volume.Builder, plugin volume.VolumePlugin) error
	}{
		{
			pv: &api.PersistentVolume{
				ObjectMeta: api.ObjectMeta{
					Name: "pvA",
				},
				Spec: api.PersistentVolumeSpec{
					PersistentVolumeSource: api.PersistentVolumeSource{
						GCEPersistentDisk: &api.GCEPersistentDiskVolumeSource{},
					},
					ClaimRef: &api.ObjectReference{
						Name: "claimA",
					},
				},
			},
			claim: &api.PersistentVolumeClaim{
				ObjectMeta: api.ObjectMeta{
					Name:      "claimA",
					Namespace: "nsA",
				},
				Status: api.PersistentVolumeClaimStatus{
					Phase: api.ClaimBound,
					VolumeRef: &api.ObjectReference{
						Name: "pvA",
					},
				},
			},
			podVolume: api.VolumeSource{
				PersistentVolumeClaimVolumeSource: &api.PersistentVolumeClaimVolumeSource{
					ReadOnly:  false,
					ClaimName: "claimA",
				},
			},
			plugin: gce_pd.ProbeVolumePlugins()[0],
			testFunc: func(builder volume.Builder, plugin volume.VolumePlugin) error {
				if !strings.Contains(builder.GetPath(), util.EscapeQualifiedNameForDisk(plugin.Name())) {
					return fmt.Errorf("builder path expected to contain plugin name.  Got: %s", builder.GetPath())
				}
				return nil
			},
		},
		{
			pv: &api.PersistentVolume{
				ObjectMeta: api.ObjectMeta{
					Name: "pvB",
				},
				Spec: api.PersistentVolumeSpec{
					PersistentVolumeSource: api.PersistentVolumeSource{
						HostPath: &api.HostPathVolumeSource{Path: "/tmp"},
					},
					ClaimRef: &api.ObjectReference{
						Name: "claimB",
					},
				},
			},
			claim: &api.PersistentVolumeClaim{
				ObjectMeta: api.ObjectMeta{
					Name:      "claimB",
					Namespace: "nsB",
				},
				Status: api.PersistentVolumeClaimStatus{
					VolumeRef: &api.ObjectReference{
						Name: "pvB",
					},
				},
			},
			podVolume: api.VolumeSource{
				PersistentVolumeClaimVolumeSource: &api.PersistentVolumeClaimVolumeSource{
					ReadOnly:  false,
					ClaimName: "claimB",
				},
			},
			plugin: host_path.ProbeVolumePlugins()[0],
			testFunc: func(builder volume.Builder, plugin volume.VolumePlugin) error {
				if builder.GetPath() != "/tmp" {
					return fmt.Errorf("Expected HostPath.Path /tmp, got: %s", builder.GetPath())
				}
				return nil
			},
		},
	}

	for _, item := range tests {
		o := testclient.NewObjects(api.Scheme)
		o.Add(item.pv)
		o.Add(item.claim)
		client := &testclient.Fake{ReactFn: testclient.ObjectReaction(o, latest.RESTMapper)}

		plugMgr := volume.VolumePluginMgr{}
		plugMgr.InitPlugins(testProbeVolumePlugins(), newTestHost(t, client))

		plug, err := plugMgr.FindPluginByName("kubernetes.io/persistent-claim")
		if err != nil {
			t.Errorf("Can't find the plugin by name")
		}
		spec := &volume.Spec{
			Name:         "vol1",
			VolumeSource: item.podVolume,
		}
		builder, err := plug.NewBuilder(spec, &api.ObjectReference{UID: types.UID("poduid")}, volume.VolumeOptions{})
		if err != nil {
			t.Errorf("Failed to make a new Builder: %v", err)
		}
		if builder == nil {
			t.Errorf("Got a nil Builder: %v", builder)
		}

		if builder != nil {
			if err := item.testFunc(builder, item.plugin); err != nil {
				t.Errorf("Unexpected error %+v", err)
			}
		}
	}
}

func testProbeVolumePlugins() []volume.VolumePlugin {
	allPlugins := []volume.VolumePlugin{}
	allPlugins = append(allPlugins, gce_pd.ProbeVolumePlugins()...)
	allPlugins = append(allPlugins, host_path.ProbeVolumePlugins()...)
	allPlugins = append(allPlugins, ProbeVolumePlugins()...)
	return allPlugins
}
