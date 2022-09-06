package aws

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"

	"terraform-percona/internal/service"
)

func (c *Cloud) DeleteInfrastructure(ctx context.Context, resourceId string) error {
	resourceGroupingClient := resourcegroupstaggingapi.New(c.session)
	getResourcesOutput, err := resourceGroupingClient.GetResourcesWithContext(ctx, &resourcegroupstaggingapi.GetResourcesInput{
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
			describeInstanceOutput, err := c.client.DescribeInstancesWithContext(ctx, &ec2.DescribeInstancesInput{
				InstanceIds: []*string{aws.String(resource[1])},
			})
			if err != nil {
				return err
			}
			if describeInstanceOutput.Reservations == nil {
				break
			}
			if _, err := c.client.TerminateInstancesWithContext(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: []*string{aws.String(resource[1])},
			}); err != nil {
				return err
			}
			time.Sleep(20 * time.Second)
		case ec2.ResourceTypeSubnet:
			if _, err := c.client.DeleteSubnetWithContext(ctx, &ec2.DeleteSubnetInput{
				SubnetId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeSecurityGroup:
			_, err = c.client.RevokeSecurityGroupIngressWithContext(ctx, &ec2.RevokeSecurityGroupIngressInput{
				GroupId: aws.String(resource[1]),
				IpPermissions: []*ec2.IpPermission{
					(&ec2.IpPermission{}).
						SetIpProtocol("-1").
						SetFromPort(-1).
						SetToPort(-1).
						SetIpRanges([]*ec2.IpRange{
							{CidrIp: aws.String(service.AllAddressesCidrBlock)},
						}),
				},
			})
			if err != nil {
				return err
			}
			time.Sleep(time.Second * 60)

			_, err = c.client.RevokeSecurityGroupEgressWithContext(ctx, &ec2.RevokeSecurityGroupEgressInput{
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

			if _, err := c.client.DeleteSecurityGroupWithContext(ctx, &ec2.DeleteSecurityGroupInput{
				GroupId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeInternetGateway:
			if _, err = c.client.DetachInternetGatewayWithContext(ctx, &ec2.DetachInternetGatewayInput{
				InternetGatewayId: aws.String(resource[1]),
				VpcId:             aws.String(vpcId),
			}); err != nil {
				return err
			}
			if _, err := c.client.DeleteInternetGatewayWithContext(ctx, &ec2.DeleteInternetGatewayInput{
				InternetGatewayId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeKeyPair:
			if _, err := c.client.DeleteKeyPairWithContext(ctx, &ec2.DeleteKeyPairInput{
				KeyPairId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeRouteTable:
			if _, err := c.client.DeleteRouteWithContext(ctx, &ec2.DeleteRouteInput{
				DestinationCidrBlock: aws.String(service.AllAddressesCidrBlock),
				RouteTableId:         aws.String(resource[1]),
			}); err != nil {
				return err
			}
			describeRouteTableOutput, err := c.client.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{
				RouteTableIds: []*string{aws.String(resource[1])},
			})
			if err != nil {
				return err
			}
			if len(describeRouteTableOutput.RouteTables) > 0 {
				if len(describeRouteTableOutput.RouteTables[0].Associations) > 0 {
					if _, err := c.client.DisassociateRouteTableWithContext(ctx, &ec2.DisassociateRouteTableInput{
						AssociationId: describeRouteTableOutput.RouteTables[0].Associations[0].RouteTableAssociationId,
					}); err != nil {
						return err
					}
				}
			}
			if _, err := c.client.DeleteRouteTableWithContext(ctx, &ec2.DeleteRouteTableInput{
				RouteTableId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		case ec2.ResourceTypeVpc:
			if _, err := c.client.DeleteVpcWithContext(ctx, &ec2.DeleteVpcInput{
				VpcId: aws.String(resource[1]),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
