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

package main

import (
	"flag"
	"strconv"
	"time"
	"crypto/md5"
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/tools"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/exec"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/latest"
	"github.com/coreos/go-etcd/etcd"
)

const (
	NetBindingPath = "/registry/netbindings/"
)

var (
	etcdServerList util.StringList
	etcdConfigFile = flag.String("etcd_config", "", "The config file for the etcd client. Mutually exclusive with -etcd_servers")
	hostnameOverride = flag.String("hostname_override", "", "If non-empty, use this string as identification instead of the actual hostname.")
	etcdHelper *tools.EtcdHelper
	vnidMap = make(map[string]*vnidInfo)
)

type vnidInfo struct {
	Vnid	string
	PodCount int
	StopSignal chan struct{}
}

func newVnidInfo(vnid string) (*vnidInfo) {
	return &vnidInfo{
		Vnid: vnid, 
		PodCount: 0,
		StopSignal: make(chan struct{}),
	}
}

func init() {
	flag.Var(&etcdServerList, "etcd_servers", "List of etcd servers to watch (http://ip:port), comma separated (optional).")
	exec.New().Command("ovs-vsctl", "add-port", "tap1", "obr0", "--", "set", "Interface", "tap1", "ofport_request=3").CombinedOutput()
	exec.New().Command("ip", "addr", "add", "10.246.1.1/16", "dev", "tap1").CombinedOutput()
	exec.New().Command("ip", "link", "set", "dev", "tap1", "up").CombinedOutput()
	exec.New().Command("ovs-ofctl", "del-flows", "-O", "OpenFlow13", "obr0").CombinedOutput()
	exec.New().Command("ovs-ofctl", "add-flow", "-O", "OpenFlow13", "obr0", "table=0,priority=200,in_port=10,actions=goto_table:1").CombinedOutput()
	exec.New().Command("ovs-ofctl", "add-flow", "-O", "OpenFlow13", "obr0", "table=0,priority=50,in_port=3,actions=goto_table:2").CombinedOutput()
	exec.New().Command("ovs-ofctl", "add-flow", "-O", "OpenFlow13", "obr0", "table=1,priority=100,actions=output:3").CombinedOutput()
	exec.New().Command("sh", "-c", "iptables -t nat -D POSTROUTING -s 10.246.0.0/16 ! -d 10.246.0.0/16 -j MASQUERADE").CombinedOutput()
	exec.New().Command("sh", "-c", "iptables -t nat -A POSTROUTING -s 10.246.0.0/16 ! -d 10.246.0.0/16 -j MASQUERADE").CombinedOutput()
}

func makeEtcdClient() *etcd.Client {
	var etcdClient *etcd.Client
	// Set up etcd client
	if len(etcdServerList) > 0 {
		// Set up logger for etcd client
		etcd.SetLogger(util.NewLogger("etcd "))
		etcdClient = etcd.NewClient(etcdServerList)
	}
	return etcdClient
}

func hostName() string {
	if (*hostnameOverride)=="" {
		fqdn,_ := exec.New().Command("hostname", "-f").CombinedOutput()
		return string(fqdn)
	}
	return (*hostnameOverride)
}

func makeBoundPodsKey() string {
	return "/registry/nodes/" + hostName() + "/boundpods"
}

func main() {
	flag.Parse()
	util.InitLogs()
	defer util.FlushLogs()


	etcdClient := makeEtcdClient()
	etcdHelper = &tools.EtcdHelper{
		etcdClient,
		latest.Codec,
		tools.RuntimeVersionAdapter{latest.ResourceVersioner},
	}

	// initialize the existing pods' flow rules
	syncBoundPods(true)

	// launch a forever function that periodically checks for boundpods come and go
	// TODO: replace this with an etcd watch 
	util.Forever( func() { syncBoundPods(false) }, time.Second*30)
}

func syncBoundPods(initialize bool) {
	// get all boundpods
	// and then get each of the boundpod's vnid
	// pool it into a list of vnids to watch
	boundPods := &api.BoundPods{}
	key := makeBoundPodsKey()
	err := etcdHelper.ExtractObj(key, boundPods, false)
	if err!=nil {
		fmt.Printf("Error getting list of pods: %v\n", err)
		return
	}

	fmt.Printf("Found %d boundpods\n", len(boundPods.Items))
	if initialize {
		for _,pod := range boundPods.Items {
			// get the netbinding and init the rules
			nb := &api.NetBinding{}
			chksum := strconv.Itoa(int(md5.Sum([]byte(pod.Namespace))[0]))
			key := (NetBindingPath+"/"+chksum+"/"+pod.Name)
			// curl http://etcd:4001/v2/keys/netbindings/vnid/podname 
			err := etcdHelper.ExtractObj(key, nb, false) 
			if err!=nil {
				fmt.Printf("Error in fetching netbinding for pod %v.\nError: %v\n", pod, err)
				continue
			}
			ex := exec.New()
			cmd := ex.Command("pod-set-local-network.sh", "", pod.Name, nb.IPAddress, nb.MacAddress, strconv.Itoa(nb.NetID), strconv.Itoa(nb.BridgePort))
			out, err := cmd.CombinedOutput()
			fmt.Printf("Output of initializing network of %s: %s\n", pod.Name, out)
			if err!=nil {
				fmt.Printf("Error while initializing network of %s: %v\n", pod.Name, err)
			}
		}	
		// done with this func call.. we are just supposed to 
		// initialize, not watch yet
		return
	}

	// extract the vnid list out of boundpods
	// launch the watch functions for new vnids on this host
	// and kill the threads that are watching vnids that do not exist on this host anymore
	for _,pod := range boundPods.Items {
		vnid := strconv.Itoa(int(md5.Sum([]byte(pod.Namespace))[0]))
		vinfo, found := vnidMap[vnid]
		if !found {
			vinfo = newVnidInfo(vnid)
			vnidMap[vnid] = vinfo
			vinfo.PodCount = 1
			fmt.Printf("Launching watcher for %s\n", vnid)
			go watchVnid(vinfo)
		} 
	}
	for vnid, vinfo := range vnidMap {
		// found but pod count is zero? time to kill the go thread
		if vinfo.PodCount==0 {
			fmt.Printf("Stopping watch for vnid %s\n", vnid)
			vinfo.StopSignal <- struct{}{}
		}
	}
}

