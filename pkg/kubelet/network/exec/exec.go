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

// Package exec scans and loads networking plugins that are installed
// under /usr/libexec/kubernetes/kubelet-plugins/net/exec/
// The layout convention for a plugin is:
//   plugin-name/
//   plugin-name/plugin-name
//   plugin-name/<other-files>
//   where, plugin-name/plugin-name has the following requirements:
//     - should have exec permissions
//     - should give non-zero exit code on failure, and zero on sucess
//     - the arguments will be <action> <pod_name> <pod_namespace> <docker_id_of_infra_container>
//       whereupon, <action> will be one of:
//         - init, called when the kubelet loads the plugin
//         - setup, called after the infra container of a pod is
//                created, but before other containers of the pod are created
//         - teardown, called before the pod infra container is killed
package exec

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"syscall"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/kubelet/dockertools"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/kubelet/network"
	utilexec "github.com/GoogleCloudPlatform/kubernetes/pkg/util/exec"
	"github.com/golang/glog"
)

type execNetworkPlugin struct {
	execName string
	execPath string
	host     network.Host
}

const (
	initCmd     = "init"
	setUpCmd    = "setup"
	tearDownCmd = "teardown"
	execDir     = "/usr/libexec/kubernetes/kubelet-plugins/net/exec/"
)

func ProbeNetworkPlugins() []network.NetworkPlugin {
	return probeNetworkPluginsWithExecDir(execDir)
}

func probeNetworkPluginsWithExecDir(pluginDir string) []network.NetworkPlugin {
	execPlugins := []network.NetworkPlugin{}

	files, _ := ioutil.ReadDir(pluginDir)
	for _, f := range files {
		// only directories are counted as plugins
		// and we expect pluginDir/dirname/dirname to be an executable
		if f.IsDir() {
			execPath := path.Join(pluginDir, f.Name())
			execPlugins = append(execPlugins, &execNetworkPlugin{execName: f.Name(), execPath: execPath})
		}
	}
	return execPlugins
}

func (plugin *execNetworkPlugin) Init(host network.Host) error {
	plugin.host = host
	// call the init script
	executable := path.Join(plugin.execPath, plugin.execName)
	out, err := utilexec.New().Command(executable, initCmd).CombinedOutput()
	glog.V(5).Infof("Init 'exec' network plugin output: %s, %v", string(out), err)
	return err
}

func (plugin *execNetworkPlugin) Name() string {
	return plugin.execName
}

func (plugin *execNetworkPlugin) Validate() error {
	executable := path.Join(plugin.execPath, plugin.execName)
	if syscall.Access(executable, 0x1) != nil {
		errStr := fmt.Sprintf("Invalid exec plugin. Executable '%s' does not have correct permissions.", plugin.execName)
		return errors.New(errStr)
	}
	return nil
}

func (plugin *execNetworkPlugin) SetUpPod(name string, namespace string, id dockertools.DockerID) error {
	executable := path.Join(plugin.execPath, plugin.execName)
	out, err := utilexec.New().Command(executable, setUpCmd, name, namespace, string(id)).CombinedOutput()
	glog.V(5).Infof("SetUpPod 'exec' network plugin output: %s, %v", string(out), err)
	return err
}

func (plugin *execNetworkPlugin) TearDownPod(name string, namespace string, id dockertools.DockerID) error {
	executable := path.Join(plugin.execPath, plugin.execName)
	out, err := utilexec.New().Command(executable, tearDownCmd, name, namespace, string(id)).CombinedOutput()
	glog.V(5).Infof("TearDownPod 'exec' network plugin output: %s, %v", string(out), err)
	return err
}
