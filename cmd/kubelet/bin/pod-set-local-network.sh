#!/bin/sh


# ARGS : docker_id pod_name ip mac netid ovs_port

docker_id=$1
pod_name=$2
ip=$3
mac=$4
netid=$5
ovs_port=$6
tap_port=3

iphex=`printf '%02x' ${ip//./ }; echo`
machex=`printf '%s' ${mac//:/ }; echo`

if [ "$docker_id" != "" ]; then
	pid=`docker inspect --format "{{.State.Pid}}" ${docker_id}`
	veth_host=`jq .network_state.veth_host /var/lib/docker/execdriver/native/${docker_id}*/state.json | tr -d '"'`

	# pull veth host out of kbr0 and put it as internal port to obr0
	brctl delif kbr0 $veth_host
	ovs-vsctl add-port obr0 $veth_host -- set interface $veth_host ofport_request=${ovs_port}

	nsenter -n -t $pid -- ip link set dev eth0 down
	nsenter -n -t $pid -- ip link set dev eth0 addr $mac
	nsenter -n -t $pid -- ip addr add ${ip}/24 dev eth0
	nsenter -n -t $pid -- ip link set dev eth0 up
	nsenter -n -t $pid -- ip route set default via 10.246.1.1 dev eth0
fi

## add flows

# rule to add vnid to outbound traffic from pod
ovs-ofctl add-flow -O OpenFlow13 obr0 "table=0,cookie=${netid},priority=200,in_port=${ovs_port},actions=set_field:${netid}->tun_id,goto_table:1"
# rule to send traffic meant for this pod to the correct port
ovs-ofctl add-flow -O OpenFlow13 obr0 "table=2,cookie=${tap_port},priority=200,arp,nw_dst=${ip},actions=output:${ovs_port}"
ovs-ofctl add-flow -O OpenFlow13 obr0 "table=2,cookie=${tap_port},priority=200,ip,nw_dst=${ip},actions=output:${ovs_port}"
# rule to identify inbound traffic for pod and redirect it to correct ovs-port/veth-pair
ovs-ofctl add-flow -O OpenFlow13 obr0 "table=1,cookie=${netid},priority=200,tun_id=${netid},dl_dst=${mac},actions=output:${ovs_port}"
# rule to respond to arp requests that come locally from this host
ovs-ofctl add-flow -O OpenFlow13 obr0 "table=1,cookie=${netid},priority=200,tun_id=${netid},dl_type=0x0806,nw_dst=${ip}, actions=move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[], mod_dl_src:${mac}, load:0x2->NXM_OF_ARP_OP[], move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[], move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[], load:0x${machex}->NXM_NX_ARP_SHA[], load:0x${iphex}->NXM_OF_ARP_SPA[],in_port"


