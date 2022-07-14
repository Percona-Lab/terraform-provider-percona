#!/usr/bin/env bash

apt-get update
apt-get install -y gnupg2 curl
apt-get install -y debconf-utils
apt-get install -y net-tools

wget https://repo.percona.com/apt/percona-release_latest.$(lsb_release -sc)_all.deb
dpkg -i percona-release_latest.$(lsb_release -sc)_all.deb

apt-get update
percona-release setup ps80