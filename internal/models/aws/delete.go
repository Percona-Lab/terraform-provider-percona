package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"sort"
	"strings"
	"terraform-percona/internal/service"
	"time"
)

func (cloud *Cloud) DeleteInfrastructure(resourceId string) error {
	resourceGroupingClient := resourcegroupstaggingapi.New(cloud.session)
	getResourcesOutput, err := resourceGroupingClient.GetResources(&resourcegroupstaggingapi.GetResourcesInput{
		TagFilters: []*resourcegroupstaggingapi.TagFilter{
			{
				Key:    aws.String(service.ClusterResourcesTagName),
				Values: []*string{aws.String(resourceId)},
			},
		},
	})
	if err != nil {
		return err
	}

	//if sshKeyFile, ok := data.Get(awsModel.KeyPairName).(string); ok {
	//	if path, ok := data.Get(awsModel.PathToKeyPairStorage).(string); ok {
	//		sshKeyPath := fmt.Sprintf("%s%s.sh", path, sshKeyFile)
	//		if _, err := os.Stat(sshKeyPath); err == nil {
	//			if err := os.Remove(sshKeyPath); err != nil {
	//				return diag.FromErr(fmt.Errorf("failed delete ssh file: path:%s, error:%w", sshKeyPath, err))
	//			}
	//		} else if !errors.Is(err, os.ErrNotExist) {
	//			return diag.FromErr(fmt.Errorf("failed describe ssh file: path:%s, error:%w", sshKeyPath, err))
	//		}
	//	}
	//}

	var resources []string
	for _, m := range getResourcesOutput.ResourceTagMappingList {
		if arn.IsARN(*m.ResourceARN) {
			parsedArn, err := arn.Parse(*m.ResourceARN)
			if err != nil {
				return err
			}
			resources = append(resources, parsedArn.Resource)
		}
	}

	var vpcId string
	sort.Slice(resources, func(i, j int) bool {
		iResource := strings.Split(resources[i], "/")
		jResource := strings.Split(resources[j], "/")

		iResourceType := iResource[0]
		jResourceType := jResource[0]

		if iResourceType == ec2.ResourceTypeVpc {
			vpcId = iResource[1]
		}

		switch iResourceType {
		case ec2.ResourceTypeInstance:
			return true
		case ec2.ResourceTypeKeyPair:
			return true
		case ec2.ResourceTypeVpc:
			return false
		case ec2.ResourceTypeRouteTable:
			switch jResourceType {
			case ec2.ResourceTypeInternetGateway, ec2.ResourceTypeSubnet, ec2.ResourceTypeVpc:
				return true
			default:
				return false
			}
		case ec2.ResourceTypeInternetGateway, ec2.ResourceTypeSubnet:
			if jResourceType == ec2.ResourceTypeVpc {
				return true
			}
			return false
		case ec2.ResourceTypeSecurityGroup:
			switch jResourceType {
			case ec2.ResourceTypeInstance:
				return false
			default:
				return true
			}
		default:
			return false
		}
	})

	for _, v := range resources {
		resource := strings.Split(v, "/")
		switch resource[0] {
		case ec2.ResourceTypeInstance:
			describeInstanceOutput, err := cloud.client.DescribeInstances(&ec2.DescribeInstancesInput{
				InstanceIds: []*string{aws.String(resource[1])},
			})
			if err != nil {
				return err
			}
			if describeInstanceOutput.Reservations == nil {
				break
			}
			if _, err := cloud.client.TerminateInstances(&ec2.TerminateInstancesInput{
				InstanceIds: []*string{aws.String(resource[1])},
			}); err != nil {
				return err
			}
			time.Sleep(20 * time.Second)
		case ec2.ResourceTypeSubnet:
			if _, err := cloud.client.DeleteSubnet(&ec2.DeleteSubnetInput{
				SubnetId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeSecurityGroup:
			_, err = cloud.client.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
				GroupId: aws.String(resource[1]),
				IpPermissions: []*ec2.IpPermission{
					(&ec2.IpPermission{}).
						SetIpProtocol("-1").
						SetFromPort(-1).
						SetToPort(-1).
						SetIpRanges([]*ec2.IpRange{
							{CidrIp: aws.String(AllAddressesCidrBlock)},
						}),
				},
			})
			if err != nil {
				return err
			}
			time.Sleep(time.Second * 60)

			_, err = cloud.client.RevokeSecurityGroupEgress(&ec2.RevokeSecurityGroupEgressInput{
				GroupId: aws.String(resource[1]),
				IpPermissions: []*ec2.IpPermission{
					(&ec2.IpPermission{}).
						SetIpProtocol("-1").
						SetFromPort(-1).
						SetToPort(-1),
				},
			})
			if err != nil {
				return err
			}
			time.Sleep(time.Second * 60)

			if _, err := cloud.client.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
				GroupId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeInternetGateway:
			if _, err = cloud.client.DetachInternetGateway(&ec2.DetachInternetGatewayInput{
				InternetGatewayId: aws.String(resource[1]),
				VpcId:             aws.String(vpcId),
			}); err != nil {
				return err
			}
			if _, err := cloud.client.DeleteInternetGateway(&ec2.DeleteInternetGatewayInput{
				InternetGatewayId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeKeyPair:
			if _, err := cloud.client.DeleteKeyPair(&ec2.DeleteKeyPairInput{
				KeyPairId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeRouteTable:
			if _, err := cloud.client.DeleteRoute(&ec2.DeleteRouteInput{
				DestinationCidrBlock: aws.String(AllAddressesCidrBlock),
				RouteTableId:         aws.String(resource[1]),
			}); err != nil {
				return err
			}
			describeRouteTableOutput, err := cloud.client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
				RouteTableIds: []*string{aws.String(resource[1])},
			})
			if err != nil {
				return err
			}
			if len(describeRouteTableOutput.RouteTables) > 0 {
				if len(describeRouteTableOutput.RouteTables[0].Associations) > 0 {
					if _, err := cloud.client.DisassociateRouteTable(&ec2.DisassociateRouteTableInput{
						AssociationId: describeRouteTableOutput.RouteTables[0].Associations[0].RouteTableAssociationId,
					}); err != nil {
						return err
					}
				}
			}
			if _, err := cloud.client.DeleteRouteTable(&ec2.DeleteRouteTableInput{
				RouteTableId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeVpc:
			if _, err := cloud.client.DeleteVpc(&ec2.DeleteVpcInput{
				VpcId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
