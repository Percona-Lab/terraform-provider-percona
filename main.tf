provider "percona" {
  region  = "eu-north-1"
  profile = "default"
}

resource "percona_cluster" "pxc" {
  instance_type            = "t3.micro"
  key_pair_name            = "sshKey"
  instance_profile         = "AdemaSSMInstanceProfile"
  password                 = "password"
  cluster_size             = 3
  min_count                = 1
  max_count                = 1
  path_to_key_pair_storage = "/tmp/"
}