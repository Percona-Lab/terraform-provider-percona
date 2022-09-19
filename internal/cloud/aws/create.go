package aws

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"

	"terraform-percona/internal/resource"
	"terraform-percona/internal/utils"
)

func (c *Cloud) Configure(_ context.Context, resourceId string, data *schema.ResourceData) error {
	if c.configs == nil {
		c.configs = make(map[string]*resourceConfig)
	}
	if _, ok := c.configs[resourceId]; !ok {
		c.configs[resourceId] = &resourceConfig{}
	}
	cfg := c.configs[resourceId]
	cfg.keyPair = aws.String(data.Get(resource.KeyPairName).(string))
	cfg.pathToKeyPair = aws.String(data.Get(resource.PathToKeyPairStorage).(string))
	cfg.instanceType = aws.String(data.Get(resource.InstanceType).(string))
	cfg.volumeType = aws.String(data.Get(resource.VolumeType).(string))
	if aws.StringValue(cfg.volumeType) == "" {
		cfg.volumeType = aws.String("gp2")
	}
	cfg.volumeSize = aws.Int64(int64(data.Get(resource.VolumeSize).(int)))
	if v, ok := data.Get(resource.VolumeIOPS).(int); ok {
		if v != 0 {
			cfg.volumeIOPS = aws.Int64(int64(v))
		}
	}
	cfg.vpcName = aws.String(data.Get(resource.VPCName).(string))

	if c.Region != nil {
		if ami, ok := mapRegionImage[aws.StringValue(c.Region)]; ok {
			cfg.ami = aws.String(ami)
		} else {
			return errors.Errorf("can't find any AMI for region %s", aws.StringValue(c.Region))
		}
	}

	var err error
	c.session, err = session.NewSession(&aws.Config{
		Region: c.Region,
	})
	if err != nil {
		return errors.Wrap(err, "failed create aws session")
	}
	c.client = ec2.New(c.session)
	return nil
}

func (c *Cloud) CreateInfrastructure(ctx context.Context, resourceId string) error {
	if err := c.createKeyPair(ctx, resourceId); err != nil {
		return err
	}

	vpc, err := c.createOrGetVPC(ctx, resourceId)
	if err != nil {
		return err
	}

	internetGateway, err := c.createOrGetInternetGateway(ctx, vpc, resourceId)
	if err != nil {
		return err
	}

	securityGroupId, err := c.createOrGetSecurityGroup(ctx, vpc, resourceId)
	if err != nil {
		return err
	}

	subnet, err := c.createOrGetSubnet(ctx, vpc, resourceId)
	if err != nil {
		return err
	}

	_, err = c.createOrGetRouteTable(ctx, vpc, internetGateway, subnet, resourceId)
	if err != nil {
		return err
	}

	c.configs[resourceId].securityGroupID = securityGroupId
	c.configs[resourceId].subnetID = subnet.SubnetId
	return nil
}

func (c *Cloud) createKeyPair(ctx context.Context, resourceId string) error {
	cfg := c.configs[resourceId]
	if aws.StringValue(cfg.keyPair) == "" {
		return errors.New("cannot create key pair with empty name")
	}

	keyPairPath, err := c.keyPairPath(resourceId)
	if err != nil {
		return errors.Wrap(err, "failed to get key pair path")
	}

	pairs, err := c.client.DescribeKeyPairsWithContext(ctx, &ec2.DescribeKeyPairsInput{
		KeyNames:         []*string{cfg.keyPair},
		IncludePublicKey: aws.Bool(true),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() != "InvalidKeyPair.NotFound" {
				return errors.Wrap(err, "failed describe key pairs")
			}
		} else {
			return errors.Wrap(err, "failed describe key pairs")
		}
	}
	if _, err = os.Stat(keyPairPath); err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "failed to check key pair file")
		}
		if len(pairs.KeyPairs) > 0 {
			return errors.New("ssh key pair does not exist locally, but exists in AWS")
		}
	}
	pubKey, err := utils.GetSSHPublicKey(keyPairPath)
	if err != nil {
		return errors.Wrap(err, "failed to get public key")
	}
	if len(pairs.KeyPairs) > 0 {
		awsPublicKey := aws.StringValue(pairs.KeyPairs[0].PublicKey)
		parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(awsPublicKey))
		if err != nil {
			return err
		}
		cleanKey := string(ssh.MarshalAuthorizedKey(parsedKey))
		if cleanKey != pubKey {
			return errors.New("local public key does not match with existing key in AWS")
		}
		return nil
	}
	_, err = c.client.ImportKeyPairWithContext(ctx, &ec2.ImportKeyPairInput{
		KeyName:           cfg.keyPair,
		PublicKeyMaterial: []byte(pubKey),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeKeyPair),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(resource.TagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to import key pair")
	}
	return nil
}

