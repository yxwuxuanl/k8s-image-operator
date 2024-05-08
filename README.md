# k8s-image-operator

Rewrite the image of the pod in the k8s cluster.

## Install

```shell
helm repo add image-operator https://yxwuxuanl.github.io/k8s-image-operator/

helm install image-operator image-operator/image-operator
```

## Rewrite

Create a `Rule` resource to rewrite the image:

```yaml
apiVersion: image.lin2ur.cn/v1
kind: Rule
metadata:
  name: mirror.example.com
spec:
  # Ignore or Fail, refer to the `webhooks.failurePolicy` in the `MutatingWebhookConfiguration` resource
  failurePolicy: Ignore
  # Optional, refer to the `webhooks.namespaceSelector` in the `MutatingWebhookConfiguration` resource, default is all namespaces
  namespaceSelector: { }
  # Optional, refer to the `webhooks.objectSelector` in the `MutatingWebhookConfiguration` resource, default is all pods
  podSelector: { }
  # Specify the image rewrite rules
  rewrite:
    - registry: docker.io
      replacement: docker.mirror.example.com
    - regex: ^docker\.io/(.*)$ # <- or use regex to match the image
      replacement: docker.io/$1
  # Specify the tags that are not allowed
  disallowedTags: [ "latest" ]
```

## Mirror

The `Mirror` resource allows you to mirror the image to another registry:

```yaml
apiVersion: image.lin2ur.cn/v1
kind: Mirror
metadata:
  generateName: nginx-
  namespace: default
spec:
  images:
    - source: nginx:1.25 # <- single image
      target: myregistry/nginx:1.25
    - source: nginx
      target: myregistry/nginx
      tags: # <- multiple tags
        - 1.25
        - 1.24
      platforms: # <- multiple platforms
        - linux/amd64
        - linux/arm64
  parallelism: 5 # <- specify the number of parallel jobs, default is 5
  dockerConfig: # <- specify the registry credentials
    secretName: myregistry-secret
  httpProxy: http://myproxy # <- pull the image through the proxy
  resources: { } # <- specify the resources for the job
  sizeLimit: 1Gi # <- specify the tmpfs size limit for the job
```

create the `Mirror` resource:

```shell
$ kubectl create -f mirror.yaml
mirror.image.lin2ur.cn/nginx-6km7p created

$ kubectl get mirror.image.lin2ur.cn/nginx-6km7p -n default
NAME          RUNNING   FAILED   SUCCEEDED
nginx-6km7p   1         0        2
```