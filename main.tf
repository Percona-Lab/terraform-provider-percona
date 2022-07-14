provider "percona" {
  region  = "eu-north-1"
  profile = "default"
  cloud   = "aws"
}

resource "percona_ps" "ps" {
  instance_type            = "t3.micro"
  key_pair_name            = "sshKey1"
  password                 = "password"
  replica_password         = "replicaPassword"
  cluster_size             = 2
  path_to_key_pair_storage = "/tmp/"
}

resource "percona_pxc" "pxc" {
  instance_type            = "t3.micro"
  key_pair_name            = "sshKey2"
  password                 = "password"
  cluster_size             = 2
  path_to_key_pair_storage = "/tmp/"
}