func (c *Cloud) createOrGetVPC(ctx context.Context, resourceId string) (*ec2.Vpc, error) {
	cfg := c.configs[resourceId]
	name := aws.StringValue(cfg.vpcName)
	if name != "" {
		out, err := c.client.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(name)},
			}},
		})
		if err != nil {
			return nil, errors.Wrap(err, "describe vpc")
		}
		if len(out.Vpcs) > 0 {
			return out.Vpcs[0], nil
		}
	}
	in := &ec2.CreateVpcInput{
		CidrBlock: aws.String(resource.DefaultVpcCidrBlock),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeVpc),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(resource.TagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	}
	if name != "" {
		in.TagSpecifications[0].Tags = append(in.TagSpecifications[0].Tags, &ec2.Tag{
			Key:   aws.String("Name"),
			Value: aws.String(name),
		})
	}

	createVpcOutput, err := c.client.CreateVpcWithContext(ctx, in)
	if err != nil {
		return nil, errors.Wrap(err, "error occurred during vpc creating")
	}

	if _, err = c.client.ModifyVpcAttributeWithContext(ctx, &ec2.ModifyVpcAttributeInput{
		EnableDnsHostnames: &ec2.AttributeBooleanValue{Value: aws.Bool(true)},
		VpcId:              createVpcOutput.Vpc.VpcId,
	}); err != nil {
		return nil, errors.Wrapf(err, "failed modify vpc %s", aws.StringValue(createVpcOutput.Vpc.VpcId))
	}
	return createVpcOutput.Vpc, nil
}

func (c *Cloud) createOrGetInternetGateway(ctx context.Context, vpc *ec2.Vpc, resourceId string) (*ec2.InternetGateway, error) {
	cfg := c.configs[resourceId]
	vpcName := aws.StringValue(cfg.vpcName)
	var name string
	if vpcName != "" {
		name = vpcName + "-igw"
		out, err := c.client.DescribeInternetGatewaysWithContext(ctx, &ec2.DescribeInternetGatewaysInput{
			Filters: []*ec2.Filter{{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(name)},
			}},
		})
		if err != nil {
			return nil, errors.Wrap(err, "describe internet gateway")
		}
		if len(out.InternetGateways) > 0 {
			return out.InternetGateways[0], nil
		}
	}
	in := &ec2.CreateInternetGatewayInput{
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeInternetGateway),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(resource.TagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	}
	if name != "" {
		in.TagSpecifications[0].Tags = append(in.TagSpecifications[0].Tags, &ec2.Tag{
			Key:   aws.String("Name"),
			Value: aws.String(name),
		})
	}
	out, err := c.client.CreateInternetGatewayWithContext(ctx, in)
	if err != nil {
		return nil, errors.Wrap(err, "failed create internet gateway")
	}

	if _, err = c.client.AttachInternetGatewayWithContext(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: out.InternetGateway.InternetGatewayId,
		VpcId:             vpc.VpcId,
	}); err != nil {
		return nil, errors.Wrapf(err, "failed attach internet gateway to Vpc %s", *vpc.VpcId)
	}

	return out.InternetGateway, nil
}

