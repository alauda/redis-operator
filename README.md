# RedisOperator [![Coverage Status](https://coveralls.io/repos/github/alauda/redis-operator/badge.svg?branch=main)](https://coveralls.io/github/alauda/redis-operator?branch=main)

**RedisOperator** is a production-ready kubernetes operator to deploy and manage high available [Redis® Sentinel](https://redis.io/docs/management/sentinel/) and [Redis® Cluster](https://redis.io/docs/reference/cluster-spec/) instances. This repository contains multi [Custom Resource Definition (CRD)](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/#customresourcedefinitions) designed for the lifecycle of Redis standalone, sentinel or cluster instance.

This operator deploys separately published Redis container images that you select and obtain independently; it does not contain, fork, or redistribute Redis server source code. See [Trademarks](#trademarks) below.

## Features

* Standalone/Sentinel/Cluster redis arch supported.
* Redis ACL supported.
* Redis 6.0, 6.2, 7.0, 7.2 supported (only versions 6.0 and 7.2 have undergone thorough testing. 5.0 also supported, but no acl supported).
* Nodeport/LB access supported; nodeport assignement also supported.
* IPv4/IPv6 supported.
* Online scale up/down.
* Graceful version upgrade.
* Nodeselector, toleration and affinity supported.
* High available in production environment.

## Quickstart

If you have a Kubernetes cluster and `kubectl` configured to access it, run the following command to instance the operator:

TODO

## Documentation

RedisOperator is covered by following topics:

* **TODO** Operator overview
* **TODO** Deploying the operator
* **TODO** Deploying a Redis sentinel/cluster instance
* **TODO** Monitoring the instance 

In addition, few [samples](./config/samples) can be find in this repo.

## Contributing

This project follows the typical GitHub pull request model. Before starting any work, please either comment on an [existing issue](https://github.com/alauda/redis-operator/issues), or file a new one. For more details, please refer to the [CONTRIBUTING.md](./CONTRIBUTING.md) file.

## Releasing

To release a new version of the RedisOperator, create a versioned tag (e.g. `v3.18.0`) of the repo, and the release pipeline will generate a new draft release, along side release artefacts.

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE). Attribution and trademark notices are collected in [NOTICE](NOTICE).

The Apache-2.0 license covers this operator's own source code and the container images built from it. It does not cover the Redis® server: this project does not contain, fork, or distribute Redis server source code, and instead deploys separately published Redis container images that you select and obtain independently. Those images carry their own license terms, which you are responsible for reviewing and complying with. The license of the Redis server and the license of this operator are separate and independent of one another.

## Trademarks

Redis is a registered trademark of Redis Ltd. Any rights therein are reserved to Redis Ltd. Any use by this project is for referential purposes only and does not indicate any sponsorship, endorsement, or affiliation between Redis Ltd. and this project.

Valkey and the Valkey logo are trademarks of LF Projects, LLC. Kubernetes® is a registered trademark of The Linux Foundation in the United States and other countries. All other trademarks are the property of their respective owners.

This project is an independent work developed and maintained by [Alauda](https://github.com/alauda). It is **not endorsed, sponsored, supported, or certified by Redis Ltd.**
