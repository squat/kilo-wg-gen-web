# kilo-wg-gen-web

`kilo-wg-gen-web` enables using [Wg Gen Web](https://github.com/vx3r/wg-gen-web) as a UI to define and manage peers for [Kilo](https://github.com/squat/kilo).

[![Build Status](https://travis-ci.org/squat/kilo-wg-gen-web.svg?branch=master)](https://travis-ci.org/squat/kilo-wg-gen-web)
[![Go Report Card](https://goreportcard.com/badge/github.com/squat/kilo-wg-gen-web)](https://goreportcard.com/report/github.com/squat/kilo-wg-gen-web)

## Getting Started

To run `kilo-wg-gen-web`, first [install Kilo](https://github.com/squat/kilo#installing-on-kubernetes).
Next, edit the [included manifest](https://github.com/squat/kilo-wg-gen-web/blob/master/manifests/kilo-wg-gen-web.yaml) and set the `NODE` variable to the name of one of the nodes in the Kilo mesh, i.e. the node that clients should use to connect to the mesh.
Finally, deploy the included manifest, which contains the configuration for both Wg Gen Web as well as kilo-wg-gen-web:

```shell
kubectl apply -f https://raw.githubusercontent.com/squat/kilo-wg-gen-web/master/manifests/kilo-wg-gen-web.yaml
```

## OIDC + RBAC

Anyone with access to the Wg Gen Web UI will have access to create, read, update, and delete Kilo Peers, which means they can grant access to the VPN to other users.
OIDC and RBAC can be used in order to restrict access to only users who are authenticated and authorized to perform certain actions on Peer resources.
To get started, first ensure that the Kubernetes API server is configured to validate OIDC tokens.
Afterwards, edit the [included OIDC + RBAC manifest](https://github.com/squat/kilo-wg-gen-web/blob/master/manifests/kilo-wg-gen-web-oidc-rbac.yaml), which configures an [OAuth2 proxy](https://github.com/oauth2-proxy/oauth2-proxy) and an [RBAC proxy](https://github.com/brancz/kube-rbac-proxy) in front of the Wg Gen Web UI to set the `NODE` variable as well as add the necessary OIDC configuration and credentials to the `kilo-wg-gen-web` Secret.
Next, deploy Wg Gen Web with OIDC and RBAC:

```shell
kubectl apply -f https://raw.githubusercontent.com/squat/kilo-wg-gen-web/master/manifests/kilo-wg-gen-web-oidc-rbac.yaml
```

Finally, grant access to certain privileges in Wg Gen Web by creating Kubernetes ClusterRoles and ClusterRoleBindings.
For example, the following command could be used to grant access to view the Wg Gen Web UI to the user `example@example.com`:

```shell
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: view-peers
rules:
- apiGroups:
  - kilo.squat.ai
  resources:
  - peers
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: example-view
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view-peers
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: example@example.com
EOF
```

Access to create Peers via the UI could be granted to the user `example@example.com` with the following command:

```shell
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: edit-peers
rules:
- apiGroups:
  - kilo.squat.ai
  resources:
  - peers
  verbs:
  - update
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: example-view
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: edit-peers
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: example@example.com
EOF
```

## Usage

[embedmd]:# (tmp/help.txt)
```txt
Use Kilo as a backend for Wg Gen Web

Usage:
  kilo-wg-gen-web [flags]
  kilo-wg-gen-web [command]

Available Commands:
  help        Help about any command
  setnode     Set the Wg Gen Web server config for the selected node.

Flags:
      --dir string          Path to the Wg Gen Web configuration directory.
  -h, --help                help for kilo-wg-gen-web
      --kubeconfig string   Path to kubeconfig. (default "/home/squat/src/infrastructure/liao/kubeconfig")
      --listen string       The address at which to listen for health and metrics. (default ":1107")
      --log-level string    Log level to use. Possible values: all, debug, info, warn, error, none (default "info")

Use "kilo-wg-gen-web [command] --help" for more information about a command.
```
