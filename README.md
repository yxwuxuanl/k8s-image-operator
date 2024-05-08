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
```