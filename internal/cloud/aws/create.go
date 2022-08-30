package aws

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"os"
	"terraform-percona/internal/service"
	"terraform-percona/internal/utils"
	"terraform-percona/internal/utils/val"
)

func (c *Cloud) Configure(_ context.Context, resourceId string, data *schema.ResourceData) error {
	if c.configs == nil {
		c.configs = make(map[string]*resourceConfig)
	}
	if _, ok := c.configs[resourceId]; !ok {
		c.configs[resourceId] = &resourceConfig{}
	}
	cfg := c.configs[resourceId]
	if v, ok := data.Get(service.KeyPairName).(string); ok {
		cfg.keyPair = aws.String(v)
	}

	if v, ok := data.Get(service.PathToKeyPairStorage).(string); ok {
		cfg.pathToKeyPair = aws.String(v)
	}

	if v, ok := data.Get(service.InstanceType).(string); ok {
		cfg.instanceType = aws.String(v)
	}

	if v, ok := data.Get(service.VolumeType).(string); ok {
		cfg.volumeType = aws.String(v)
	}
	if aws.StringValue(cfg.volumeType) == "" {
		cfg.volumeType = aws.String("gp2")
	}

	if v, ok := data.Get(service.VolumeSize).(int); ok {
		cfg.volumeSize = aws.Int64(int64(v))
	}
	if v, ok := data.Get(service.VolumeIOPS).(int); ok {
		if v != 0 {
			cfg.volumeIOPS = aws.Int64(int64(v))
		}
	}

	if c.Region != nil {
		if ami, ok := mapRegionImage[aws.StringValue(c.Region)]; ok {
			cfg.ami = aws.String(ami)
		} else {
			return fmt.Errorf("can't find any AMI for region - %s", *c.Region)
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

	vpc, err := c.createVpc(ctx, resourceId)
	if err != nil {
		return err
	}

	internetGateway, err := c.createInternetGateway(ctx, vpc, resourceId)
	if err != nil {
		return err
	}

	securityGroupId, err := c.createSecurityGroup(ctx, vpc, aws.String(SecurityGroupName), aws.String(SecurityGroupDescription), resourceId)
	if err != nil {
		return err
	}

	subnet, err := c.createSubnet(ctx, vpc, resourceId)
	if err != nil {
		return err
	}

	_, err = c.createRouteTable(ctx, vpc, internetGateway, subnet, resourceId)
	if err != nil {
		return err
	}

	c.configs[resourceId].securityGroupID = securityGroupId
	c.configs[resourceId].subnetID = subnet.SubnetId
	return nil
}

func (c *Cloud) createKeyPair(ctx context.Context, resourceId string) error {
	cfg := c.configs[resourceId]
	if val.Str(cfg.keyPair) == "" {
		return fmt.Errorf("cannot create key pair with empty name")
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
						Key:   aws.String(service.ClusterResourcesTagName),
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

func (c *Cloud) createVpc(ctx context.Context, resourceId string) (*ec2.Vpc, error) {
	createVpcOutput, err := c.client.CreateVpcWithContext(ctx, &ec2.CreateVpcInput{
		CidrBlock: aws.String(DefaultVpcCidrBlock),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeVpc),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(service.ClusterResourcesTagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("error occurred during Vpc creating: %w", err)
		}
	}

	if _, err = c.client.ModifyVpcAttributeWithContext(ctx, &ec2.ModifyVpcAttributeInput{
		EnableDnsHostnames: &ec2.AttributeBooleanValue{Value: aws.Bool(true)},
		VpcId:              createVpcOutput.Vpc.VpcId,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("failed modify Vpc attribute: VpcId:%s, Error:%w", *createVpcOutput.Vpc.VpcId, err)
		}
	}

	return createVpcOutput.Vpc, nil
}

func (c *Cloud) createInternetGateway(ctx context.Context, vpc *ec2.Vpc, resourceId string) (*ec2.InternetGateway, error) {
	createInternetGatewayOutput, err := c.client.CreateInternetGatewayWithContext(ctx, &ec2.CreateInternetGatewayInput{
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeInternetGateway),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(service.ClusterResourcesTagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("failed create internet gateway: %w", err)
		}
	}

	if _, err = c.client.AttachInternetGatewayWithContext(ctx, &ec2.AttachInternetGatewayInput{
		InternetGatewayId: createInternetGatewayOutput.InternetGateway.InternetGatewayId,
		VpcId:             vpc.VpcId,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("failed attach internet gateway to Vpc: VpcId:%s, Error:%w", *vpc.VpcId, err)
		}
	}

	return createInternetGatewayOutput.InternetGateway, nil
}

func (c *Cloud) createSecurityGroup(ctx context.Context, vpc *ec2.Vpc, groupName, groupDescription *string, resourceId string) (*string, error) {
	createSecurityGroupResult, err := c.client.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   groupName,
		Description: groupDescription,
		VpcId:       vpc.VpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSecurityGroup),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(service.ClusterResourcesTagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("Unable to create security group %q, %v ", *groupName, err)
		}
	}

	if _, err = c.client.AuthorizeSecurityGroupIngressWithContext(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: createSecurityGroupResult.GroupId,
		IpPermissions: []*ec2.IpPermission{
			(&ec2.IpPermission{}).
				SetIpProtocol("-1").
				SetFromPort(-1).
				SetToPort(-1).
				SetIpRanges([]*ec2.IpRange{
					{CidrIp: aws.String(AllAddressesCidrBlock)},
				}),
		},
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("failed authorize security group ingress traffic: %w", err)
		}
	}

	if _, err := c.client.AuthorizeSecurityGroupEgressWithContext(ctx, &ec2.AuthorizeSecurityGroupEgressInput{
		GroupId: createSecurityGroupResult.GroupId,
		IpPermissions: []*ec2.IpPermission{
			(&ec2.IpPermission{}).
				SetIpProtocol("-1").
				SetFromPort(-1).
				SetToPort(-1),
		},
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("failed authorize security group egress traffic: %w", err)
		}
	}

	return createSecurityGroupResult.GroupId, nil
}

func (c *Cloud) createSubnet(ctx context.Context, vpc *ec2.Vpc, resourceId string) (*ec2.Subnet, error) {
	createSubnetOutput, err := c.client.CreateSubnetWithContext(ctx, &ec2.CreateSubnetInput{
		CidrBlock: aws.String(DefaultSubnetCidrBlock),
		VpcId:     vpc.VpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSubnet),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(service.ClusterResourcesTagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("failed create subnet: %w", err)
		}
	}

	return createSubnetOutput.Subnet, nil
}

func (c *Cloud) createRouteTable(ctx context.Context, vpc *ec2.Vpc, iGateway *ec2.InternetGateway, subnet *ec2.Subnet, resourceId string) (*ec2.RouteTable, error) {
	createRouteTableOutput, err := c.client.CreateRouteTableWithContext(ctx, &ec2.CreateRouteTableInput{
		VpcId: vpc.VpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeRouteTable),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(service.ClusterResourcesTagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("failed create route table: %w", err)
		}
	}

	if _, err = c.client.CreateRouteWithContext(ctx, &ec2.CreateRouteInput{
		DestinationCidrBlock: aws.String(AllAddressesCidrBlock),
		GatewayId:            iGateway.InternetGatewayId,
		RouteTableId:         createRouteTableOutput.RouteTable.RouteTableId,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("failed create route: %w", err)
		}
	}

	if _, err = c.client.AssociateRouteTableWithContext(ctx, &ec2.AssociateRouteTableInput{
		RouteTableId: createRouteTableOutput.RouteTable.RouteTableId,
		SubnetId:     subnet.SubnetId,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("failed associate route table: %w", err)
		}
	}

	return createRouteTableOutput.RouteTable, nil
}
