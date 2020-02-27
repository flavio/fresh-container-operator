[![experimental](http://badges.github.io/stability-badges/dist/experimental.svg)](http://github.com/badges/stability-badges)

fresh-container-operator is a kubernetes operator that leverages
[fresh-container](https://github.com/flavio/fresh-container) to find
deployments using stale containers.

# How it works

Right now the operator monitors all the deployments and perform semantic version
checks against the ones that have the special
`fresh-container.constraint/<name of the container>`
annotation inside of the `spec.template.spec.containers` section.

Take the following deployment object as an example:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  annotations:
    fresh-container.autopilot: "false"
spec:
  selector:
    matchLabels:
      app: nginx
  replicas: 1
  template:
    metadata:
      labels:
        app: nginx
      annotations:
        fresh-container.constraint/nginx: ">= 1.9.0 < 1.10.0"
    spec:
      containers:
      - name: nginx
        image: nginx:1.9.0
        ports:
        - containerPort: 80
```

This deployment creates a POD with a single container named `nginx` running
inside of it.

The version of the `nginx` container is evaluated by
`fresh-container-operator` using the semver constraint specified by the
`fresh-container.constraint/nginx` annotation. In this case the constraint is
`>= 1.9.0 < 1.10.0`.

## Automatic labelling of deployments with stale containers

The operator adds the special label `fresh-container.hasOutdatedContainers=true`
to all the deployments that have one or more stale containers inside of them.

This allows quick searches against all the deployments:

```
$ kubectl get deployments --all-namespaces -l fresh-container.hasOutdatedContainers=true
NAMESPACE   NAME               READY   UP-TO-DATE   AVAILABLE   AGE
default     nginx-deployment   1/1     1            1           19m
```

The details about the stale containers are added inside of the annotations of
the deployment:

```
kubectl describe deployments.apps nginx-deployment
Name:                   nginx-deployment
Namespace:              default
CreationTimestamp:      Thu, 27 Feb 2020 10:32:55 +0100
Labels:                 fresh-container.hasOutdatedContainers=true
Annotations:            deployment.kubernetes.io/revision: 1
                        fresh-container.autopilot: false
                        fresh-container.lastChecked: 2020-02-27T09:45:07Z
                        fresh-container.nextTag/nginx: 1.9.15
```

For each stale container the operator adds an annotation with
`fresh-container.nextTag/<container name>` as key and the tag of the most
recent container that satisfies the constraint as value.

In the example above you can see that the `nginx` container inside of the deployment
can be updated to the `1.9.15` tag while still satisfying the `>= 1.9.0 < 1.10.0`
constraint.

# Automatic updates of stale containers

The fresh-container-operator can also automatically update the stale containers
that are found inside of a deployment. This behaviour is disable by default, it
can be enabled by creating an annotation inside of `metadata.annotations`
named `fresh-container.autopilot` with value `true`.
