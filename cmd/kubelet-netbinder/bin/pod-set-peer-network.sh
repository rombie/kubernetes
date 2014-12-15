#!/bin/sh


# ARGS : vtep ip mac netid 

vtep=$1
ip=$2
mac=$3
netid=$4

echo "Fixing flows with args : $@"

iphex=`printf '%02x' ${ip//./ }; echo`
machex=`printf '%s' ${mac//:/ }; echo`

## add flows
# arp-responder, dl_type 0x0806 means ARP
ovs-ofctl add-flow -O OpenFlow13 obr0 "table=1,cookie=${netid},priority=200,tun_id=${netid},dl_type=0x0806,nw_dst=${ip}, actions=move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[], mod_dl_src:${mac}, load:0x2->NXM_OF_ARP_OP[], move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[], move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[], load:0x${machex}->NXM_NX_ARP_SHA[], load:0x${iphex}->NXM_OF_ARP_SPA[],in_port"

# destination packet redirect
ovs-ofctl add-flow -O OpenFlow13 obr0 "table=1,cookie=${netid},priority=200,tun_id=${netid},dl_dst=${mac},actions=set_field:${netid}->tun_id,set_field:${vtep}->tun_dst,output:10"

echo "Done"
