[![CircleCI](https://circleci.com/gh/giantswarm/kubectl-openstack.svg?style=shield)](https://circleci.com/gh/giantswarm/kubectl-openstack)

# kubectl-openstack

A tool that helps you manage OpenStack infrastructure CLI access for Cluster API OpenStack (CAPO) clusters.

## Usage

Typically you simply specify the name of one of your CAPO clusters:

```bash
$ kubectl get openstackcluster -A
NAMESPACE   NAME           CLUSTER        READY   NETWORK                                SUBNET                                 BASTION IP
my-ns1      my-cluster-1   my-cluster-1   true    xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
my-ns1      my-cluster-2   my-cluster-1   true    xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

The `--management-cluster` flag can be skipped. More on that later. If the cluster name is not unique across namespaces then `--namepsace` flag is required.

```bash
$ ./kubectl-openstack login --management-cluster my-mc my-cluster-1
Writing "my-mc-my-cluster-1" cloud to /home/username/.config/openstack/clouds.yaml

To use the cloud run:

    openstack --os-cloud="my-mc-my-cluster-1" server list
```

If `--management-cluster` flag flag is omitted then it is inferred as the second segment of the API URL.

Example:

```
$ kubectl cluster-info
Kubernetes control plane is running at https://api.my-mc.test.gigantic.io:6443
CoreDNS is running at https://api.my-mc.test.gigantic.io:6443/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy
```

The inferred Management Cluster name would be `my-mc`.

The full `--help` output:

```
kubectl-openstack login --help
Usage of ./kubectl-openstack:
      --clouds-file string          absolute path to the clouds.yaml file (default "$HOME/.config/openstack/clouds.yaml")
  -f, --force                       force overwriting existing cloud (if it exists) in the clouds file
      --kubeconfig string           absolute path to the kubeconfig file (default "$HOME/.kube/config")
      --management-cluster string   (optional) name of the management cluster, if not set will be inferred from the API URL
  -n, --namespace string            (optional) namespace of the OpenstackCluster resource, required only if the cluster name is ambiguous
  ```

## Things to do

- [ ] Makefiles with install
- [ ] Write installation instructions
- [ ] Add to [Krew](https://krew.sigs.k8s.io/)
- [ ] Readme with placeholders for `--help`
- [ ] Improve command package structure (the `login` command is currently a fixed string in `main.go`)
- [ ] Extract a package with generic collection functions (`contains`, `keys`, etc.)
- [ ] Add `--version` flag
- [ ] Add `selfupdate` command
