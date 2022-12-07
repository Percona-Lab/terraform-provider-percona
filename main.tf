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
  count = 1

  instance_type = "t3.micro" # for AWS
  #instance_type             = "e2-micro" # for GCP
  key_pair_name            = "sshKey1"
  password                 = "password"
  replica_password         = "replicaPassword"
  cluster_size             = 2
  path_to_key_pair_storage = "/tmp/"
}

output "ps_resources" {
  value = [for resource in percona_ps.ps : {
    resource_id = resource.id,
    instances   = resource.instances,
  }]
}

resource "percona_pxc" "pxc" {
  count = 1

  instance_type = "t3.micro" # for AWS
  #instance_type             = "e2-micro" # for GCP
  key_pair_name            = "sshKey2"
  password                 = "password"
  cluster_size             = 2
  path_to_key_pair_storage = "/tmp/"
}


output "pxc_resources" {
  value = [for resource in percona_pxc.pxc : {
    resource_id = resource.id,
    instances   = resource.instances,
  }]
}