func (c *Cloud) createOrGetSecurityGroup(ctx context.Context, vpc *ec2.Vpc, resourceId string) (*string, error) {
	cfg := c.configs[resourceId]
	vpcName := aws.StringValue(cfg.vpcName)
	var name string
	if vpcName != "" {
		name = vpcName + "-sg"
	} else {
		name = DefaultSecurityGroupName
	}

	out, err := c.client.DescribeSecurityGroupsWithContext(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{{
			Name:   aws.String("group-name"),
			Values: []*string{aws.String(name)},
		}},
	})
	if err != nil {
		return nil, errors.Wrap(err, "describe security group")
	}
	if len(out.SecurityGroups) > 0 {
		return out.SecurityGroups[0].GroupId, nil
	}

	createSecurityGroupOutput, err := c.client.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String(DefaultSecurityGroupDescription),
		VpcId:       vpc.VpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSecurityGroup),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(resource.TagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create security group %s", name)
	}

	if _, err = c.client.AuthorizeSecurityGroupIngressWithContext(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: createSecurityGroupOutput.GroupId,
		IpPermissions: []*ec2.IpPermission{{
			IpProtocol: aws.String("-1"),
			FromPort:   aws.Int64(-1),
			ToPort:     aws.Int64(-1),
			IpRanges: []*ec2.IpRange{{
				CidrIp: aws.String(resource.AllAddressesCidrBlock),
			}},
		}},
	}); err != nil {
		return nil, errors.Wrap(err, "failed authorize security group ingress traffic")
	}

	if _, err = c.client.AuthorizeSecurityGroupEgressWithContext(ctx, &ec2.AuthorizeSecurityGroupEgressInput{
		GroupId: createSecurityGroupOutput.GroupId,
		IpPermissions: []*ec2.IpPermission{
			{
				IpProtocol: aws.String("-1"),
				FromPort:   aws.Int64(-1),
				ToPort:     aws.Int64(-1),
			}},
	}); err != nil {
		return nil, errors.Wrap(err, "failed authorize security group egress traffic")
	}

	return createSecurityGroupOutput.GroupId, nil
}

func (c *Cloud) createOrGetSubnet(ctx context.Context, vpc *ec2.Vpc, resourceId string) (*ec2.Subnet, error) {
	cfg := c.configs[resourceId]
	vpcName := aws.StringValue(cfg.vpcName)
	var name string
	if vpcName != "" {
		name = vpcName + "-subnet"
		out, err := c.client.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{
			Filters: []*ec2.Filter{{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(name)},
			}},
		})
		if err != nil {
			return nil, errors.Wrap(err, "describe subnet")
		}
		if len(out.Subnets) > 0 {
			return out.Subnets[0], nil
		}
	}
	in := &ec2.CreateSubnetInput{
		VpcId:     vpc.VpcId,
		CidrBlock: aws.String(DefaultSubnetCidrBlock),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSubnet),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(resource.TagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	}
	if name != "" {
		in.TagSpecifications[0].Tags = append(in.TagSpecifications[0].Tags, &ec2.Tag{
			Key:   aws.String("Name"),
			Value: aws.String(name),
		})
	}
	createSubnetOutput, err := c.client.CreateSubnetWithContext(ctx, in)
	if err != nil {
		return nil, errors.Wrap(err, "failed create subnet")
	}
	return createSubnetOutput.Subnet, nil
}

func (c *Cloud) createOrGetRouteTable(ctx context.Context, vpc *ec2.Vpc, gateway *ec2.InternetGateway, subnet *ec2.Subnet, resourceId string) (*ec2.RouteTable, error) {
	cfg := c.configs[resourceId]
	vpcName := aws.StringValue(cfg.vpcName)
	var name string
	if vpcName != "" {
		name = vpcName + "-rtb"
		out, err := c.client.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{
			Filters: []*ec2.Filter{{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(name)},
			}},
		})
		if err != nil {
			return nil, errors.Wrap(err, "describe route table")
		}
		if len(out.RouteTables) > 0 {
			return out.RouteTables[0], nil
		}
	}
	in := &ec2.CreateRouteTableInput{
		VpcId: vpc.VpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeRouteTable),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(resource.TagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	}
	if name != "" {
		in.TagSpecifications[0].Tags = append(in.TagSpecifications[0].Tags, &ec2.Tag{
			Key:   aws.String("Name"),
			Value: aws.String(name),
		})
	}
	out, err := c.client.CreateRouteTableWithContext(ctx, in)
	if err != nil {
		return nil, errors.Wrap(err, "failed create route table")
	}

	if _, err = c.client.CreateRouteWithContext(ctx, &ec2.CreateRouteInput{
		DestinationCidrBlock: aws.String(resource.AllAddressesCidrBlock),
		GatewayId:            gateway.InternetGatewayId,
		RouteTableId:         out.RouteTable.RouteTableId,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to create route")
	}

	if _, err = c.client.AssociateRouteTableWithContext(ctx, &ec2.AssociateRouteTableInput{
		RouteTableId: out.RouteTable.RouteTableId,
		SubnetId:     subnet.SubnetId,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to associate route table")
	}
	return out.RouteTable, nil
}
