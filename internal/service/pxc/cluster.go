package pxc

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"sort"
	"strings"
	awsModel "terraform-percona/internal/models/aws"
	"terraform-percona/internal/utils"
	"time"
)

func ResourceInstance() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceInitCluster,
		ReadContext:   resourceInstanceRead,
		UpdateContext: resourceInstanceUpdate,
		DeleteContext: resourceInstanceDelete,

		Schema: map[string]*schema.Schema{
			awsModel.Ami: {
				Type:         schema.TypeString,
				Required:     true,
				InputDefault: "ami-0d527b8c289b4af7f",
			},
			awsModel.InstanceType: {
				Type:         schema.TypeString,
				Required:     true,
				InputDefault: "t4g.nano",
			},
			awsModel.KeyPairName: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "pxc",
			},
			awsModel.PathToClusterBootstrapScript: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "./bootstrap.sh",
			},
			awsModel.PathToKeyPairStorage: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "/tmp/",
			},
			awsModel.ClusterSize: {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  3,
			},
			awsModel.MinCount: {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
			awsModel.MaxCount: {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
		},
	}
}

func resourceInitCluster(_ context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	awsController, ok := meta.(*awsModel.AWSController)
	if !ok {
		return diag.Errorf("nil aws controller")
	}

	xtraDBClusterManager := awsController.XtraDBClusterManager
	if v, ok := data.Get(awsModel.Ami).(string); ok {
		xtraDBClusterManager.Config.Ami = aws.String(v)
	}
	if v, ok := data.Get(awsModel.InstanceType).(string); ok {
		xtraDBClusterManager.Config.InstanceType = aws.String(v)
	}
	if v, ok := data.Get(awsModel.MinCount).(int); ok {
		xtraDBClusterManager.Config.MinCount = aws.Int64(int64(v))
	}
	if v, ok := data.Get(awsModel.MaxCount).(int); ok {
		xtraDBClusterManager.Config.MaxCount = aws.Int64(int64(v))
	}
	if v, ok := data.Get(awsModel.KeyPairName).(string); ok {
		xtraDBClusterManager.Config.KeyPairName = aws.String(v)
	}
	if v, ok := data.Get(awsModel.PathToClusterBootstrapScript).(string); ok {
		xtraDBClusterManager.Config.PathToClusterBoostrapScript = aws.String(v)
	}
	if v, ok := data.Get(awsModel.PathToKeyPairStorage).(string); ok {
		xtraDBClusterManager.Config.PathToKeyPairStorage = aws.String(v)
	}
	if v, ok := data.Get(awsModel.ClusterSize).(int); ok {
		xtraDBClusterManager.Config.ClusterSize = aws.Int64(int64(v))
	}

	resourceId := utils.GetRandomString(awsModel.ResourceIdLen)
	if _, err := xtraDBClusterManager.CreateCluster(resourceId); err != nil {
		return diag.Errorf("Error occurred during cluster creating: %w", err)
	}

	//TODO add creation of terraform resource id
	data.SetId(resourceId)
	return nil
}

