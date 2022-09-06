package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/pkg/errors"

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
		return errors.Wrap(err, "failed to get resources")
	}
	resources := map[string][]string{}
	for _, m := range getResourcesOutput.ResourceTagMappingList {
		if arn.IsARN(*m.ResourceARN) {
			parsedArn, err := arn.Parse(*m.ResourceARN)
			if err != nil {
				return errors.Wrap(err, "failed to parse arn")
			}
			resource := strings.Split(parsedArn.Resource, "/")
			resourceType := resource[0]
			resourceID := resource[1]
			resources[resourceType] = append(resources[resourceType], resourceID)
		}
	}

	// Delete Instances
	var instanceIDs []*string
	for _, id := range resources[ec2.ResourceTypeInstance] {
		instanceIDs = append(instanceIDs, aws.String(id))
	}
	if len(instanceIDs) > 0 {
		_, err = c.client.TerminateInstancesWithContext(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: instanceIDs,
		})
		if err != nil {
			return errors.Wrap(err, "failed to terminate instance")
		}
		if err = c.client.WaitUntilInstanceTerminatedWithContext(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: instanceIDs,
		}); err != nil {
			return errors.Wrap(err, "failed to wait for instance termination")
		}
	}

	// Delete Route Tables
	for _, id := range resources[ec2.ResourceTypeRouteTable] {
		out, err := c.client.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{
			RouteTableIds: []*string{aws.String(id)},
		})
		if err != nil {
			return errors.Wrap(err, "failed to describe route tables")
		}
		if len(out.RouteTables) > 0 {
			if len(out.RouteTables[0].Associations) > 0 {
				if _, err = c.client.DisassociateRouteTableWithContext(ctx, &ec2.DisassociateRouteTableInput{
					AssociationId: out.RouteTables[0].Associations[0].RouteTableAssociationId,
				}); err != nil {
					return errors.Wrap(err, "failed to disassociate route table")
				}
			}
		}
		if _, err = c.client.DeleteRouteTableWithContext(ctx, &ec2.DeleteRouteTableInput{
			RouteTableId: aws.String(id),
		}); err != nil {
			return errors.Wrap(err, "failed to delete route table")
		}
	}

	// Delete Subnets
	for _, id := range resources[ec2.ResourceTypeSubnet] {
		if _, err = c.client.DeleteSubnetWithContext(ctx, &ec2.DeleteSubnetInput{
			SubnetId: aws.String(id),
		}); err != nil {
			return errors.Wrap(err, "failed to delete subnet")
		}
	}

	// Delete Security Groups
	for _, id := range resources[ec2.ResourceTypeSecurityGroup] {
		if _, err = c.client.DeleteSecurityGroupWithContext(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: aws.String(id),
		}); err != nil {
			return errors.Wrap(err, "failed to delete security group")
		}
	}

	// Delete Internet Gateways
	for _, id := range resources[ec2.ResourceTypeInternetGateway] {
		out, err := c.client.DescribeInternetGatewaysWithContext(ctx, &ec2.DescribeInternetGatewaysInput{
			InternetGatewayIds: []*string{aws.String(id)},
		})
		if err != nil {
			return errors.Wrap(err, "describe internet gateway")
		}
		for _, attachment := range out.InternetGateways[0].Attachments {
			if _, err = c.client.DetachInternetGatewayWithContext(ctx, &ec2.DetachInternetGatewayInput{
				InternetGatewayId: aws.String(id),
				VpcId:             attachment.VpcId,
			}); err != nil {
				return errors.Wrap(err, "detach internet gateway")
			}
		}
		if _, err = c.client.DeleteInternetGatewayWithContext(ctx, &ec2.DeleteInternetGatewayInput{
			InternetGatewayId: aws.String(id),
		}); err != nil {
			return errors.Wrap(err, "delete internet gateway")
		}
	}

	// Delete VPC
	for _, id := range resources[ec2.ResourceTypeVpc] {
		if _, err = c.client.DeleteVpcWithContext(ctx, &ec2.DeleteVpcInput{
			VpcId: aws.String(id),
		}); err != nil {
			return errors.Wrap(err, "delete vpc")
		}
	}
	return nil
}
