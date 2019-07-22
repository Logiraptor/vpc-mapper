#!/bin/bash

account_name=$1

function json {
    array=( $@ )
    len=${#array[@]}
    script=${array[$len-1]}
    api=${array[$len-2]}
    args=${array[@]:0:$len-2}

    jq -L . $args "include \"util\"; $script" $account_name/*/$api.json
}

vpcs=$(json ec2-describe-vpcs vpcmap)
vpcnames=$(json ec2-describe-vpcs vpcnames)

ips=$(json ec2-describe-network-interfaces used_ips)

json \
    --slurpfile vpcs <(echo $vpcs) \
    --slurpfile vpcnames <(echo $vpcnames) \
    --slurpfile ips <(echo $ips) \
    ec2-describe-subnets 'vpcs($vpcs)' | go run main.go

