package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/spf13/cobra"
	"github.com/squat/kilo/pkg/k8s"
	"gitlab.127-0-0-1.fr/vx3r/wg-gen-web/model"
	"k8s.io/client-go/kubernetes"
)

const serverConfigurationName = "server.json"

func setNode(dir *string, c *kubernetes.Interface) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setnode",
		Short: "Set the Wg Gen Web server config for the selected node.",
		Args:  cobra.ExactArgs(1),
	}
	var allowedIPs, addressPools []string
	cmd.PersistentFlags().StringSliceVar(&allowedIPs, "allowed-ips", []string{"10.5.0.0/16"}, "The default allowed IPs that should be used for clients.")
	cmd.PersistentFlags().StringSliceVar(&addressPools, "address-pools", []string{"10.6.0.0/16"}, "The default address pools from which client addresses should be allocated.")
	cmd.RunE = func(_ *cobra.Command, args []string) error {
		node := args[0]
		b := k8s.New(*c, nil, nil)
		stop := make(chan struct{})
		defer close(stop)
		if err := b.Nodes().Init(stop); err != nil {
			return fmt.Errorf("initializing backend: %w", err)
		}
		nodes, err := b.Nodes().List()
		if err != nil {
			return fmt.Errorf("listing nodes: %w", err)
		}
		cfg := &model.Server{
			Address:    addressPools,
			AllowedIPs: allowedIPs,
		}
		var ok bool
		for _, n := range nodes {
			if n.Name == node {
				cfg.Endpoint = n.Endpoint.String()
				cfg.ListenPort = int(n.Endpoint.Port)
				cfg.PersistentKeepalive = n.PersistentKeepalive
				cfg.PublicKey = string(n.Key)
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("could not find node %q", node)
		}
		j, err := ioutil.ReadFile(path.Join(*dir, serverConfigurationName))
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("read configuration file %q: %w", serverConfigurationName, err)
		}
		if j != nil {
			if err := json.Unmarshal(j, cfg); err != nil {
				return fmt.Errorf("unmarshal configuration as JSON: %w", err)
			}
		}
		j, err = json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal configuration: %w", err)
		}
		if err := ioutil.WriteFile(path.Join(*dir, serverConfigurationName), j, 0644); err != nil {
			return fmt.Errorf("write server configuration: %w", err)
		}
		return nil
	}

	return cmd
}
