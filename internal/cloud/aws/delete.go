package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/pkg/errors"

	"terraform-percona/internal/resource"
)

func (c *Cloud) DeleteInfrastructure(ctx context.Context, resourceID string) error {
	resourceGroupingClient := resourcegroupstaggingapi.New(c.session)
	getResourcesOutput, err := resourceGroupingClient.GetResourcesWithContext(ctx, &resourcegroupstaggingapi.GetResourcesInput{
		TagFilters: []*resourcegroupstaggingapi.TagFilter{
			{
				Key:    aws.String(resource.LabelKeyResourceID),
				Values: []*string{aws.String(resourceID)},
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
			if !c.Meta.IgnoreErrorsOnDestroy {
				return errors.Wrap(err, "failed to terminate instance")
			}
			tflog.Error(ctx, "failed to terminate instance", map[string]interface{}{
				"error": err,
			})
		}
		if err = c.client.WaitUntilInstanceTerminatedWithContext(ctx, &ec2.DescribeInstancesInput{
			InstanceIds: instanceIDs,
		}); err != nil {
			if !c.Meta.IgnoreErrorsOnDestroy {
				return errors.Wrap(err, "failed to wait for instance termination")
			}
			tflog.Error(ctx, "failed to wait for instance termination", map[string]interface{}{
				"error": err,
			})
		}
	}

	// Delete Route Tables
	for _, id := range resources[ec2.ResourceTypeRouteTable] {
		out, err := c.client.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{
			RouteTableIds: []*string{aws.String(id)},
		})
		if err != nil {
			if !c.Meta.IgnoreErrorsOnDestroy {
				return errors.Wrap(err, "failed to describe route tables")
			}
			tflog.Error(ctx, "failed to describe route tables", map[string]interface{}{
				"error": err,
			})
		}
		if len(out.RouteTables) > 0 {
			if len(out.RouteTables[0].Associations) > 0 {
				if _, err = c.client.DisassociateRouteTableWithContext(ctx, &ec2.DisassociateRouteTableInput{
					AssociationId: out.RouteTables[0].Associations[0].RouteTableAssociationId,
				}); err != nil {
					return errors.Wrap(err, "failed to disassociate route table")
				}
				tflog.Error(ctx, "failed to disassociate route table", map[string]interface{}{
					"error": err,
				})
			}
		}
		if _, err = c.client.DeleteRouteTableWithContext(ctx, &ec2.DeleteRouteTableInput{
			RouteTableId: aws.String(id),
		}); err != nil {
			if !c.Meta.IgnoreErrorsOnDestroy {
				return errors.Wrap(err, "failed to delete route table")
			}
			tflog.Error(ctx, "failed to delete route table", map[string]interface{}{
				"error": err,
			})
		}
	}

	// Delete Subnets
	for _, id := range resources[ec2.ResourceTypeSubnet] {
		if _, err = c.client.DeleteSubnetWithContext(ctx, &ec2.DeleteSubnetInput{
			SubnetId: aws.String(id),
		}); err != nil {
			if !c.Meta.IgnoreErrorsOnDestroy {
				return errors.Wrap(err, "failed to delete subnet")
			}
			tflog.Error(ctx, "failed to delete subnet", map[string]interface{}{
				"error": err,
			})
		}
	}

	// Delete Security Groups
	for _, id := range resources[ec2.ResourceTypeSecurityGroup] {
		if _, err = c.client.DeleteSecurityGroupWithContext(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: aws.String(id),
		}); err != nil {
			if !c.Meta.IgnoreErrorsOnDestroy {
				return errors.Wrap(err, "failed to delete security group")
			}
			tflog.Error(ctx, "failed to delete security group", map[string]interface{}{
				"error": err,
			})
		}
	}

	// Delete Internet Gateways
	for _, id := range resources[ec2.ResourceTypeInternetGateway] {
		out, err := c.client.DescribeInternetGatewaysWithContext(ctx, &ec2.DescribeInternetGatewaysInput{
			InternetGatewayIds: []*string{aws.String(id)},
		})
		if err != nil {
			if !c.Meta.IgnoreErrorsOnDestroy {
				return errors.Wrap(err, "describe internet gateway")
			}
			tflog.Error(ctx, "describe internet gateway", map[string]interface{}{
				"error": err,
			})
		}
		for _, attachment := range out.InternetGateways[0].Attachments {
			if _, err = c.client.DetachInternetGatewayWithContext(ctx, &ec2.DetachInternetGatewayInput{
				InternetGatewayId: aws.String(id),
				VpcId:             attachment.VpcId,
			}); err != nil {
				if !c.Meta.IgnoreErrorsOnDestroy {
					return errors.Wrap(err, "detach internet gateway")
				}
				tflog.Error(ctx, "detach internet gateway", map[string]interface{}{
					"error": err,
				})
			}
		}
		if _, err = c.client.DeleteInternetGatewayWithContext(ctx, &ec2.DeleteInternetGatewayInput{
			InternetGatewayId: aws.String(id),
		}); err != nil {
			if !c.Meta.IgnoreErrorsOnDestroy {
				return errors.Wrap(err, "delete internet gateway")
			}
			tflog.Error(ctx, "delete internet gateway", map[string]interface{}{
				"error": err,
			})
		}
	}

	// Delete VPC
	for _, id := range resources[ec2.ResourceTypeVpc] {
		if _, err = c.client.DeleteVpcWithContext(ctx, &ec2.DeleteVpcInput{
			VpcId: aws.String(id),
		}); err != nil {
			if !c.Meta.IgnoreErrorsOnDestroy {
				return errors.Wrap(err, "delete vpc")
			}
			tflog.Error(ctx, "delete vpc", map[string]interface{}{
				"error": err,
			})
		}
	}
	return nil
}
