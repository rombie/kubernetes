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
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/types"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/volume"
	"github.com/golang/glog"
)

func ProbeVolumePlugins() []volume.VolumePlugin {
	return []volume.VolumePlugin{&persistentClaimPlugin{nil}}
}

type persistentClaimPlugin struct {
	host volume.VolumeHost
}

var _ volume.VolumePlugin = &persistentClaimPlugin{}

const (
	persistentClaimPluginName = "kubernetes.io/persistent-claim"
)

func (plugin *persistentClaimPlugin) Init(host volume.VolumeHost) {
	plugin.host = host
}

func (plugin *persistentClaimPlugin) Name() string {
	return persistentClaimPluginName
}

func (plugin *persistentClaimPlugin) CanSupport(spec *volume.Spec) bool {
	return spec.VolumeSource.PersistentVolumeClaimVolumeSource != nil
}

func (plugin *persistentClaimPlugin) NewBuilder(spec *volume.Spec, podRef *api.ObjectReference, opts volume.VolumeOptions) (volume.Builder, error) {
	claim, err := plugin.host.GetKubeClient().PersistentVolumeClaims(podRef.Namespace).Get(spec.VolumeSource.PersistentVolumeClaimVolumeSource.ClaimName)
	if err != nil {
		glog.Errorf("Error finding claim: %+v\n", spec.VolumeSource.PersistentVolumeClaimVolumeSource.ClaimName)
		return nil, err
	}

	pv, err := plugin.host.GetKubeClient().PersistentVolumes().Get(claim.Status.VolumeRef.Name)
	if err != nil {
		glog.Errorf("Error finding persistent volume for claim: %+v\n", claim.Name)
		return nil, err
	}

	builder, err := plugin.host.NewWrapperBuilder(volume.NewSpecFromPersistentVolume(pv), podRef, opts)
	if err != nil {
		glog.Errorf("Error creating builder for claim: %+v\n", claim.Name)
		return nil, err
	}

	return builder, nil
}

func (plugin *persistentClaimPlugin) NewCleaner(volName string, podUID types.UID) (volume.Cleaner, error) {
	return nil, fmt.Errorf("This will never be called directly. The PV backing this claim has a cleaner.  Kubelet uses that cleaner, not this one, when removing orphaned volumes.")
}
