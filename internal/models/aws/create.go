package aws

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"os"
	"terraform-percona/internal/utils"
	"terraform-percona/internal/utils/val"
)

func (cloud *Cloud) Configure(resourceId string, data *schema.ResourceData) error {
	if cloud.configs == nil {
		cloud.configs = make(map[string]*resourceConfig)
	}
	if _, ok := cloud.configs[resourceId]; !ok {
		cloud.configs[resourceId] = &resourceConfig{}
	}
	cfg := cloud.configs[resourceId]
	if v, ok := data.Get(KeyPairName).(string); ok {
		cfg.keyPair = aws.String(v)
	}

	if v, ok := data.Get(PathToKeyPairStorage).(string); ok {
		cfg.pathToKeyPair = aws.String(v)
	}

	if v, ok := data.Get(InstanceType).(string); ok {
		cfg.instanceType = aws.String(v)
	}

	if v, ok := data.Get(VolumeType).(string); ok {
		cfg.volumeType = aws.String(v)
	}

	if v, ok := data.Get(VolumeSize).(int); ok {
		cfg.volumeSize = aws.Int64(int64(v))
	}

	if cloud.Region != nil {
		if ami, ok := mapRegionImage[aws.StringValue(cloud.Region)]; ok {
			cfg.ami = aws.String(ami)
		} else {
			return fmt.Errorf("can't find any AMI for region - %s", *cloud.Region)
		}
	}

	var err error
	cloud.session, err = session.NewSession(&aws.Config{
		Region: cloud.Region,
	})
	if err != nil {
		return errors.Wrap(err, "failed create aws session")
	}
	cloud.client = ec2.New(cloud.session)
	keyPairPath, err := cloud.keyPairPath(resourceId)
	if err != nil {
		return err
	}
	if _, err := os.Stat(keyPairPath); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		cfg.sshConfig, err = utils.SSHConfig("ubuntu", keyPairPath)
		if err != nil {
			return errors.Wrap(err, "failed create ssh config")
		}
	}

	return nil
}

func (cloud *Cloud) CreateInfrastructure(resourceId string) error {
	if err := cloud.createKeyPair(resourceId); err != nil {
		return err
	}

	vpc, err := cloud.createVpc(resourceId)
	if err != nil {
		return err
	}

	internetGateway, err := cloud.createInternetGateway(vpc, resourceId)
	if err != nil {
		return err
	}

	securityGroupId, err := cloud.createSecurityGroup(vpc, aws.String(SecurityGroupName), aws.String(SecurityGroupDescription), resourceId)
	if err != nil {
		return err
	}

	subnet, err := cloud.createSubnet(vpc, resourceId)
	if err != nil {
		return err
	}

	_, err = cloud.createRouteTable(vpc, internetGateway, subnet, resourceId)
	if err != nil {
		return err
	}

	cloud.configs[resourceId].securityGroupID = securityGroupId
	cloud.configs[resourceId].subnetID = subnet.SubnetId
	return nil
}

func (cloud *Cloud) createKeyPair(resourceId string) error {
	//TODO add validation

	cfg := cloud.configs[resourceId]
	if val.Str(cfg.keyPair) == "" {
		return fmt.Errorf("cannot create key pair with empty name")
	}

	keyPairPath, err := cloud.keyPairPath(resourceId)
	if err != nil {
		return err
	}
	createKeyPairOutput, err := cloud.client.CreateKeyPair(&ec2.CreateKeyPairInput{
		KeyName: cfg.keyPair,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeKeyPair),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(ClusterResourcesTagName),
						Value: aws.String(resourceId),
					},
				},
			},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return errors.New(aerr.Message())
		} else {
			return fmt.Errorf("error occurred during key pair creating: %w", err)
		}
	}

	if err := writeKey(keyPairPath, createKeyPairOutput.KeyMaterial); err != nil {
		return fmt.Errorf("failed write key pair to file: %w", err)
	}
	cfg.sshConfig, err = utils.SSHConfig("ubuntu", keyPairPath)
	if err != nil {
		return errors.Wrap(err, "failed create ssh config")
	}
	return nil
}

func writeKey(fileName string, fileData *string) error {
	err := os.WriteFile(fileName, []byte(*fileData), 0400)
	return err
}

func (cloud *Cloud) createVpc(resourceId string) (*ec2.Vpc, error) {
	//TODO add validation

	createVpcOutput, err := cloud.client.CreateVpc(&ec2.CreateVpcInput{
		CidrBlock: aws.String(DefaultVpcCidrBlock),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeVpc),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(ClusterResourcesTagName),
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

	if _, err = cloud.client.ModifyVpcAttribute(&ec2.ModifyVpcAttributeInput{
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

func (cloud *Cloud) createInternetGateway(vpc *ec2.Vpc, resourceId string) (*ec2.InternetGateway, error) {
	//TODO add manager validation

	createInternetGatewayOutput, err := cloud.client.CreateInternetGateway(&ec2.CreateInternetGatewayInput{
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeInternetGateway),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(ClusterResourcesTagName),
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

	if _, err = cloud.client.AttachInternetGateway(&ec2.AttachInternetGatewayInput{
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

func (cloud *Cloud) createSecurityGroup(vpc *ec2.Vpc, groupName, groupDescription *string, resourceId string) (*string, error) {
	//TODO add manager validation

	createSecurityGroupResult, err := cloud.client.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		GroupName:   groupName,
		Description: groupDescription,
		VpcId:       vpc.VpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSecurityGroup),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(ClusterResourcesTagName),
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

	if _, err = cloud.client.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
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

	if _, err := cloud.client.AuthorizeSecurityGroupEgress(&ec2.AuthorizeSecurityGroupEgressInput{
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

func (cloud *Cloud) createSubnet(vpc *ec2.Vpc, resourceId string) (*ec2.Subnet, error) {
	//TODO add manager validation

	createSubnetOutput, err := cloud.client.CreateSubnet(&ec2.CreateSubnetInput{
		CidrBlock: aws.String(DefaultSubnetCidrBlock),
		VpcId:     vpc.VpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeSubnet),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(ClusterResourcesTagName),
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

func (cloud *Cloud) createRouteTable(vpc *ec2.Vpc, iGateway *ec2.InternetGateway, subnet *ec2.Subnet, resourceId string) (*ec2.RouteTable, error) {
	//TODO add manager validation

	createRouteTableOutput, err := cloud.client.CreateRouteTable(&ec2.CreateRouteTableInput{
		VpcId: vpc.VpcId,
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeRouteTable),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String(ClusterResourcesTagName),
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

	if _, err = cloud.client.CreateRoute(&ec2.CreateRouteInput{
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

	if _, err = cloud.client.AssociateRouteTable(&ec2.AssociateRouteTableInput{
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
