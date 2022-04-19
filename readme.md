Percona Terraform Provider
=========================

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) 1.1.2
- [Go](https://golang.org/doc/install) 1.16.x (to build the provider plugin)
- [AWS](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) 1 or 2 version

## Warning!

In this version created resources don't destroyed automatically, you need to do this manually

## How to run?

1. Clone repo
2. Configure AWS CLI - [tutorial](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-quickstart.html)
3. Switch to project directory
4. Execute in console `make all` or go through **Makefile**(in the root of project) manually
5. When cluster is set up, connect to one of instances
6. Login to mysql with command `sudo mysql -uroot -p` and enter password `pass`(soon make this dynamic)
7. Check cluster status `show status like 'wsrep%';`

## Configuration

File **main.tf**

```
provider "percona" {
  region  = "eu-north-1"                                #required
  profile = "default"                                   #optional
}

resource "percona_cluster" "pxc" {
  ami                      = "ami-092cce4a19b438926"    #required
  instance_type            = "t3.micro"                 #required    
  path_to_bootstrap_script = "./bootstrap.sh"           #optional
  key_pair_name            = "sshKey"                   #optional
  cluster_size             = 3                          #optional
  path_to_key_pair_storage = "/tmp/"                    #optional
}   
```

File **version.tf**

```
terraform {
  required_providers {
    percona = {
      version = "~> 1.0.0"
      source  = "terraform-percona.com/terraform-percona/percona"
    }
  }
}
```

## Required AWS permissions policies in order to create Percona XtraDB Cluster

```
//Custome policies set
    {
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": [
                "ec2:CreateDhcpOptions",
                "ec2:AuthorizeSecurityGroupIngress",
                "ec2:DeleteSubnet",
                "ec2:DescribeInstances",
                "ec2:MonitorInstances",
                "ec2:CreateKeyPair",
                "ec2:AttachInternetGateway",
                "ec2:UpdateSecurityGroupRuleDescriptionsIngress",
                "ec2:AssociateRouteTable",
                "ec2:DeleteRouteTable",
                "ec2:StartInstances",
                "ec2:RevokeSecurityGroupEgress",
                "ec2:CreateRoute",
                "ec2:CreateInternetGateway",
                "ec2:DescribeVolumes",
                "ec2:DeleteInternetGateway",
                "ec2:DescribeReservedInstances",
                "ec2:DescribeKeyPairs",
                "ec2:DescribeRouteTables",
                "ec2:DetachVolume",
                "ec2:UpdateSecurityGroupRuleDescriptionsEgress",
                "ec2:DescribeReservedInstancesOfferings",
                "ec2:CreateRouteTable",
                "ec2:RunInstances",
                "ec2:ModifySecurityGroupRules",
                "ec2:StopInstances",
                "ec2:CreateVolume",
                "ec2:RevokeSecurityGroupIngress",
                "ec2:DescribeSecurityGroupRules",
                "ec2:DeleteDhcpOptions",
                "ec2:DescribeInstanceTypes",
                "ec2:DeleteVpc",
                "ec2:AssociateAddress",
                "ec2:CreateSubnet",
                "ec2:DescribeSubnets",
                "ec2:DeleteKeyPair",
                "ec2:AttachVolume",
                "ec2:DisassociateAddress",
                "ec2:DescribeAddresses",
                "ec2:PurchaseReservedInstancesOffering",
                "ec2:DescribeInstanceAttribute",
                "ec2:CreateVpc",
                "ec2:DescribeDhcpOptions",
                "ec2:DescribeAvailabilityZones",
                "ec2:CreateSecurityGroup",
                "ec2:ModifyVpcAttribute",
                "ec2:ModifyReservedInstances",
                "ec2:DescribeInstanceStatus",
                "ec2:RebootInstances",
                "ec2:AuthorizeSecurityGroupEgress",
                "ec2:AssociateDhcpOptions",
                "ec2:TerminateInstances",
                "ec2:DescribeIamInstanceProfileAssociations",
                "ec2:DescribeTags",
                "ec2:DeleteRoute",
                "ec2:AllocateAddress",
                "ec2:DescribeSecurityGroups",
                "ec2:DescribeImages",
                "ec2:DescribeVpcs",
                "ec2:DeleteSecurityGroup",
                "ec2:CreateNetworkInterface",
                "ec2:DescribeInternetGateways",
                "ec2:DescribeVpcAttribute",
                "ec2:DeleteNetworkInterface",
                "ec2:DeleteSecurityGroup",
                "ec2:ModifyNetworkInterfaceAttribute",
            ],
            "Resource": "*"
        }
    ]
}

//AmazonEC2ContainerServiceAutoscaleRole
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ecs:DescribeServices",
                "ecs:UpdateService"
            ],
            "Resource": [
                "*"
            ]
        },
        {
            "Effect": "Allow",
            "Action": [
                "cloudwatch:DescribeAlarms",
                "cloudwatch:PutMetricAlarm"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}

```

## NOTE

In different regions **AMI** may differ from each other, even if the system is the same. Also, **instance types**, in
some regions some may be available, and in others they may not.