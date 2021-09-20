# RedisOperator

**RedisOperator** is a product ready kubernetes operator to deploy and manage high available [Redis Sentinel] and [Redis Cluster] instances. This repository contains multi [Custom Resource Definition (CRD)] designed for the lifecycle of Redis sentinel or cluster instance.

## Features

* Redis sentinel/cluster supported
* ACL supported
* Redis 6.0, 6.2, 7.0, 7.2 supported (only versions 6.0 and 7.2 have undergone thorough testing. 5.0 also supported, but no acl supported)
* Online data backup/restore
* Graceful version upgrade
* High available in product environment

## Quickstart

If you have a Kubernetes cluster and `kubectl` configured to access it, run the following command to instance the operator:

## Documentation

RedisOperator is covered by following topics:

* **TODO** Operator overview
* **TODO** Deploying the operator
* **TODO** Deploying a Redis sentinel/cluster instance
* **TODO** Monitoring the instance 
* **TODO** Backup instance data

In addition, few [samples](./config/samples) can be find in this repo.

## Contributing

This project follows the typical GitHub pull request model. Before starting any work, please either comment on an [existing issue](https://github.com/alauda/redis-operatoa, ,), or file a new one.

Please read [contribution guidelines](CONTRIBUTING.md) if you are interested in contributing to this project.

## Releasing

To release a new version of the RedisOperator, create a versioned tag (e.g. `v1.2.3`) of the repo, and the release pipeline will generate a new draft release, along side release artefacts.

## License

[Licensed under Apache 2.0](LICENSE)
