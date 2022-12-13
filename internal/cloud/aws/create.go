package aws

import (
	"context"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/utils"
)

func (c *Cloud) sourceImage(ctx context.Context) (*string, error) {
	in := &ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("name"),
				Values: []*string{aws.String("ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-*")},
			},
		},
		Owners: []*string{aws.String("amazon")},
	}
	out, err := c.client.DescribeImagesWithContext(ctx, in)
	if err != nil {
		return nil, errors.Wrap(err, "describe images")
	}
	var latestCreationDate time.Time
	var latestImage *ec2.Image

	for _, image := range out.Images {
		date, err := time.Parse("2006-01-02T15:04:05.000Z", aws.StringValue(image.CreationDate))
		if err != nil {
			return nil, errors.Wrap(err, "parse creation date")
		}
		if date.After(latestCreationDate) {
			latestCreationDate = date
			latestImage = image
		}
	}

	return latestImage.ImageId, nil
}

func (c *Cloud) Configure(ctx context.Context, resourceID string, data *schema.ResourceData) error {
	cfg := c.config(resourceID)
	if data != nil {
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
		if v, ok := data.Get(volumeThroughput).(int); ok {
			if v != 0 {
				cfg.volumeThroughput = aws.Int64(int64(v))
			}
		}
		cfg.vpcName = aws.String(data.Get(resource.VPCName).(string))
		cfg.vpcId = aws.String(data.Get(vpcID).(string))
	}
	var err error
	c.session, err = session.NewSession(&aws.Config{
		Region: c.Region,
	})
	if err != nil {
		return errors.Wrap(err, "failed create aws session")
	}
	c.client = ec2.New(c.session)
	cfg.ami, err = c.sourceImage(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get latest Ubuntu 20.04 ami")
	}
	return nil
}

func (c *Cloud) Credentials() (cloud.Credentials, error) {
	creds, err := c.session.Config.Credentials.Get()
	if err != nil {
		return cloud.Credentials{}, errors.Wrap(err, "failed to get credentials")
	}
	return cloud.Credentials{
		AccessKey: creds.AccessKeyID,
		SecretKey: creds.SecretAccessKey,
	}, nil
}

func (c *Cloud) CreateInfrastructure(ctx context.Context, resourceID string) error {
	if err := c.createKeyPair(ctx, resourceID); err != nil {
		return err
	}

	vpc, err := c.createOrGetVPC(ctx, resourceID)
	if err != nil {
		return err
	}

	internetGateway, err := c.createOrGetInternetGateway(ctx, vpc, resourceID)
	if err != nil {
		return err
	}

	securityGroupId, err := c.createOrGetSecurityGroup(ctx, vpc, resourceID)
	if err != nil {
		return err
	}

	subnet, err := c.createOrGetSubnet(ctx, vpc, resourceID)
	if err != nil {
		return err
	}

	_, err = c.createOrGetRouteTable(ctx, vpc, internetGateway, subnet, resourceID)
	if err != nil {
		return err
	}

	c.config(resourceID).securityGroupID = securityGroupId
	c.config(resourceID).subnetID = subnet.SubnetId
	return nil
}

func (c *Cloud) createKeyPair(ctx context.Context, resourceID string) error {
	cfg := c.config(resourceID)
	if aws.StringValue(cfg.keyPair) == "" {
		return errors.New("cannot create key pair with empty name")
	}

	keyPairPath, err := c.keyPairPath(resourceID)
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
						Value: aws.String(resourceID),
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

func (c *Cloud) createOrGetVPC(ctx context.Context, resourceID string) (*ec2.Vpc, error) {
	cfg := c.config(resourceID)
	name := aws.StringValue(cfg.vpcName)
	vpcID := aws.StringValue(cfg.vpcId)
	if vpcID != "" {
		out, err := c.client.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
			}},
		})
		if err != nil {
			return nil, errors.Wrap(err, "describe vpc")
		}
		if len(out.Vpcs) > 0 {
			return out.Vpcs[0], nil
		}
		tflog.Info(ctx, "VPC is not found by vpc_id", map[string]interface{}{"vpc_id": vpcID})
	}
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
						Value: aws.String(resourceID),
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

func vpcName(vpc *ec2.Vpc) string {
	for _, t := range vpc.Tags {
		if aws.StringValue(t.Key) == "Name" {
			return aws.StringValue(t.Value)
		}
	}
	return ""
}

func (c *Cloud) createOrGetInternetGateway(ctx context.Context, vpc *ec2.Vpc, resourceID string) (*ec2.InternetGateway, error) {
	vpcName := vpcName(vpc)
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
						Value: aws.String(resourceID),
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

func (c *Cloud) createOrGetSecurityGroup(ctx context.Context, vpc *ec2.Vpc, resourceID string) (*string, error) {
	vpcName := vpcName(vpc)
	var name string
	if vpcName != "" {
		name = vpcName + "-sg"
	} else {
		name = defaultSecurityGroupName
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
		Description: aws.String(defaultSecurityGroupDescription),
		VpcId:       vpc.VpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSecurityGroup),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(resource.TagName),
						Value: aws.String(resourceID),
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

func (c *Cloud) createOrGetSubnet(ctx context.Context, vpc *ec2.Vpc, resourceID string) (*ec2.Subnet, error) {
	vpcName := vpcName(vpc)
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
		CidrBlock: aws.String(defaultSubnetCidrBlock),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSubnet),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(resource.TagName),
						Value: aws.String(resourceID),
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

func (c *Cloud) createOrGetRouteTable(ctx context.Context, vpc *ec2.Vpc, gateway *ec2.InternetGateway, subnet *ec2.Subnet, resourceID string) (*ec2.RouteTable, error) {
	vpcName := vpcName(vpc)
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
						Value: aws.String(resourceID),
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
