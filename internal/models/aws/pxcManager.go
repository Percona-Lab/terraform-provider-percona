package aws

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"os"
	"terraform-percona/internal/bootstrap/pxc"
	"terraform-percona/internal/utils/val"
)

type XtraDBClusterManager struct {
	Config  *Config
	Session *session.Session
	Client  *ec2.EC2
}

type Config struct {
	Region    *string
	Profile   *string
	AccessKey *string
	SecretKey *string
	*InstanceSettings
}

type InstanceSettings struct {
	Ami                  *string
	InstanceType         *string
	MinCount             *int64
	MaxCount             *int64
	KeyPairName          *string
	InstanceProfile      *string
	PathToKeyPairStorage *string
	ClusterSize          *int64
	VolumeType           *string
	VolumeSize           *int64
	MySQLPassword        *string
}

const (
	InstanceType         = "instance_type"
	MinCount             = "min_count"
	MaxCount             = "max_count"
	KeyPairName          = "key_pair_name"
	PathToKeyPairStorage = "path_to_key_pair_storage"
	ClusterSize          = "cluster_size"
	VolumeType           = "volume_type"
	VolumeSize           = "volume_size"
	InstanceProfile      = "instance_profile"
	MySQLPassword        = "password"

	DefaultVpcCidrBlock    = "10.0.0.0/16"
	DefaultSubnetCidrBlock = "10.0.1.0/16"
	AllAddressesCidrBlock  = "0.0.0.0/0"

	SecurityGroupName        = "security-group"
	SecurityGroupDescription = "security-group"

	ErrorUserDataMsgFailedOpenFile   = "failed open file with user data"
	ErrorUserDataMsgFileNotExist     = "can't find user data file with proposed path"
	ErrorUserDataMsgPermissionDenied = "application doesn't have permission to open file with user data"

	ResourceIdLen           = 20
	ClusterResourcesTagName = "percona-xtradb-cluster-stack-id"
)

func (c *Config) Valid() bool {
	if c == nil {
		return false
	}

	return c.Region != nil
}

var MapRegionImage = map[string]string{
	"us-east-1":      "ami-04505e74c0741db8d",
	"us-east-2":      "ami-0fb653ca2d3203ac1",
	"us-west-1":      "ami-01f87c43e618bf8f0",
	"us-west-2":      "ami-0892d3c7ee96c0bf7",
	"af-south-1":     "ami-0670428c515903d37",
	"ap-east-1":      "ami-0350928fdb53ae439",
	"ap-southeast-3": "ami-0f06496957d1fe04a",
	"ap-south-1":     "ami-05ba3a39a75be1ec4",
	"ap-northeast-3": "ami-0c2223049202ca738",
	"ap-northeast-2": "ami-0225bc2990c54ce9a",
	"ap-southeast-1": "ami-0750a20e9959e44ff",
	"ap-southeast-2": "ami-0d539270873f66397",
	"ap-northeast-1": "ami-0a3eb6ca097b78895",
	"ca-central-1":   "ami-073c944d45ffb4f27",
	"eu-central-1":   "ami-02584c1c9d05efa69",
	"eu-west-1":      "ami-00e7df8df28dfa791",
	"eu-west-2":      "ami-00826bd51e68b1487",
	"eu-south-1":     "ami-06ea0ad3f5adc2565",
	"eu-west-3":      "ami-0a21d1c76ac56fee7",
	"eu-north-1":     "ami-09f0506c9ef0fb473",
	"me-south-1":     "ami-05b680b37c7917206",
	"sa-east-1":      "ami-077518a464c82703b",
	"us-gov-east-1":  "ami-0eb7ef4cc0594fa04",
	"us-gov-west-1":  "ami-029a634618d6c0300",
}

