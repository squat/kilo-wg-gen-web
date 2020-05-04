# kilo-wg-gen-web

`kilo-wg-gen-web` enables using [Wg Gen Web](https://github.com/vx3r/wg-gen-web) as a UI to define and manage peers for [Kilo](https://github.com/squat/kilo).

[![Build Status](https://travis-ci.org/squat/kilo-wg-gen-web.svg?branch=master)](https://travis-ci.org/squat/kilo-wg-gen-web)
[![Go Report Card](https://goreportcard.com/badge/github.com/squat/kilo-wg-gen-web)](https://goreportcard.com/report/github.com/squat/kilo-wg-gen-web)

## Getting Started

To run `kilo-wg-gen-web`, first [install Kilo](https://github.com/squat/kilo#installing-on-kubernetes).
Next, edit the included manifest and set the `NODE` variable to the name of one of the nodes in the Kilo mesh, i.e. the node that clients should use to connect to the mesh.
Finally, deploy the included manifest, which contains the configuration for both Wg Gen Web as well as kilo-wg-gen-web:

```shell
kubectl apply -f https://raw.githubusercontent.com/squat/kilo-wg-gen-web/master/manifests/kilo-wg-gen-web.yaml
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