func resourceInstanceRead(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func resourceInstanceUpdate(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func resourceInstanceDelete(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	awsController, ok := meta.(*awsModel.AWSController)
	if !ok {
		return diag.Errorf("nil aws controller")
	}

	resourceId := data.Id()
	if resourceId == "" {
		return diag.FromErr(fmt.Errorf("empty resource id"))
	}

	xtraDBClusterManager := awsController.XtraDBClusterManager
	resourceGroupingClient := resourcegroupstaggingapi.New(xtraDBClusterManager.Session)
	getResourcesOutput, err := resourceGroupingClient.GetResources(&resourcegroupstaggingapi.GetResourcesInput{
		TagFilters: []*resourcegroupstaggingapi.TagFilter{
			{
				Key:    aws.String(awsModel.ClusterResourcesTagName),
				Values: []*string{aws.String(resourceId)},
			},
		},
	})
	if err != nil {
		return diag.FromErr(err)
	}

	var resources []string
	for _, m := range getResourcesOutput.ResourceTagMappingList {
		if arn.IsARN(*m.ResourceARN) {
			parsedArn, err := arn.Parse(*m.ResourceARN)
			if err != nil {
				return diag.FromErr(err)
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
			describeInstanceOutput, err := xtraDBClusterManager.Client.DescribeInstances(&ec2.DescribeInstancesInput{
				InstanceIds: []*string{aws.String(resource[1])},
			})
			if err != nil {
				return diag.FromErr(err)
			}
			if describeInstanceOutput.Reservations == nil {
				break
			}
			if _, err := xtraDBClusterManager.Client.TerminateInstances(&ec2.TerminateInstancesInput{
				InstanceIds: []*string{aws.String(resource[1])},
			}); err != nil {
				return diag.FromErr(err)
			}
			time.Sleep(20 * time.Second)
		case ec2.ResourceTypeSubnet:
			if _, err := xtraDBClusterManager.Client.DeleteSubnet(&ec2.DeleteSubnetInput{
				SubnetId: aws.String(resource[1]),
			}); err != nil {
				return diag.FromErr(err)
			}
		case ec2.ResourceTypeSecurityGroup:
			_, err = xtraDBClusterManager.Client.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
				GroupId: aws.String(resource[1]),
				IpPermissions: []*ec2.IpPermission{
					(&ec2.IpPermission{}).
						SetIpProtocol("-1").
						SetFromPort(-1).
						SetToPort(-1).
						SetIpRanges([]*ec2.IpRange{
							{CidrIp: aws.String(awsModel.AllAddressesCidrBlock)},
						}),
				},
			})
			if err != nil {
				return diag.FromErr(err)
			}
			time.Sleep(time.Second * 10)

			_, err = xtraDBClusterManager.Client.RevokeSecurityGroupEgress(&ec2.RevokeSecurityGroupEgressInput{
				GroupId: aws.String(resource[1]),
				IpPermissions: []*ec2.IpPermission{
					(&ec2.IpPermission{}).
						SetIpProtocol("-1").
						SetFromPort(-1).
						SetToPort(-1),
				},
			})
			if err != nil {
				return diag.FromErr(err)
			}
			time.Sleep(time.Second * 10)

			if _, err := xtraDBClusterManager.Client.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
				GroupId: aws.String(resource[1]),
			}); err != nil {
				return diag.FromErr(err)
			}
		case ec2.ResourceTypeInternetGateway:
			if _, err = xtraDBClusterManager.Client.DetachInternetGateway(&ec2.DetachInternetGatewayInput{
				InternetGatewayId: aws.String(resource[1]),
				VpcId:             aws.String(vpcId),
			}); err != nil {
				return diag.FromErr(err)
			}
			if _, err := xtraDBClusterManager.Client.DeleteInternetGateway(&ec2.DeleteInternetGatewayInput{
				InternetGatewayId: aws.String(resource[1]),
			}); err != nil {
				return diag.FromErr(err)
			}
		case ec2.ResourceTypeKeyPair:
			if _, err := xtraDBClusterManager.Client.DeleteKeyPair(&ec2.DeleteKeyPairInput{
				KeyPairId: aws.String(resource[1]),
			}); err != nil {
				return diag.FromErr(err)
			}
		case ec2.ResourceTypeRouteTable:
			if _, err := xtraDBClusterManager.Client.DeleteRoute(&ec2.DeleteRouteInput{
				DestinationCidrBlock: aws.String(awsModel.AllAddressesCidrBlock),
				RouteTableId:         aws.String(resource[1]),
			}); err != nil {
				return diag.FromErr(err)
			}
			describeRouteTableOutput, err := xtraDBClusterManager.Client.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
				RouteTableIds: []*string{aws.String(resource[1])},
			})
			if err != nil {
				return diag.FromErr(err)
			}
			if len(describeRouteTableOutput.RouteTables) > 0 {
				if len(describeRouteTableOutput.RouteTables[0].Associations) > 0 {
					if _, err := xtraDBClusterManager.Client.DisassociateRouteTable(&ec2.DisassociateRouteTableInput{
						AssociationId: describeRouteTableOutput.RouteTables[0].Associations[0].RouteTableAssociationId,
					}); err != nil {
						return diag.FromErr(err)
					}
				}
			}
			if _, err := xtraDBClusterManager.Client.DeleteRouteTable(&ec2.DeleteRouteTableInput{
				RouteTableId: aws.String(resource[1]),
			}); err != nil {
				return diag.FromErr(err)
			}
		case ec2.ResourceTypeVpc:
			if _, err := xtraDBClusterManager.Client.DeleteVpc(&ec2.DeleteVpcInput{
				VpcId: aws.String(resource[1]),
			}); err != nil {
				return diag.FromErr(err)
			}
		}
	}
	return nil
}
