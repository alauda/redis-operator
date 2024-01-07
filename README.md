# RedisOperator [![Coverage Status](https://coveralls.io/repos/github/alauda/redis-operator/badge.svg?branch=main)](https://coveralls.io/github/alauda/redis-operator?branch=main)

**RedisOperator** is a production-ready kubernetes operator to deploy and manage high available [Redis Sentinel](https://redis.io/docs/management/sentinel/) and [Redis Cluster](https://redis.io/docs/reference/cluster-spec/) instances. This repository contains multi [Custom Resource Definition (CRD)](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#customresourcedefinitions) designed for the lifecycle of Redis sentinel or cluster instance.

## Features

* Redis sentinel/cluster supported
* ACL supported
* Redis 6.0, 6.2, 7.0, 7.2 supported (only versions 6.0 and 7.2 have undergone thorough testing. 5.0 also supported, but no acl supported)
* Nodeport access supported, assigne ports also supported 
* IPv4/IpV6 supported
* Online scale up/down
* Online data backup/restore
* Graceful version upgrade
* Nodeselector, toleration and affinity supported
* High available in production environment

## Quickstart

If you have a Kubernetes cluster and `kubectl` configured to access it, run the following command to instance the operator:

TODO

## Documentation

RedisOperator is covered by following topics:

* **TODO** Operator overview
* **TODO** Deploying the operator
* **TODO** Deploying a Redis sentinel/cluster instance
* **TODO** Monitoring the instance 
* **TODO** Backup instance data

In addition, few [samples](./config/samples) can be find in this repo.

## Contributing

This project follows the typical GitHub pull request model. Before starting any work, please either comment on an [existing issue](https://github.com/alauda/redis-operator/issues), or file a new one.

## Releasing

To release a new version of the RedisOperator, create a versioned tag (e.g. `v1.2.3`) of the repo, and the release pipeline will generate a new draft release, along side release artefacts.

## License

[Licensed under Apache 2.0](LICENSE)
