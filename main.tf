# AWS provider configuration
provider "percona" {
  region  = "eu-north-1"
  profile = "default"
  cloud   = "aws"
}

# GCP provider configuration
#provider "percona" {
#  region  = "europe-west1"
#  zone    = "europe-west1-c"
#  project = "project-name"
#  cloud =   "gcp"
#}

resource "percona_ps" "ps" {
  instance_type            = "t3.micro" # for AWS
#  instance_type             = "e2-micro" # for GCP
  key_pair_name            = "sshKey1"
  password                 = "password"
  replica_password         = "replicaPassword"
  cluster_size             = 2
  path_to_key_pair_storage = "/tmp/"
}

locals {
  primary = one([for instance in percona_ps.ps.instances: instance if instance.is_replica == false])
  replicas = [for instance in percona_ps.ps.instances: instance if instance.is_replica == true]
}

output "ps_primary_instance" {
  value = {
    public_ip = local.primary.public_ip_address
    private_ip = local.primary.private_ip_address
  }
}

output "ps_replica_instances" {
  value = [for instance in local.replicas:
  {
    public_ip = instance.public_ip_address
    private_ip = instance.private_ip_address
  }]
}

resource "percona_pxc" "pxc" {
  instance_type            = "t3.micro" # for AWS
#  instance_type             = "e2-micro" # for GCP
  key_pair_name            = "sshKey2"
  password                 = "password"
  cluster_size             = 2
  path_to_key_pair_storage = "/tmp/"
}


output "pxc_instances" {
  value = [for instance in percona_pxc.pxc.instances: instance]
}