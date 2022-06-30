#!/usr/bin/env bash

apt-get update
apt-get install net-tools


# install from repository
apt-get install debconf-utils
apt-get install -y wget gnupg2 lsb-release curl
wget https://repo.percona.com/apt/percona-release_latest.generic_all.deb
dpkg -i percona-release_latest.generic_all.deb
apt-get update
percona-release setup pxc80