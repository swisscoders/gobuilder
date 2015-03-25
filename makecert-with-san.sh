#!/bin/bash
echo subjectAltName = IP:127.0.0.1 > cert.cnf

# Private CA
openssl genrsa -out myCA.key 2048
openssl req -x509 -new -key myCA.key -out myCA.cer -days 730 -subj /CN="something"
