# Changelog

## 0.15.0
* Enables v1beta3 API and sets it to the default API version (#6098)
  * See the [v1beta3 conversion guide](http://docs.k8s.io/api.md#v1beta3-conversion-tips)
* Added multi-port Services (#6182)
* New Getting Started Guides
  * Multi-node local startup guide (#6505)
  * JUJU (#5414)
  * Mesos on Google Cloud Platform (#5442)
  * Ansible Setup instructions (#6237)
* Added a controller framework (#5270, #5473)
* The Kubelet now listens on a secure HTTPS port (#6380)
* Made kubectl errors more user-friendly (#6338)
* The apiserver now supports client cert authentication (#6190)
* The apiserver now limits the number of concurrent requests it processes (#6207)
* Added rate limiting to pod deleting (#6355)
* Implement Balanced Resource Allocation algorithm as a PriorityFunction in scheduler package (#6150)
* Enabled log collection from master (#6396)
* Added an api endpoint to pull logs from Pods (#6497)
* Added latency metrics to scheduler (#6368)
* Added latency metrics to REST client (#6409)
* etcd now runs in a pod on the master (#6221)
* nginx now runs in a container on the master (#6334)
* Began creating Docker images for master components (#6326)
* Updated GCE provider to work with gcloud 0.9.54 (#6270)
* Updated AWS provider to fix Region vs Zone semantics (#6011)
* Record event when image GC fails (#6091)
* Add a QPS limiter to the kubernetes client (#6203)
* Decrease the time it takes to run make release (#6196)
* New volume support
  * Added iscsi volume plugin (#5506)
  * Added glusterfs volume plugin (#6174)
  * AWS EBS volume support (#5138)
* Updated to heapster version to v0.10.0 (#6331)
* Updated to etcd 2.0.9 (#6544)
* Updated to Kibana to v1.2 (#6426)
* Bug Fixes
  * Kube-proxy now updates iptables rules if a service's public IPs change (#6123)
  * Retry kube-addons creation if the initial creation fails (#6200)
  * Make kube-proxy more resiliant to running out of file descriptors (#6727)

## 0.14.2
 * Fix a regression in service port handling validation
 * Add a work around for etcd bugs in watch

## 0.14.1
 * Fixed an issue where containers with hostPort would sometimes go pending forever. (#6110)

## 0.14.0
 * Add HostNetworking container option to the API.
 * PersistentVolume API
 * NFS volume plugin fixed/re-added
 * Upgraded to etcd 2.0.5 on Salt configs
 * .kubeconfig changes
 * Kubelet now posts pod status to master, versus master polling.
 * All cluster add-on images are pulled from gcr.io

## 0.13.2
 * Fixes possible cluster bring-up flakiness on GCE/Salt based clusters
 

## 0.12.2
 * #5348 - Health check the docker socket and Docker generally
 * #5395 - Garbage collect unknown containers

## 0.12.1
 * DockerCache doesn't get containers at startup (#5115)
 * Update version of kube2sky to 1.1 (#5127)
 * Monit health check kubelet and restart unhealthy one (#5120)

## 0.12.0
 * Hide the infrastructure pod from users
 * Configure scheduler via JSON
 * Improved object validation
 * Improved messages on scheduler failure
 * Improved messages on port conflicts
 * Move to thread-per-pod in the kubelet
 * Misc. kubectl improvements
 * Update etcd used by SkyDNS to 2.0.3
 * Fixes to GCE PD support
 * Improved support for secrets in the API
 * Improved OOM behavior

## 0.11
* Secret API Resources
* Better error handling in various places
* Improved RackSpace support
* Fix ```kubectl``` patch behavior
* Health check failures fire events
* Don't delete the pod infrastructure container on health check failures
* Improvements to Pod Status detection and reporting
* Reduce the size of scheduled pods in etcd
* Fix some bugs in namespace clashing
* More detailed info on failed image pulls
* Remove pods from a failed node
* Safe format and mount of GCE PDs
* Make events more resilient to etcd watch failures
* Upgrade to container-vm 01-29-2015

## 0.10
   * Improvements to swagger API documentation.
   * Upgrade container VM to 20150129
   * Start to move e2e tests to Ginkgo
   * Fix apiserver proxy path rewriting
   * Upgrade to etcd 2.0.0
   * Add a wordpress/mysql example
   * Improve responsiveness of the master when creating new pods
   * Improve api object validation in numerous small ways
   * Add support for IPC namespaces
   * Improve GCE PD support
   * Make replica controllers with node selectors work correctly
   * Lots of improvements to e2e tests (more to come)

## 0.9
### Features
 - Various improvements to kubectl
 - Improvements to API Server caching
 - Full control over container command (docker entrypoint) and arguments (docker cmd);
   users of v1beta3 must change to use the Args field of the container for images that
   set a default entrypoint

### Bug fixes
 - Disable image GC since it was causing docker pull problems
 - Various small bug fixes

## 0.8
### Features 
 - Docker 1.4.1
 - Optional session affinity for Services
 - Better information on out of memory errors
 - Scheduling pods on specific machines
 - Improve performance of Pod listing
 - Image garbage collection
 - Automatic internal DNS for Services
 - Swagger UI for the API
 - Update cAdvisor Manifest to use google/cadvisor:0.7.1 image

### Bug fixes
 - Fix Docker exec liveness health checks
 - Fix a bug where the service proxy would ignore new events
 - Fix a crash for kubelet when without EtcdClient

## 0.7
### Features
  - Make updating node labels easier
  - Support updating node capacity
  - kubectl streaming log support
  - Improve /validate validation
  - Fix GCE-PD to work across machine reboots
  - Don't delete other attached disks on cluster turn-down
  - Return errors if a user attempts to create a UDP external balancer
  - TLS version bump from SSLv3 to TLSv1.0
  - x509 request authenticator
  - Container VM on GCE updated to 20141208
  - Improvements to kubectl yaml handling
### Bug fixes
  - Fix kubelet panics when docker has no name for containers
  - Only count non-dead pods in replica controller status reporting
  - Fix version requirements for docker exec

## 0.6
### Features
  - Docker 1.3.3 (0.6.2)
  - Authentication for Kubelet/Apiserver communication
  - Kubectl clean ups
  - Enable Docker Cache on GCE
  - Better support for Private Repositories
### Bug fixes
  - Fixed Public IP support on non-GCE hosts
  - Fixed 32-bit build

## 0.5 (11/17/2014)
### Features
  - New client utility available: kubectl. This will eventually replace kubecfg. (#1325)
  - Services v2. We now assign IP addresses to services.  Details in #1107. (#1402)
  - Event support: (#1789, #2267, #2270, #2384)
  - Namespaces: (#1564)
  - Fixes for Docker 1.3 (#1841, #1842)
  - Support for automatically installing log saving and searching using fluentd and elasticsearch (#1610) and GCP logging (#1919).  If using elastic search, logs can be viewed with Kibana (#2013)
  - Read only API endpoint for internal lookups (#1916)
  - Lots of ground work for pluggable auth model. (#1847)
  - "run once" mode for the kubelet (#1707)
  - Restrict which minion a pod schedules on based on predicate tested agains minion labels. (#1946, #2007)
  - git based volumes: (#1945)
  - Container garbage collection.  Remove old instances of containers in the case of crash/fail loops. (#2022)
  - Publish the APIServer as a service to pods in the cluster (#1920)
  - Heapster monitoring (#2208)
  - cAdvisor 0.5.0
  - Switch default pull policy to PullIfNotPresent (#2388) except latest images
  - Initial IPv6 support (#2147)
  - Service proxy retry support (#2281)
  - Windows client build (largely untested) (#2332)
  - UDP Portals (#2191)
  - Capture application termination log (#2225)
  - pod update support (#1865, #2077, #2160)

### Cluster/Cloud support
  - Add OpenStack support with CloudProvider. (#1676)
  - Example systemd units (#1831)
  - Updated Rackspace support based on CoreOS (#1832)
  - Automatic security updates for debian based systems (#2012)
  - For debian (and GCE) pull docker (#2104), salt and etcd (#2245) from Google Cloud Storage.
  - For GCE, start with the Container VM image instead of stock debian.  This enables memcg support. (#2046)
  - Cluster install: Updated support for deploying to vSphere (#1747)
  - AWS support (#2260, #2216)

### Examples/Extras/Docs
  - Documentation on how to use SkyDNS with Kubernetes (#1845)
  - Podex (convert Docker image to pod desc) tool now supports multiple images. (#1898)
  - Documentation: 201 level walk through. (#1924)
  - Local Docker Setup: (#1716)

## 0.4 (10/14/2014)
 - Support Persistent Disk volume type
