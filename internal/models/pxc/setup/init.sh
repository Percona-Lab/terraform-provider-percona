#!/usr/bin/env bash

sudo apt-get update
sudo apt-get install net-tools

# install from repository
sudo apt-get install debconf-utils
sudo apt-get install -y wget gnupg2 lsb-release curl
wget https://repo.percona.com/apt/percona-release_latest.generic_all.deb
sudo dpkg -i percona-release_latest.generic_all.deb
sudo apt-get update
sudo percona-release setup pxc80