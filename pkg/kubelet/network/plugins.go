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

package network

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/kubelet/dockertools"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/errors"
	"github.com/golang/glog"
)

const DefaultPluginName = "no_op"

// Plugin is an interface to network plugins for the kubelet
type NetworkPlugin interface {
	// Validate tests the validity of the plugin. The implementation
	// should check whether any appropriate packages are installed
	//  correctly or not. etc.
	Validate() error

	// Init initializes the plugin.  This will be called exactly once
	// before any other methods are called.
	Init(host Host) error

	// Name returns the plugin's name. This will be used when searching
	// for a plugin by name, e.g.
	Name() string

	// SetUpPod is the method called after the infra container of
	// the pod has been created but before the other containers of the
	// pod are launched.
	SetUpPod(name string, namespace string, podInfraContainerID dockertools.DockerID) error

	// TearDownPod is the method called before a pod's infra container will be deleted
	TearDownPod(name string, namespace string, podInfraContainerID dockertools.DockerID) error
}

// Host is an interface that plugins can use to access the kubelet.
type Host interface {
	// Get the pod structure by its name, namespace
	GetPodByName(namespace, name string) (*api.Pod, bool)

	// GetKubeClient returns a client interface
	GetKubeClient() client.Interface
}

// InitNetworkPlugin validates all plugins, and inits the plugin that matches networkPluginName. Plugins must have unique names.
func InitNetworkPlugin(plugins []NetworkPlugin, networkPluginName string, host Host) (NetworkPlugin, error) {
	if networkPluginName == "" {
		// default to the no_op plugin
		plug := &noopNetworkPlugin{}
		plug.Init(host)
		return plug, nil
	}

	pluginMap := map[string]NetworkPlugin{}

	allErrs := []error{}
	for _, plugin := range plugins {
		name := plugin.Name()
		if !util.IsQualifiedName(name) {
			allErrs = append(allErrs, fmt.Errorf("network plugin has invalid name: %#v", plugin))
			continue
		}

		if _, found := pluginMap[name]; found {
			allErrs = append(allErrs, fmt.Errorf("network plugin %q was registered more than once", name))
			continue
		}
		pluginMap[name] = plugin
	}

	chosenPlugin := pluginMap[networkPluginName]
	if chosenPlugin != nil {
		err := chosenPlugin.Validate()
		if err != nil {
			allErrs = append(allErrs, fmt.Errorf("Network plugin %q failed validation: %v", networkPluginName, err))
		} else {
			glog.V(1).Infof("Loaded network plugin %q", networkPluginName)
			err = chosenPlugin.Init(host)
			if err != nil {
				allErrs = append(allErrs, err)
			}
		}
	} else {
		allErrs = append(allErrs, fmt.Errorf("Network plugin %q not found.", networkPluginName))
	}

	return chosenPlugin, errors.NewAggregate(allErrs)
}

type noopNetworkPlugin struct {
	host Host
}

func (plugin *noopNetworkPlugin) Init(host Host) error {
	plugin.host = host
	return nil
}

func (plugin *noopNetworkPlugin) Name() string {
	return DefaultPluginName
}

func (plugin *noopNetworkPlugin) Validate() error {
	return nil
}

func (plugin *noopNetworkPlugin) SetUpPod(name string, namespace string, id dockertools.DockerID) error {
	return nil
}

func (plugin *noopNetworkPlugin) TearDownPod(name string, namespace string, id dockertools.DockerID) error {
	return nil
}