// WatchLoop loops, handling events from the watch interface
// If there is an interrupt signal, shut down cleanly. Otherwise, never return.
// stopSignal is set by boundPod watcher.. which basically sends a signal when there
// are no more pods of a given vnid remaining on 'this' host
func WatchLoop(w watch.Interface, netinfo *vnidInfo) {
	stopSignal := netinfo.StopSignal
	for {
		select {
		case <-stopSignal:
			fmt.Printf("Got stop signal.. exiting with thread watching %s\n", netinfo.Vnid)
			w.Stop()
			return
		case event, ok := <-w.ResultChan():
			fmt.Printf("Got an event : %v\n", event)
			if !ok {
				continue
			}
			handleVnidEvent(event, netinfo)
		}
	}
}

func makeNetBindingsKey(vnid string) string {
	return (NetBindingPath + vnid)
}

func getNetbindingResourceVersion(key string) string {
	return ""

	//netblist = &api.NetBindings{}
 	//err := etcdHelper.ExtractObj(key, netblist, false)
	//if err == nil {
	//	fmt.Printf("Error getting resource %s\n", key)
	//	return
	//}
	//return netblist.ResourceVersion
}

func watchVnid(netinfo *vnidInfo) {
	vnid := netinfo.Vnid
	key := makeNetBindingsKey(vnid)
	resourceVersion := getNetbindingResourceVersion(key)
	version,_ := tools.ParseWatchResourceVersion(resourceVersion, "netbindings")
	w,_ := etcdHelper.WatchList(key, version, tools.Everything)
	fmt.Printf("Set watch on %s\n", key)

	WatchLoop(w, netinfo)
}

func handleVnidEvent(event watch.Event, netinfo *vnidInfo) {
	nb,ok := event.Object.(*api.NetBinding)
	if !ok {
		fmt.Printf("expected a netbinding object, got %#v\n", event.Object)
		return
	}
	
	switch event.Type {
		case watch.Added, watch.Modified:
			AddOFRules(nb, netinfo)
		case watch.Deleted:
			DelOFRules(nb, netinfo)
		default:
			fmt.Printf("Unknown event %v\n", event)
	}
}

func DelOFRules(nb *api.NetBinding, netinfo *vnidInfo) {
	ex := exec.New()
	if nb.Vtep==hostName() {
		// remove the pod from netinfo list
		netinfo.PodCount = (netinfo.PodCount-1)
		if netinfo.PodCount==0 {
			// if podcount is zero, then remove all rules related to this vnid
			fmt.Printf("Remove all rules related to vnid %s\n", netinfo.Vnid)
			ex.Command("ovs-ofctl", "-O", "OpenFlow13", "del-flows", "obr0", fmt.Sprintf("cookie=%s/0xffffffff", nb.NetID)).CombinedOutput()
		} else {
			// remove rules only specific to this pod
			cmd := ex.Command("pod-remove-local-network.sh", "", nb.PodID, nb.IPAddress, nb.MacAddress, strconv.Itoa(nb.NetID), strconv.Itoa(nb.BridgePort))
			out, err := cmd.CombinedOutput()
			fmt.Printf("Output of removing local network for %s: %s\n", nb.PodID, out)
			if err!=nil {
				fmt.Printf("Error while removing local network for %s: %v\n", nb.PodID, err)
			}
		}
	} else {
		cmd := ex.Command("pod-remove-peer-network.sh", nb.Vtep, nb.IPAddress, nb.MacAddress, strconv.Itoa(nb.NetID))
		out, err := cmd.CombinedOutput()
		fmt.Printf("Output of removing peer network for %s: %s\n", nb.PodID, out)
		if err!=nil {
			fmt.Printf("Error while removing peer network for %s: %v\n", nb.PodID, err)
		}
	}
}

func AddOFRules(nb *api.NetBinding, netinfo *vnidInfo) {
	// call out to script that adds the vtep rules (ignore the pod that resides locally)
	fmt.Printf("Netbinding object found : %v\n", nb)
	if nb.Vtep==hostName() {
		// no need to add rules, the kubelet would have added it, just track it here
		netinfo.PodCount = (netinfo.PodCount+1)
	} else {
		ex := exec.New()
		cmd := ex.Command("pod-set-peer-network.sh", nb.Vtep, nb.IPAddress, nb.MacAddress, strconv.Itoa(nb.NetID))
		out, err := cmd.CombinedOutput()
		fmt.Printf("Output of modifying peer network for %s: %s\n", nb.PodID, out)
		if err!=nil {
			fmt.Printf("Error while modifying peer network for %s: %v\n", nb.PodID, err)
		}
		return
	}
}
