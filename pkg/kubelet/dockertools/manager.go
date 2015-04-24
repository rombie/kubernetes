/*
Copyright 2015 Google Inc. All rights reserved.

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

package dockertools

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/capabilities"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/record"
	kubecontainer "github.com/GoogleCloudPlatform/kubernetes/pkg/kubelet/container"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/types"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/fsouza/go-dockerclient"
	"github.com/golang/glog"
	"github.com/golang/groupcache/lru"
)

const (
	maxReasonCacheEntries = 200
)

// TODO: Eventually DockerManager should implement kubecontainer.Runtime
// interface.
type DockerManager struct {
	client              DockerInterface
	recorder            record.EventRecorder
	readinessManager    *kubecontainer.ReadinessManager
	containerRefManager *kubecontainer.RefManager

	// TODO(yifan): PodInfraContainerImage can be unexported once
	// we move createPodInfraContainer into dockertools.
	PodInfraContainerImage string
	// reasonCache stores the failure reason of the last container creation
	// and/or start in a string, keyed by <pod_UID>_<container_name>. The goal
	// is to propagate this reason to the container status. This endeavor is
	// "best-effort" for two reasons:
	//   1. The cache is not persisted.
	//   2. We use an LRU cache to avoid extra garbage collection work. This
	//      means that some entries may be recycled before a pod has been
	//      deleted.
	reasonCache stringCache
	// TODO(yifan): We export this for testability, so when we have a fake
	// container manager, then we can unexport this. Also at that time, we
	// use the concrete type so that we can record the pull failure and eliminate
	// the image checking in GetPodStatus().
	Puller DockerPuller
}

func NewDockerManager(
	client DockerInterface,
	recorder record.EventRecorder,
	readinessManager *kubecontainer.ReadinessManager,
	containerRefManager *kubecontainer.RefManager,
	podInfraContainerImage string,
	qps float32,
	burst int) *DockerManager {
	reasonCache := stringCache{cache: lru.New(maxReasonCacheEntries)}
	return &DockerManager{
		client:                 client,
		recorder:               recorder,
		readinessManager:       readinessManager,
		containerRefManager:    containerRefManager,
		PodInfraContainerImage: podInfraContainerImage,
		reasonCache:            reasonCache,
		Puller:                 newDockerPuller(client, qps, burst),
	}
}

// A cache which stores strings keyed by <pod_UID>_<container_name>.
type stringCache struct {
	lock  sync.RWMutex
	cache *lru.Cache
}

func (sc *stringCache) composeKey(uid types.UID, name string) string {
	return fmt.Sprintf("%s_%s", uid, name)
}

func (sc *stringCache) Add(uid types.UID, name string, value string) {
	sc.lock.Lock()
	defer sc.lock.Unlock()
	sc.cache.Add(sc.composeKey(uid, name), value)
}

func (sc *stringCache) Remove(uid types.UID, name string) {
	sc.lock.Lock()
	defer sc.lock.Unlock()
	sc.cache.Remove(sc.composeKey(uid, name))
}

func (sc *stringCache) Get(uid types.UID, name string) (string, bool) {
	sc.lock.RLock()
	defer sc.lock.RUnlock()
	value, ok := sc.cache.Get(sc.composeKey(uid, name))
	if ok {
		return value.(string), ok
	} else {
		return "", ok
	}
}

// GetKubeletDockerContainerLogs returns logs of a specific container. By
// default, it returns a snapshot of the container log. Set |follow| to true to
// stream the log. Set |follow| to false and specify the number of lines (e.g.
// "100" or "all") to tail the log.
// TODO: Make 'RawTerminal' option  flagable.
func (dm *DockerManager) GetKubeletDockerContainerLogs(containerID, tail string, follow bool, stdout, stderr io.Writer) (err error) {
	opts := docker.LogsOptions{
		Container:    containerID,
		Stdout:       true,
		Stderr:       true,
		OutputStream: stdout,
		ErrorStream:  stderr,
		Timestamps:   true,
		RawTerminal:  false,
		Follow:       follow,
	}

	if !follow {
		opts.Tail = tail
	}

	err = dm.client.Logs(opts)
	return
}

var (
	// ErrNoContainersInPod is returned when there are no containers for a given pod
	ErrNoContainersInPod = errors.New("no containers exist for this pod")

	// ErrNoPodInfraContainerInPod is returned when there is no pod infra container for a given pod
	ErrNoPodInfraContainerInPod = errors.New("No pod infra container exists for this pod")

	// ErrContainerCannotRun is returned when a container is created, but cannot run properly
	ErrContainerCannotRun = errors.New("Container cannot run")
)

// Internal information kept for containers from inspection
type containerStatusResult struct {
	status api.ContainerStatus
	ip     string
	err    error
}

func (dm *DockerManager) inspectContainer(dockerID, containerName, tPath string) *containerStatusResult {
	result := containerStatusResult{api.ContainerStatus{}, "", nil}

	inspectResult, err := dm.client.InspectContainer(dockerID)

	if err != nil {
		result.err = err
		return &result
	}
	if inspectResult == nil {
		// Why did we not get an error?
		return &result
	}

	glog.V(3).Infof("Container inspect result: %+v", *inspectResult)
	result.status = api.ContainerStatus{
		Name:        containerName,
		Image:       inspectResult.Config.Image,
		ImageID:     DockerPrefix + inspectResult.Image,
		ContainerID: DockerPrefix + dockerID,
	}

	if inspectResult.State.Running {
		result.status.State.Running = &api.ContainerStateRunning{
			StartedAt: util.NewTime(inspectResult.State.StartedAt),
		}
		if containerName == PodInfraContainerName && inspectResult.NetworkSettings != nil {
			result.ip = inspectResult.NetworkSettings.IPAddress
		}
	} else if !inspectResult.State.FinishedAt.IsZero() {
		reason := ""
		// Note: An application might handle OOMKilled gracefully.
		// In that case, the container is oom killed, but the exit
		// code could be 0.
		if inspectResult.State.OOMKilled {
			reason = "OOM Killed"
		} else {
			reason = inspectResult.State.Error
		}
		result.status.State.Termination = &api.ContainerStateTerminated{
			ExitCode:    inspectResult.State.ExitCode,
			Reason:      reason,
			StartedAt:   util.NewTime(inspectResult.State.StartedAt),
			FinishedAt:  util.NewTime(inspectResult.State.FinishedAt),
			ContainerID: DockerPrefix + dockerID,
		}
		if tPath != "" {
			path, found := inspectResult.Volumes[tPath]
			if found {
				data, err := ioutil.ReadFile(path)
				if err != nil {
					glog.Errorf("Error on reading termination-log %s: %v", path, err)
				} else {
					result.status.State.Termination.Message = string(data)
				}
			}
		}
	} else {
		// TODO(dchen1107): Separate issue docker/docker#8294 was filed
		result.status.State.Waiting = &api.ContainerStateWaiting{
			Reason: ErrContainerCannotRun.Error(),
		}
	}

	return &result
}

// GetPodStatus returns docker related status for all containers in the pod as
// well as the infrastructure container.
func (dm *DockerManager) GetPodStatus(pod *api.Pod) (*api.PodStatus, error) {
	podFullName := kubecontainer.GetPodFullName(pod)
	uid := pod.UID
	manifest := pod.Spec

	oldStatuses := make(map[string]api.ContainerStatus, len(pod.Spec.Containers))
	lastObservedTime := make(map[string]util.Time, len(pod.Spec.Containers))
	for _, status := range pod.Status.ContainerStatuses {
		oldStatuses[status.Name] = status
		if status.LastTerminationState.Termination != nil {
			lastObservedTime[status.Name] = status.LastTerminationState.Termination.FinishedAt
		}
	}

	var podStatus api.PodStatus
	statuses := make(map[string]*api.ContainerStatus, len(pod.Spec.Containers))

	expectedContainers := make(map[string]api.Container)
	for _, container := range manifest.Containers {
		expectedContainers[container.Name] = container
	}
	expectedContainers[PodInfraContainerName] = api.Container{}

	containers, err := dm.client.ListContainers(docker.ListContainersOptions{All: true})
	if err != nil {
		return nil, err
	}

	containerDone := util.NewStringSet()
	// Loop through list of running and exited docker containers to construct
	// the statuses. We assume docker returns a list of containers sorted in
	// reverse by time.
	for _, value := range containers {
		if len(value.Names) == 0 {
			continue
		}
		dockerName, _, err := ParseDockerName(value.Names[0])
		if err != nil {
			continue
		}
		if dockerName.PodFullName != podFullName {
			continue
		}
		if uid != "" && dockerName.PodUID != uid {
			continue
		}
		dockerContainerName := dockerName.ContainerName
		c, found := expectedContainers[dockerContainerName]
		if !found {
			continue
		}
		terminationMessagePath := c.TerminationMessagePath
		if containerDone.Has(dockerContainerName) {
			continue
		}

		var terminationState *api.ContainerState = nil
		// Inspect the container.
		result := dm.inspectContainer(value.ID, dockerContainerName, terminationMessagePath)
		if result.err != nil {
			return nil, result.err
		} else if result.status.State.Termination != nil {
			terminationState = &result.status.State
		}

		if containerStatus, found := statuses[dockerContainerName]; found {
			if containerStatus.LastTerminationState.Termination == nil && terminationState != nil {
				// Populate the last termination state.
				containerStatus.LastTerminationState = *terminationState
			}
			count := true
			// Only count dead containers terminated after last time we observed,
			if lastObservedTime, ok := lastObservedTime[dockerContainerName]; ok {
				if terminationState != nil && terminationState.Termination.FinishedAt.After(lastObservedTime.Time) {
					count = false
				} else {
					// The container finished before the last observation. No
					// need to examine/count the older containers. Mark the
					// container name as done.
					containerDone.Insert(dockerContainerName)
				}
			}
			if count {
				containerStatus.RestartCount += 1
			}
			continue
		}

		if dockerContainerName == PodInfraContainerName {
			// Found network container
			if result.status.State.Running != nil {
				podStatus.PodIP = result.ip
			}
		} else {
			// Add user container information.
			if oldStatus, found := oldStatuses[dockerContainerName]; found {
				// Use the last observed restart count if it's available.
				result.status.RestartCount = oldStatus.RestartCount
			}
			statuses[dockerContainerName] = &result.status
		}
	}

	// Handle the containers for which we cannot find any associated active or
	// dead docker containers.
	for _, container := range manifest.Containers {
		if _, found := statuses[container.Name]; found {
			continue
		}
		var containerStatus api.ContainerStatus
		containerStatus.Name = container.Name
		containerStatus.Image = container.Image
		if oldStatus, found := oldStatuses[container.Name]; found {
			// Some states may be lost due to GC; apply the last observed
			// values if possible.
			containerStatus.RestartCount = oldStatus.RestartCount
			containerStatus.LastTerminationState = oldStatus.LastTerminationState
		}
		//Check image is ready on the node or not.
		// TODO: If we integrate DockerPuller into DockerManager, we can
		// record the pull failure and eliminate the image checking below.
		image := container.Image
		// TODO(dchen1107): docker/docker/issues/8365 to figure out if the image exists
		_, err := dm.client.InspectImage(image)
		if err == nil {
			containerStatus.State.Waiting = &api.ContainerStateWaiting{
				Reason: fmt.Sprintf("Image: %s is ready, container is creating", image),
			}
		} else if err == docker.ErrNoSuchImage {
			containerStatus.State.Waiting = &api.ContainerStateWaiting{
				Reason: fmt.Sprintf("Image: %s is not ready on the node", image),
			}
		}
		statuses[container.Name] = &containerStatus
	}

	podStatus.ContainerStatuses = make([]api.ContainerStatus, 0)
	for containerName, status := range statuses {
		if status.State.Waiting != nil {
			// For containers in the waiting state, fill in a specific reason if it is recorded.
			if reason, ok := dm.reasonCache.Get(uid, containerName); ok {
				status.State.Waiting.Reason = reason
			}
		}
		podStatus.ContainerStatuses = append(podStatus.ContainerStatuses, *status)
	}

	return &podStatus, nil
}

func (dm *DockerManager) GetRunningContainers(ids []string) ([]*docker.Container, error) {
	var result []*docker.Container
	if dm.client == nil {
		return nil, fmt.Errorf("unexpected nil docker client.")
	}
	for ix := range ids {
		status, err := dm.client.InspectContainer(ids[ix])
		if err != nil {
			return nil, err
		}
		if status != nil && status.State.Running {
			result = append(result, status)
		}
	}
	return result, nil
}

func (dm *DockerManager) runContainerRecordErrorReason(pod *api.Pod, container *api.Container, opts *kubecontainer.RunContainerOptions, ref *api.ObjectReference) (string, error) {
	dockerID, err := dm.runContainer(pod, container, opts, ref)
	if err != nil {
		errString := err.Error()
		if errString != "" {
			dm.reasonCache.Add(pod.UID, container.Name, errString)
		} else {
			dm.reasonCache.Remove(pod.UID, container.Name)
		}
	}
	return dockerID, err
}

func (dm *DockerManager) runContainer(pod *api.Pod, container *api.Container, opts *kubecontainer.RunContainerOptions, ref *api.ObjectReference) (string, error) {
	dockerName := KubeletContainerName{
		PodFullName:   kubecontainer.GetPodFullName(pod),
		PodUID:        pod.UID,
		ContainerName: container.Name,
	}
	exposedPorts, portBindings := makePortsAndBindings(container)

	// TODO(vmarmol): Handle better.
	// Cap hostname at 63 chars (specification is 64bytes which is 63 chars and the null terminating char).
	const hostnameMaxLen = 63
	containerHostname := pod.Name
	if len(containerHostname) > hostnameMaxLen {
		containerHostname = containerHostname[:hostnameMaxLen]
	}
	dockerOpts := docker.CreateContainerOptions{
		Name: BuildDockerName(dockerName, container),
		Config: &docker.Config{
			Env:          opts.Envs,
			ExposedPorts: exposedPorts,
			Hostname:     containerHostname,
			Image:        container.Image,
			Memory:       container.Resources.Limits.Memory().Value(),
			CPUShares:    milliCPUToShares(container.Resources.Limits.Cpu().MilliValue()),
			WorkingDir:   container.WorkingDir,
		},
	}

	setEntrypointAndCommand(container, &dockerOpts)

	glog.V(3).Infof("Container %v/%v/%v: setting entrypoint \"%v\" and command \"%v\"", pod.Namespace, pod.Name, container.Name, dockerOpts.Config.Entrypoint, dockerOpts.Config.Cmd)

	dockerContainer, err := dm.client.CreateContainer(dockerOpts)
	if err != nil {
		if ref != nil {
			dm.recorder.Eventf(ref, "failed", "Failed to create docker container with error: %v", err)
		}
		return "", err
	}

	if ref != nil {
		dm.recorder.Eventf(ref, "created", "Created with docker id %v", dockerContainer.ID)
	}

	// The reason we create and mount the log file in here (not in kubelet) is because
	// the file's location depends on the ID of the container, and we need to create and
	// mount the file before actually starting the container.
	// TODO(yifan): Consider to pull this logic out since we might need to reuse it in
	// other container runtime.
	if opts.PodContainerDir != "" && len(container.TerminationMessagePath) != 0 {
		containerLogPath := path.Join(opts.PodContainerDir, dockerContainer.ID)
		fs, err := os.Create(containerLogPath)
		if err != nil {
			// TODO: Clean up the previouly created dir? return the error?
			glog.Errorf("Error on creating termination-log file %q: %v", containerLogPath, err)
		} else {
			fs.Close() // Close immediately; we're just doing a `touch` here
			b := fmt.Sprintf("%s:%s", containerLogPath, container.TerminationMessagePath)
			opts.Binds = append(opts.Binds, b)
		}
	}

	privileged := false
	if capabilities.Get().AllowPrivileged {
		privileged = container.Privileged
	} else if container.Privileged {
		return "", fmt.Errorf("container requested privileged mode, but it is disallowed globally.")
	}

	capAdd, capDrop := makeCapabilites(container.Capabilities.Add, container.Capabilities.Drop)
	hc := &docker.HostConfig{
		PortBindings: portBindings,
		Binds:        opts.Binds,
		NetworkMode:  opts.NetMode,
		IpcMode:      opts.IpcMode,
		Privileged:   privileged,
		CapAdd:       capAdd,
		CapDrop:      capDrop,
	}
	if len(opts.DNS) > 0 {
		hc.DNS = opts.DNS
	}
	if len(opts.DNSSearch) > 0 {
		hc.DNSSearch = opts.DNSSearch
	}

	if err = dm.client.StartContainer(dockerContainer.ID, hc); err != nil {
		if ref != nil {
			dm.recorder.Eventf(ref, "failed",
				"Failed to start with docker id %v with error: %v", dockerContainer.ID, err)
		}
		return "", err
	}
	if ref != nil {
		dm.recorder.Eventf(ref, "started", "Started with docker id %v", dockerContainer.ID)
	}
	return dockerContainer.ID, nil
}

func setEntrypointAndCommand(container *api.Container, opts *docker.CreateContainerOptions) {
	if len(container.Command) != 0 {
		opts.Config.Entrypoint = container.Command
	}
	if len(container.Args) != 0 {
		opts.Config.Cmd = container.Args
	}
}

func makePortsAndBindings(container *api.Container) (map[docker.Port]struct{}, map[docker.Port][]docker.PortBinding) {
	exposedPorts := map[docker.Port]struct{}{}
	portBindings := map[docker.Port][]docker.PortBinding{}
	for _, port := range container.Ports {
		exteriorPort := port.HostPort
		if exteriorPort == 0 {
			// No need to do port binding when HostPort is not specified
			continue
		}
		interiorPort := port.ContainerPort
		// Some of this port stuff is under-documented voodoo.
		// See http://stackoverflow.com/questions/20428302/binding-a-port-to-a-host-interface-using-the-rest-api
		var protocol string
		switch strings.ToUpper(string(port.Protocol)) {
		case "UDP":
			protocol = "/udp"
		case "TCP":
			protocol = "/tcp"
		default:
			glog.Warningf("Unknown protocol %q: defaulting to TCP", port.Protocol)
			protocol = "/tcp"
		}
		dockerPort := docker.Port(strconv.Itoa(interiorPort) + protocol)
		exposedPorts[dockerPort] = struct{}{}
		portBindings[dockerPort] = []docker.PortBinding{
			{
				HostPort: strconv.Itoa(exteriorPort),
				HostIP:   port.HostIP,
			},
		}
	}
	return exposedPorts, portBindings
}

func makeCapabilites(capAdd []api.CapabilityType, capDrop []api.CapabilityType) ([]string, []string) {
	var (
		addCaps  []string
		dropCaps []string
	)
	for _, cap := range capAdd {
		addCaps = append(addCaps, string(cap))
	}
	for _, cap := range capDrop {
		dropCaps = append(dropCaps, string(cap))
	}
	return addCaps, dropCaps
}

func (dm *DockerManager) GetPods(all bool) ([]*kubecontainer.Pod, error) {
	pods := make(map[types.UID]*kubecontainer.Pod)
	var result []*kubecontainer.Pod

	containers, err := GetKubeletDockerContainers(dm.client, all)
	if err != nil {
		return nil, err
	}

	// Group containers by pod.
	for _, c := range containers {
		if len(c.Names) == 0 {
			glog.Warningf("Cannot parse empty docker container name: %#v", c.Names)
			continue
		}
		dockerName, hash, err := ParseDockerName(c.Names[0])
		if err != nil {
			glog.Warningf("Parse docker container name %q error: %v", c.Names[0], err)
			continue
		}
		pod, found := pods[dockerName.PodUID]
		if !found {
			name, namespace, err := kubecontainer.ParsePodFullName(dockerName.PodFullName)
			if err != nil {
				glog.Warningf("Parse pod full name %q error: %v", dockerName.PodFullName, err)
				continue
			}
			pod = &kubecontainer.Pod{
				ID:        dockerName.PodUID,
				Name:      name,
				Namespace: namespace,
			}
			pods[dockerName.PodUID] = pod
		}
		pod.Containers = append(pod.Containers, &kubecontainer.Container{
			ID:      types.UID(c.ID),
			Name:    dockerName.ContainerName,
			Hash:    hash,
			Created: c.Created,
		})
	}

	// Convert map to list.
	for _, c := range pods {
		result = append(result, c)
	}
	return result, nil
}

func (dm *DockerManager) Pull(image string) error {
	return dm.Puller.Pull(image)
}

func (dm *DockerManager) IsImagePresent(image string) (bool, error) {
	return dm.Puller.IsImagePresent(image)
}

// PodInfraContainer returns true if the pod infra container has changed.
func (dm *DockerManager) PodInfraContainerChanged(pod *api.Pod, podInfraContainer *kubecontainer.Container) (bool, error) {
	networkMode := ""
	var ports []api.ContainerPort

	dockerPodInfraContainer, err := dm.client.InspectContainer(string(podInfraContainer.ID))
	if err != nil {
		return false, err
	}

	// Check network mode.
	if dockerPodInfraContainer.HostConfig != nil {
		networkMode = dockerPodInfraContainer.HostConfig.NetworkMode
	}
	if pod.Spec.HostNetwork {
		if networkMode != "host" {
			glog.V(4).Infof("host: %v, %v", pod.Spec.HostNetwork, networkMode)
			return true, nil
		}
	} else {
		// Docker only exports ports from the pod infra container. Let's
		// collect all of the relevant ports and export them.
		for _, container := range pod.Spec.Containers {
			ports = append(ports, container.Ports...)
		}
	}
	expectedPodInfraContainer := &api.Container{
		Name:  PodInfraContainerName,
		Image: dm.PodInfraContainerImage,
		Ports: ports,
	}
	return podInfraContainer.Hash != HashContainer(expectedPodInfraContainer), nil
}

type dockerVersion docker.APIVersion

func NewVersion(input string) (dockerVersion, error) {
	version, err := docker.NewAPIVersion(input)
	return dockerVersion(version), err
}

func (dv dockerVersion) String() string {
	return docker.APIVersion(dv).String()
}

func (dv dockerVersion) Compare(other string) (int, error) {
	a := docker.APIVersion(dv)
	b, err := docker.NewAPIVersion(other)
	if err != nil {
		return 0, err
	}
	if a.LessThan(b) {
		return -1, nil
	}
	if a.GreaterThan(b) {
		return 1, nil
	}
	return 0, nil
}

func (dm *DockerManager) Version() (kubecontainer.Version, error) {
	env, err := dm.client.Version()
	if err != nil {
		return nil, fmt.Errorf("docker: failed to get docker version: %v", err)
	}

	apiVersion := env.Get("ApiVersion")
	version, err := docker.NewAPIVersion(apiVersion)
	if err != nil {
		return nil, fmt.Errorf("docker: failed to parse docker server version %q: %v", apiVersion, err)
	}
	return dockerVersion(version), nil
}

// The first version of docker that supports exec natively is 1.3.0 == API 1.15
var dockerAPIVersionWithExec = "1.15"

func (dm *DockerManager) nativeExecSupportExists() (bool, error) {
	version, err := dm.Version()
	if err != nil {
		return false, err
	}
	result, err := version.Compare(dockerAPIVersionWithExec)
	if result >= 0 {
		return true, err
	}
	return false, err
}

func (dm *DockerManager) getRunInContainerCommand(containerID string, cmd []string) (*exec.Cmd, error) {
	args := append([]string{"exec"}, cmd...)
	command := exec.Command("/usr/sbin/nsinit", args...)
	command.Dir = fmt.Sprintf("/var/lib/docker/execdriver/native/%s", containerID)
	return command, nil
}

func (dm *DockerManager) runInContainerUsingNsinit(containerID string, cmd []string) ([]byte, error) {
	c, err := dm.getRunInContainerCommand(containerID, cmd)
	if err != nil {
		return nil, err
	}
	return c.CombinedOutput()
}

// RunInContainer uses nsinit to run the command inside the container identified by containerID
// TODO(yifan): Use strong type for containerID.
func (dm *DockerManager) RunInContainer(containerID string, cmd []string) ([]byte, error) {
	// If native exec support does not exist in the local docker daemon use nsinit.
	useNativeExec, err := dm.nativeExecSupportExists()
	if err != nil {
		return nil, err
	}
	if !useNativeExec {
		return dm.runInContainerUsingNsinit(containerID, cmd)
	}
	createOpts := docker.CreateExecOptions{
		Container:    containerID,
		Cmd:          cmd,
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
	}
	execObj, err := dm.client.CreateExec(createOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to run in container - Exec setup failed - %v", err)
	}
	var buf bytes.Buffer
	wrBuf := bufio.NewWriter(&buf)
	startOpts := docker.StartExecOptions{
		Detach:       false,
		Tty:          false,
		OutputStream: wrBuf,
		ErrorStream:  wrBuf,
		RawTerminal:  false,
	}
	errChan := make(chan error, 1)
	go func() {
		errChan <- dm.client.StartExec(execObj.ID, startOpts)
	}()
	wrBuf.Flush()
	return buf.Bytes(), <-errChan
}

// ExecInContainer uses nsenter to run the command inside the container identified by containerID.
//
// TODO:
//  - match cgroups of container
//  - should we support `docker exec`?
//  - should we support nsenter in a container, running with elevated privs and --pid=host?
//  - use strong type for containerId
func (dm *DockerManager) ExecInContainer(containerId string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool) error {
	nsenter, err := exec.LookPath("nsenter")
	if err != nil {
		return fmt.Errorf("exec unavailable - unable to locate nsenter")
	}

	container, err := dm.client.InspectContainer(containerId)
	if err != nil {
		return err
	}

	if !container.State.Running {
		return fmt.Errorf("container not running (%s)", container)
	}

	containerPid := container.State.Pid

	// TODO what if the container doesn't have `env`???
	args := []string{"-t", fmt.Sprintf("%d", containerPid), "-m", "-i", "-u", "-n", "-p", "--", "env", "-i"}
	args = append(args, fmt.Sprintf("HOSTNAME=%s", container.Config.Hostname))
	args = append(args, container.Config.Env...)
	args = append(args, cmd...)
	command := exec.Command(nsenter, args...)
	if tty {
		p, err := StartPty(command)
		if err != nil {
			return err
		}
		defer p.Close()

		// make sure to close the stdout stream
		defer stdout.Close()

		if stdin != nil {
			go io.Copy(p, stdin)
		}

		if stdout != nil {
			go io.Copy(stdout, p)
		}

		return command.Wait()
	} else {
		if stdin != nil {
			// Use an os.Pipe here as it returns true *os.File objects.
			// This way, if you run 'kubectl exec -p <pod> -i bash' (no tty) and type 'exit',
			// the call below to command.Run() can unblock because its Stdin is the read half
			// of the pipe.
			r, w, err := os.Pipe()
			if err != nil {
				return err
			}
			go io.Copy(w, stdin)

			command.Stdin = r
		}
		if stdout != nil {
			command.Stdout = stdout
		}
		if stderr != nil {
			command.Stderr = stderr
		}

		return command.Run()
	}
}

// PortForward executes socat in the pod's network namespace and copies
// data between stream (representing the user's local connection on their
// computer) and the specified port in the container.
//
// TODO:
//  - match cgroups of container
//  - should we support nsenter + socat on the host? (current impl)
//  - should we support nsenter + socat in a container, running with elevated privs and --pid=host?
func (dm *DockerManager) PortForward(pod *kubecontainer.Pod, port uint16, stream io.ReadWriteCloser) error {
	podInfraContainer := pod.FindContainerByName(PodInfraContainerName)
	if podInfraContainer == nil {
		return fmt.Errorf("cannot find pod infra container in pod %q", kubecontainer.BuildPodFullName(pod.Name, pod.Namespace))
	}
	container, err := dm.client.InspectContainer(string(podInfraContainer.ID))
	if err != nil {
		return err
	}

	if !container.State.Running {
		return fmt.Errorf("container not running (%s)", container)
	}

	containerPid := container.State.Pid
	// TODO what if the host doesn't have it???
	_, lookupErr := exec.LookPath("socat")
	if lookupErr != nil {
		return fmt.Errorf("Unable to do port forwarding: socat not found.")
	}
	args := []string{"-t", fmt.Sprintf("%d", containerPid), "-n", "socat", "-", fmt.Sprintf("TCP4:localhost:%d", port)}
	// TODO use exec.LookPath
	command := exec.Command("nsenter", args...)
	command.Stdin = stream
	command.Stdout = stream
	return command.Run()
}

// KillContainer kills a container identified by containerID.
// Internally, it invokes docker's StopContainer API with a timeout of 10s.
// TODO(yifan): Use new ContainerID type.
func (dm *DockerManager) KillContainer(containerID types.UID) error {
	ID := string(containerID)
	glog.V(2).Infof("Killing container with id %q", ID)
	dm.readinessManager.RemoveReadiness(ID)
	err := dm.client.StopContainer(ID, 10)

	ref, ok := dm.containerRefManager.GetRef(ID)
	if !ok {
		glog.Warningf("No ref for pod '%v'", ID)
	} else {
		// TODO: pass reason down here, and state, or move this call up the stack.
		dm.recorder.Eventf(ref, "killing", "Killing %v", ID)
	}
	return err
}

// Run a single container from a pod. Returns the docker container ID
func (dm *DockerManager) RunContainer(pod *api.Pod, container *api.Container, generator kubecontainer.RunContainerOptionsGenerator, runner kubecontainer.HandlerRunner, netMode, ipcMode string) (DockerID, error) {
	ref, err := kubecontainer.GenerateContainerRef(pod, container)
	if err != nil {
		glog.Errorf("Couldn't make a ref to pod %v, container %v: '%v'", pod.Name, container.Name, err)
	}

	opts, err := generator.GenerateRunContainerOptions(pod, container, netMode, ipcMode)
	if err != nil {
		return "", err
	}

	id, err := dm.runContainerRecordErrorReason(pod, container, opts, ref)
	if err != nil {
		return "", err
	}

	// Remember this reference so we can report events about this container
	if ref != nil {
		dm.containerRefManager.SetRef(id, ref)
	}

	if container.Lifecycle != nil && container.Lifecycle.PostStart != nil {
		handlerErr := runner.Run(id, pod, container, container.Lifecycle.PostStart)
		if handlerErr != nil {
			dm.KillContainer(types.UID(id))
			return DockerID(""), fmt.Errorf("failed to call event handler: %v", handlerErr)
		}
	}
	return DockerID(id), err
}
