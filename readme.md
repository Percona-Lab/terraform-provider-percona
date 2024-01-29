# Percona Terraform Provider

### DISCLAIMER

This is an experimental project, use on your own risk. This project is not covered by Percona Support

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) 1.1.2
- [Go](https://golang.org/doc/install) 1.18.x (to build the provider plugin)
- [AWS](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) 1 or 2 version

## How to run on AWS

1. Clone repo
2. Configure AWS CLI - [tutorial](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-quickstart.html)
3. Switch to project directory
4. Execute in console `make all` or go through **Makefile**(in the root of project) manually
5. When cluster is set up, connect to one of the PXC instances
6. Login to mysql with command `sudo mysql -uroot -p` and enter password `password`
7. Check cluster status `show status like 'wsrep%';`
8. Connect to one of the Percona Server replica
9. Check replication status using `SHOW SLAVE STATUS\G` on replica

## How to run on Google Cloud Platform

1. Create service account in Google Cloud Console and create key for it (for more info, visit https://cloud.google.com/docs/authentication/getting-started)
2. Export `GOOGLE_APPLICATION_CREDENTIALS` environment variable to point to the file with credentials (e.g. `export GOOGLE_APPLICATION_CREDENTIALS=/path/to/credentials.json`)
3. Execute `make all`

## Configuration

File **main.tf**

```
# AWS provider configuration
provider "percona" {
  region                   = "eu-north-1"               # required
  profile                  = "default"                  # optional
  cloud                    = "aws"                      # required, supported values: "aws", "gcp"
  ignore_errors_on_destroy = true                       # optional, default: false
  disable_telemetry        = true                       # optional, default: false
}

# GCP provider configuration
#provider "percona" {
#  region                   = "europe-west1"
#  zone                     = "europe-west1-c"
#  project                  = "project-name"
#  cloud                    = "gcp"
#  ignore_errors_on_destroy = false
#}

resource "percona_ps" "ps" {
  instance_type            = "t3.micro"                          # required
  key_pair_name            = "sshKey1"                           # required
  password                 = "password"                          # optional, default: "password"
  replication_type         = "async"                             # optional, default: "async", supported values: "async", "group-replication"
  replication_password     = "replicaPassword"                   # optional, default: "replicaPassword"
  cluster_size             = 2                                   # optional, default: 3
  path_to_key_pair_storage = "/tmp/"                             # optional, default: "."
  volume_type              = "gp2"                               # optional, default: "gp2" for AWS, "pd-balanced" for GCP
  volume_size              = 20                                  # optional, default: 20
  volume_iops              = 4000                                # optional
  volume_throughput        = 4000                                # optional, AWS only
  config_file_path         = "./config.cnf"                      # optional, saves config file to /etc/mysql/mysql.conf.d/custom.cnf
  version                  = "8.0.28"                            # optional, installs last version if not specified
  myrocks_install          = true                                # optional, default: false
  vpc_name                 = "percona_vpc_1"                     # optional
  vpc_id                   = "cGVyY29uYV92cGNfMQ=="              # optional, AWS only
  port                     = 3306                                # optional, default: 3306
  pmm_address              = "http://admin:admin@127.0.0.1"      # optional
  pmm_password             = "password"                          # optional, password for internal `pmm` user in db
  orchestrator_size        = 3                                   # optional, default: 0
  orchestrator_password    = "password"                          # optional, default: "password"
}

resource "percona_pxc" "pxc" {
  instance_type            = "t3.micro"                          # required
  key_pair_name            = "sshKey2"                           # required
  password                 = "password"	                         # optional, default: "password"
  cluster_size             = 2                                   # optional, default: 3
  path_to_key_pair_storage = "/tmp/"                             # optional, default: "."
  volume_type              = "gp2"                               # optional, default: "gp2" for AWS, "pd-balanced" for GCP
  volume_size              = 20                                  # optional, default: 20
  volume_iops              = 4000                                # optional
  volume_throughput        = 4000                                # optional, AWS only
  config_file_path         = "./config.cnf"                      # optional, saves config file to /etc/mysql/mysql.conf.d/custom.cnf
  version                  = "8.0.28"                            # optional, installs last version if not specified
  vpc_name                 = "percona_vpc_1"                     # optional
  vpc_id                   = "cGVyY29uYV92cGNfMQ=="              # optional, AWS only
  port                     = 3306                                # optional, default: 3306
  galera_port              = 4567                                # optional, default: 4567
  pmm_address              = "http://admin:admin@127.0.0.1"      # optional
  pmm_password             = "password"                          # optional, password for internal `pmm` user in db
}

resource "percona_pmm" "pmm" {
  instance_type            = "t3.micro"                          # required
  key_pair_name            = "sshKey2"                           # required
  path_to_key_pair_storage = "/tmp/"                             # optional, default: "."
  volume_type              = "gp2"                               # optional, default: "gp2" for AWS, "pd-balanced" for GCP
  volume_size              = 20                                  # optional, default: 20
  volume_iops              = 4000                                # optional
  volume_throughput        = 4000                                # optional, AWS only
  vpc_name                 = "percona_vpc_1"                     # optional
  vpc_id                   = "cGVyY29uYV92cGNfMQ=="              # optional, AWS only

  rds_username             = "postgres"                          # optional, default: ""
  rds_password             = "password"                          # optional, default: ""
}

resource "percona_pmm_rds" "pmm_rds" {
  pmm_address              = "http://admin:admin@localhost"      # required
  rds_id                   = "database-1"                        # required
  rds_username             = "postgres"                          # required
  rds_password             = "password"                          # required
  rds_pmm_user_password    = "password"                          # optional, default: "password"
}
```

File **version.tf**

```
terraform {
  required_providers {
    percona = {
      version = "~> 0.9.10"
      source  = "terraform-percona.com/terraform-percona/percona"
    }
  }
}
```

## Required permissions

<details>
<summary>For AWS</summary>

AWS managed policy: AmazonEC2ContainerServiceAutoscaleRole

```bash
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

Custom AWS Policy

```bash
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
                "ec2:ModifyNetworkInterfaceAttribute"
            ],
            "Resource": "*"
        }
    ]
}
```

</details>

## NOTE

**Instance types**, in some regions some may be available and in others they may not.
