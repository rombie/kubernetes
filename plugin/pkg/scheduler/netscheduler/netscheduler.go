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

package netscheduler

import (
	"fmt"
	"strings"
	"errors"
	"strconv"
	"crypto/md5"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/golang/glog"
)

type NetScheduler struct {
	ipSubnet	*Subnet
	macSubnet	*Subnet
	bridgePortPool	[]int
}

type Octet uint8

type Subnet struct {
	octets		[]Octet
	mask		int
}

const (
	defaultIPSubnet = "10.246.1.2/24" // IP Subnet to generate addresses from
	defaultMacSubnet = "10:20:30:00:00:00/24"  // Mac Subnet to generate addresses from
)

func NewNetScheduler(ipsubnet string, macsubnet string) *NetScheduler {
	ipsub := NewIPSubnet()
	macsub := NewMacSubnet()
	if len(ipsubnet)==0 {
		ipsubnet = defaultIPSubnet
	}
	if len(macsubnet)==0 {
		macsubnet = defaultMacSubnet
	}
	ipsub.InitIPSubnet(ipsubnet)
	macsub.InitMacSubnet(macsubnet)
	bridgePortPool := make([]int, 1)
	bridgePortPool[0] = 10 // hardcoding the first bridge port, not to be used by anyone
	return &NetScheduler{
		ipSubnet: ipsub,
		macSubnet: macsub,
		bridgePortPool: bridgePortPool,
	}
}

func NewIPSubnet() *Subnet {
	subnet := &Subnet{}
	subnet.octets = make([]Octet,4)
	subnet.mask  = 0
	return subnet
}

func NewMacSubnet() *Subnet {
	subnet := &Subnet{}
	subnet.octets = make([]Octet,6)
	subnet.mask  = 0
	return subnet
}

func (ipsubnet *Subnet) ToIPStr() string {
	return fmt.Sprintf("%d.%d.%d.%d", ipsubnet.octets[0], ipsubnet.octets[1], ipsubnet.octets[2], ipsubnet.octets[3]) 
}

func (macsubnet *Subnet) ToMacStr() string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", macsubnet.octets[0], macsubnet.octets[1], macsubnet.octets[2], macsubnet.octets[3], macsubnet.octets[4], macsubnet.octets[5])
}

func (ipsubnet *Subnet) InitIPSubnet(fouroctets string) {
	subnet := strings.Split(fouroctets, "/")
	// if mask is missing it will be assumed to be /24
	if len(subnet)==1 {
		ipsubnet.mask = 24
	} else {
		mask,e := strconv.Atoi(subnet[1])
		if (e!=nil || (mask!=24 && mask!=32 && mask!=16 && mask!=8)) {
			// error in argument
			glog.V(2).Infof("Error in parsing subnet string: invalid range %s.", subnet[1])
			return
		}
		ipsubnet.mask = mask
	}
	octets := strings.Split(subnet[0], ".")
	if len(octets)!=4 {
		glog.V(2).Infof("Error in parsing subnet string: invalid ip %s.", subnet[0])
		return
	}
	for i := 0; i <= 3; i++ {
		oct,e := strconv.ParseUint(octets[i], 10, 8)
		if (e != nil || oct<0 || oct > 255) {
			glog.V(2).Infof("Error in parsing subnet string: invalid IP octet %s.", octets[i])
			return
		}
		ipsubnet.octets[i] = Octet(oct)
	}
}

func (macsubnet *Subnet) InitMacSubnet(sixoctets string) {

	subnet := strings.Split(sixoctets, "/")
	// if mask is missing it will be assumed to be /24
	if len(subnet)==1 {
		macsubnet.mask = 16
	} else {
		mask,e := strconv.Atoi(subnet[1])
		if (e!=nil || (mask!=24 && mask!=32 && mask!=16 && mask!=8)) {
			// error in argument
			glog.V(2).Infof("Error in parsing subnet string: invalid range %s.", subnet[1])
			return
		}
		macsubnet.mask = mask
	}
	octets := strings.Split(subnet[0], ":")
	if len(octets)!=6 {
		glog.V(2).Infof("Error in parsing subnet string: invalid mac address mask %s.", subnet[0])
		return
	}
	for i := 0; i <= 5; i++ {
		oct,e := strconv.ParseUint(octets[i], 16, 8)
		if (e != nil || oct<0 || oct > 0xff) {
			glog.V(2).Infof("Error in parsing subnet string: invalid MAC octet %s.", octets[i])
			return
		}
		macsubnet.octets[i] = Octet(oct)
	}
}

func (ipsubnet *Subnet) GetNextIPAddress() (string, error) {
	// Assume that there are 4 octets
	endOctet := ipsubnet.mask/8
	for i := 3; i>=0; i-- {
		if endOctet>i {
			errstr := "IP Range exhausted. Re-furbish the subnet, or mask."
			glog.V(2).Infof(errstr)
			return "", errors.New(errstr)
		}
		ipsubnet.octets[i] += 1
		if ipsubnet.octets[i]==0 {
			ipsubnet.octets[i] += 1
		} else {
			return ipsubnet.ToIPStr(), nil
		}
	}
	return "", errors.New("Entire IPv4 range exhausted.")
}

func (macsubnet *Subnet) GetNextMacAddress() (string, error) {
	// Assume that there are 6 octets
	endOctet := macsubnet.mask/8
	for i := 5; i>=0; i-- {
		if endOctet>i {
			errstr := "MAC Range exhausted. Re-furbish the subnet, or mask."
			glog.V(2).Infof(errstr)
			return "", errors.New(errstr)
		}
		macsubnet.octets[i] += 1
		if macsubnet.octets[i]==0 {
			macsubnet.octets[i] += 1
		} else {
			return macsubnet.ToMacStr(), nil
		}
	}
	return "", errors.New("All Mac Addresses are exhausted.")
}

func (ns *NetScheduler) GetChkSum(str string) int {
	return int(md5.Sum([]byte(str))[0])
}

func (ns *NetScheduler) AllocateNetBinding(pod *api.Pod) (*api.NetBinding, error) {
	i,ierr := ns.ipSubnet.GetNextIPAddress()
	if ierr != nil {
		glog.V(2).Infof("Error allocating IP address to pod %v", pod)
		return nil,ierr
	}
	m,merr := ns.macSubnet.GetNextMacAddress()
	if merr != nil {
		glog.V(2).Infof("Error allocating Mac address to pod %v", pod)
		return nil,merr
	}
	bridgePort := (ns.bridgePortPool[len(ns.bridgePortPool)-1]+1)
	vnid := ns.GetChkSum(pod.Namespace)
	fmt.Printf("Bridge port : %d\n", bridgePort)
	ns.bridgePortPool = append(ns.bridgePortPool, bridgePort)
	netbinding := &api.NetBinding {
			Vtep: "",
			PodID: pod.Name,
			IPAddress: i,
			MacAddress: m,
			NetID: vnid,
			BridgePort: bridgePort,
		}
	netbinding.Namespace = pod.Namespace
	return netbinding,nil
}
