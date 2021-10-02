package main

import (
	"context"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-core/host"
	discovery "github.com/libp2p/go-libp2p-discovery"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

type P2P struct {
	Ctx context.Context
	Host host.Host
	KadDHT *dht.IpfsDHT
	Discovery *discovery.RoutingDiscovery
	PubSub *pubsub.PubSub
}