func NewXtraDBClusterManager(config *Config) (*XtraDBClusterManager, error) {
	if !config.Valid() {
		return &XtraDBClusterManager{}, errors.New("invalid config for Percona XtraDB Cluster manager")
	}
	sess, err := session.NewSession(&aws.Config{
		Region: config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed create aws session: %w", err)
	}
	return &XtraDBClusterManager{
		Config:  config,
		Session: sess,
		Client:  ec2.New(sess),
	}, nil
}

func (manager *XtraDBClusterManager) CreateCluster(resourceId string) (interface{}, error) {
	//TODO full manager validation
	if manager.Client == nil {
		return nil, fmt.Errorf("nil EC2 client")
	}

	if err := manager.createKeyPair(resourceId); err != nil {
		return nil, err
	}

	vpc, err := manager.createVpc(resourceId)
	if err != nil {
		return nil, err
	}

	internetGateway, err := manager.createInternetGateway(vpc, resourceId)
	if err != nil {
		return nil, err
	}

	securityGroupId, err := manager.createSecurityGroup(vpc, aws.String(SecurityGroupName), aws.String(SecurityGroupDescription), resourceId)
	if err != nil {
		return nil, err
	}

	subnet, err := manager.createSubnet(vpc, resourceId)
	if err != nil {
		return nil, err
	}

	_, err = manager.createRouteTable(vpc, internetGateway, subnet, resourceId)
	if err != nil {
		return nil, err
	}

	userData, err := manager.getBase64UserData()
	if err != nil {
		return nil, err
	}

	instanceIds := make([]*string, 0, *manager.Config.ClusterSize)
	clusterAddresses := make([]string, 0, *manager.Config.ClusterSize)
	// TODO: run instances at once
	for i := int64(0); i < *manager.Config.ClusterSize; i++ {
		reservation, err := manager.Client.RunInstances(&ec2.RunInstancesInput{
			IamInstanceProfile: &ec2.IamInstanceProfileSpecification{
				Name: manager.Config.InstanceProfile,
			},
			ImageId:      manager.Config.Ami,
			InstanceType: manager.Config.InstanceType,
			MinCount:     manager.Config.MinCount,
			MaxCount:     manager.Config.MaxCount,
			NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{
				{
					AssociatePublicIpAddress: aws.Bool(true),
					DeviceIndex:              aws.Int64(0),
					Groups:                   []*string{securityGroupId},
					SubnetId:                 subnet.SubnetId,
					PrivateIpAddress:         aws.String(fmt.Sprintf("10.0.1.%d", i+1)),
				},
			},
			BlockDeviceMappings: []*ec2.BlockDeviceMapping{
				{
					DeviceName: aws.String(fmt.Sprintf("/dev/sd%s", string(rune('f'+i)))),
					Ebs: &ec2.EbsBlockDevice{
						VolumeType: manager.Config.VolumeType,
						VolumeSize: manager.Config.VolumeSize,
					},
				},
			},
			KeyName:  manager.Config.KeyPairName,
			UserData: userData,
			TagSpecifications: []*ec2.TagSpecification{
				{
					ResourceType: aws.String(ec2.ResourceTypeInstance),
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
				return nil, err
			}
		}

		instanceIds = append(instanceIds, reservation.Instances[0].InstanceId)
		clusterAddresses = append(clusterAddresses, aws.StringValue(reservation.Instances[0].PrivateIpAddress))
	}
	if err := manager.Client.WaitUntilInstanceStatusOk(&ec2.DescribeInstanceStatusInput{
		InstanceIds: instanceIds,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			return nil, errors.New(aerr.Message())
		} else {
			return nil, fmt.Errorf("error occurred while waiting until instances running: InstanceIds:%v, Error:%w",
				instanceIds, err)
		}
	}
	for i, id := range instanceIds {
		if err = manager.RunCommand(aws.StringValue(id), pxc.Configure(clusterAddresses, aws.StringValue(manager.Config.MySQLPassword))); err != nil {
			return nil, err
		}
		if err = manager.RunCommand(aws.StringValue(id), pxc.Start(i == 0)); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (manager *XtraDBClusterManager) createKeyPair(resourceId string) error {
	//TODO add validation

	if val.Str(manager.Config.KeyPairName) == "" {
		return fmt.Errorf("cannot create key pair with empty name")
	}

	awsKeyPairStoragePath := fmt.Sprintf("%s%s.pem", *manager.Config.PathToKeyPairStorage, *manager.Config.KeyPairName)
	createKeyPairOutput, err := manager.Client.CreateKeyPair(&ec2.CreateKeyPairInput{
		KeyName: manager.Config.KeyPairName,
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

	if err := writeKey(awsKeyPairStoragePath, createKeyPairOutput.KeyMaterial); err != nil {
		return fmt.Errorf("failed write key pair to file: %w", err)
	}
	return nil
}

func writeKey(fileName string, fileData *string) error {
	err := os.WriteFile(fileName, []byte(*fileData), 0400)
	return err
}

func (manager *XtraDBClusterManager) createVpc(resourceId string) (*ec2.Vpc, error) {
	//TODO add validation

	createVpcOutput, err := manager.Client.CreateVpc(&ec2.CreateVpcInput{
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

	if _, err = manager.Client.ModifyVpcAttribute(&ec2.ModifyVpcAttributeInput{
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

func (manager *XtraDBClusterManager) createInternetGateway(vpc *ec2.Vpc, resourceId string) (*ec2.InternetGateway, error) {
	//TODO add manager validation

	createInternetGatewayOutput, err := manager.Client.CreateInternetGateway(&ec2.CreateInternetGatewayInput{
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

	if _, err = manager.Client.AttachInternetGateway(&ec2.AttachInternetGatewayInput{
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

func (manager *XtraDBClusterManager) createSecurityGroup(vpc *ec2.Vpc, groupName, groupDescription *string, resourceId string) (*string, error) {
	//TODO add manager validation

	createSecurityGroupResult, err := manager.Client.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
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

	if _, err = manager.Client.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
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

	if _, err := manager.Client.AuthorizeSecurityGroupEgress(&ec2.AuthorizeSecurityGroupEgressInput{
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

func (manager *XtraDBClusterManager) createSubnet(vpc *ec2.Vpc, resourceId string) (*ec2.Subnet, error) {
	//TODO add manager validation

	createSubnetOutput, err := manager.Client.CreateSubnet(&ec2.CreateSubnetInput{
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

func (manager *XtraDBClusterManager) createRouteTable(vpc *ec2.Vpc, iGateway *ec2.InternetGateway, subnet *ec2.Subnet, resourceId string) (*ec2.RouteTable, error) {
	//TODO add manager validation

	createRouteTableOutput, err := manager.Client.CreateRouteTable(&ec2.CreateRouteTableInput{
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

	if _, err = manager.Client.CreateRoute(&ec2.CreateRouteInput{
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

	if _, err = manager.Client.AssociateRouteTable(&ec2.AssociateRouteTableInput{
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

func (manager *XtraDBClusterManager) getBase64UserData() (*string, error) {
	//TODO add manager validation
	return aws.String(base64.StdEncoding.EncodeToString([]byte(pxc.Initial()))), nil
}
