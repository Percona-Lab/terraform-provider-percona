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

## NOTE

In different regions **AMI** may differ from each other, even if the system is the same. Also, **instance types**, in
some regions some may be available, and in others they may not.