def nametag: .Tags // [{Key: "Name", Value: "??"}] |
             (.[] | select(.Key == "Name")).Value;


def vpcmap: .Vpcs[] | {
                (.VpcId): .CidrBlock
            };

def vpcnames: .Vpcs[] | {
                (.VpcId): nametag
            };

def vpcs($vpcs): ($vpcs | add) as $vpcmap | ($ips | add) as $ipmap | ($vpcnames | add) as $vpcnamemap |
          .Subnets | map({
              name: $vpcnamemap[.VpcId],
              cidr: .CidrBlock,
              vpc: .VpcId,
              usedIps: ($ipmap[.SubnetId] // []),
              vpcCidr: $vpcmap[.VpcId],
              az: .AvailabilityZone
          }) |
          group_by(.vpc)[] | {
              vpc: .[0].vpc,
              name: .[0].name,
              cidr: .[0].vpcCidr,
              subnets: .
          };

def used_ips: [
                  .NetworkInterfaces | map({
                      ip: .PrivateIpAddresses[].PrivateIpAddress,
                      SubnetId
                  })
                  |
                 group_by(.SubnetId)[] | {
                    (.[0].SubnetId): (map(.ip))
                 }
              ] | add;