#!/bin/sh


# ARGS : vtep ip mac netid 

vtep=$1
ip=$2
mac=$3
netid=$4

echo "Deleting flows with args : $@"

iphex=`printf '%02x' ${ip//./ }; echo`
machex=`printf '%s' ${mac//:/ }; echo`

## del flows
# arp-responder, dl_type 0x0806 means ARP
ovs-ofctl del-flows -O OpenFlow13 obr0 "table=1,cookie=${netid}/0xffffffff,tun_id=${netid},dl_type=0x0806,nw_dst=${ip}"

# destination packet redirect
ovs-ofctl del-flows -O OpenFlow13 obr0 "table=1,cookie=${netid}/0xffffffff,tun_id=${netid},dl_dst=${mac}"

echo "Done"
