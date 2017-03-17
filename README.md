# _Experimental_ Nuage Networks CNI plugin for Kubernetes 

This is an _experimental_ Nuage Networks VSP plugin for Kubernetes. It uses the experimental [CNI plugin for Nuage Networks VSP](https://github.com/nuagenetworks/nuage-cni-experimental). 

It consits of an agent running on Kubernetes Master nodes, and leverages the CNI Agent server and CNI plugin on Kubernetes nodes for performing node-specific actions. 

It provides the following features: 
- Multi-master capability with `etcd` based HA and fail-over
- The ability to specify custom network settings as part of pod activation 
- The ability to use Kubernetes networking policies
- The ability to use Nuage networks security policy framework (an extension for the above). Both those capabilities are subject to `service-account` based authorization.

The code is _experimental_ work in progress. It is and provided only as a use case for [Go SDK for Nuage Networks VSP](https://github.com/nuagenetworks/vspk-go/) and [Go SDK for Nuage Networks VRS](https://github.com/nuagenetworks/libvrsdk/).


# DISCLAIMER

This code is a developer community contribution. It is only provided under [Apache License](./LICENSE.md) **_as is_** with **_no liabilities_** whatsoever from Nuage Networks.

In particular (but not limited to):
- This code is not officially supported by any Nuage Networks product.
- It may be entirely replaced or removed, without any prior notice.
- It _may_ be eventually absorbed as part of a product offering, but Nuage Networks is under no committment or obligation to disclose if, how or when.

For any questions, comments or feedback, please raise a GitHub issue.
