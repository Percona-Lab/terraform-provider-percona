#!/usr/bin/env bash

sudo apt-get update
sudo apt-get install -y gnupg2 curl
sudo apt-get install -y debconf-utils
sudo apt-get install -y net-tools

wget https://repo.percona.com/apt/percona-release_latest.$(lsb_release -sc)_all.deb
sudo dpkg -i percona-release_latest.$(lsb_release -sc)_all.deb

sudo apt-get update
sudo percona-release setup ps80