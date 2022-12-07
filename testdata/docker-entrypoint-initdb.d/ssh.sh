#!/bin/bash -e

cp /etc/certs/*.pem /var/lib/mysql/

chown mysql:mysql /var/lib/mysql/*.pem
