Percona Terraform Provider
=========================

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) 1.1.2
- [Go](https://golang.org/doc/install) 1.16.x (to build the provider plugin)
- [AWS](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) 1 or 2 version

## Latest update

 - Percona Server resource
 - Replication for Percona Server
 - Dynamic configuration depending on cluster size
 - Configurable password
 - Removed hardcoded ip address
 - Now it's possible to create multiple resources
 - Added Google Cloud Platform support

## How to run?

1. Clone repo
2. Configure AWS CLI - [tutorial](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-quickstart.html)
3. Switch to project directory
4. Execute in console `make all` or go through **Makefile**(in the root of project) manually
5. When cluster is set up, connect to one of the PXC instances
6. Login to mysql with command `sudo mysql -uroot -p` and enter password `password`
7. Check cluster status `show status like 'wsrep%';`
8. Connect to one of the Percona Server replica
9. Check replication status using `SHOW SLAVE STATUS\G` on replica

### Run on Google Cloud Platform
1. Create service account in Google Cloud Console and create key for it (for more info, visit https://cloud.google.com/docs/authentication/getting-started)
2. Export `GOOGLE_APPLICATION_CREDENTIALS` environment variable to point to the file with credentials (e.g. `export GOOGLE_APPLICATION_CREDENTIALS=/path/to/credentials.json`)
3. Execute `make all`

## Configuration

File **main.tf**

```
# AWS provider configuration
provider "percona" {
  region  = "eu-north-1"                                #required
  profile = "default"                                   #optional
  cloud   = "aws"                                       #required, supported values: "aws", "gcp"
}

# GCP provider configuration
#provider "percona" {
#  region  = "europe-west1"
#  zone    = "europe-west1-c"
#  project = "project-name"
#  cloud =   "gcp"
#}

resource "percona_ps" "ps" {
  instance_type            = "t3.micro"         # for AWS, optional, default: t4g.nano
  machine_type             = "e2-micro"         # for GCP, optional, default: e2-micro
  key_pair_name            = "sshKey1"          # required
  password                 = "password"         # optional, default: "password"
  replica_password         = "replicaPassword"  # optional, default: "replicaPassword"
  cluster_size             = 2                  # optional, default: 3
  path_to_key_pair_storage = "/tmp/"            # optional, default: "."
  volume_type              = "gp2"              # for AWS, optional, default: "gp2"
  volume_size              = 20                 # for AWS, optional, default: 20
}

resource "percona_pxc" "pxc" {
  instance_type            = "t3.micro" # optional, default: t4g.nano
  machine_type             = "e2-micro"         # for GCP, optional, default: e2-micro
  key_pair_name            = "sshKey2"  # required
  password                 = "password"	# optional, default: "password"
  cluster_size             = 2      	# optional, default: 3
  path_to_key_pair_storage = "/tmp/"    # optional, default: "."
  volume_type              = "gp2"      # for AWS, optional, default: "gp2"
  volume_size              = 20         # for AWS, optional, default: 20
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

## Required AWS permissions policies in order to create Percona XtraDB Cluster or Percona Servers

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