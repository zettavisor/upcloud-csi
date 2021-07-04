# upcloud-csi

This project is inspired by [vultr-csi](https://github.com/vultr/vultr-csi)

The Container Storage Interface ([CSI](https://github.com/container-storage-interface/spec)) Driver for UpCloud Block Storage. This driver allows you to use UpCloud Block Storage with your container orchestrator. We have tested this CSI on Kubernetes.

More information about the CSI and Kubernetes can be found: [CSI Spec](https://github.com/container-storage-interface/spec) and [Kubernetes CSI](https://kubernetes-csi.github.io/docs/example.html)


## Installation
### Requirements

- `--allow-privileged` must be enabled for the API server and kubelet

### Kubernetes secret

In order for the csi to work properly, you will need to deploy a [kubernetes secret](https://kubernetes.io/docs/concepts/configuration/secret/). To obtain a API key, please visit [API settings](https://developers.upcloud.com/1.3/).

The `secret.yml` definition is as follows. You can also find a copy of this yaml [here](docs/releases/secret.yml.tmp).
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: upcloud-csi
  namespace: kube-system
stringData:
  api-key: "UpCloud_API_KEY"
```

To create this `secret.yml`, you must run the following

```sh
$ kubectl create -f secret.yml            
secret/upcloud-csi created
```

### Deploying the CSI

To deploy the latest release of the CSI to your Kubernetes cluster, run the following:

`kubectl apply -f https://raw.githubusercontent.com/zettavisor/upcloud-csi/master/docs/releases/latest.yaml`
