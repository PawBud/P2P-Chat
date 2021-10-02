package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	discovery "github.com/libp2p/go-libp2p-discovery"
	host "github.com/libp2p/go-libp2p-host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	tls "github.com/libp2p/go-libp2p-tls"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/libp2p/go-tcp-transport"
	"github.com/mr-tron/base58/base58"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	"github.com/sirupsen/logrus"
)

type P2P struct {
	Ctx context.Context
	Host host.Host
	KadDHT *dht.IpfsDHT
	Discovery *discovery.RoutingDiscovery
	PubSub *pubsub.PubSub
}

// NewP2P(Announce & Advertise) -> SetupHost -> SetupDHT -> BootstrapDHT -> handlePeerDiscovery -> Generate CID

const service = "PawBud/P2P-Chat"

func NewP2P() *P2P {
	ctx := context.Background()
	nodehost, kaddht := setupHost(ctx)
	logrus.Debugln("Created the P2P Host and the Kademlia DHT.")
	bootstrapDHT(ctx, nodehost, kaddht)
	logrus.Debugln("Bootstrapped the Kademlia DHT and Connected to Bootstrap Peers")
	routingdiscovery := discovery.NewRoutingDiscovery(kaddht)
	logrus.Debugln("Created the Peer Discovery Service.")
	pubsubhandler := setupPubSub(ctx, nodehost, routingdiscovery)
	logrus.Debugln("Created the PubSub Handler.")

	return &P2P{
		Ctx:       ctx,
		Host:      nodehost,
		KadDHT:    kaddht,
		Discovery: routingdiscovery,
		PubSub:    pubsubhandler,
	}
}

func (p2p *P2P) AdvertiseConnectivity() {
	ttl, err := p2p.Discovery.Advertise(p2p.Ctx, service)
	logrus.Debugln("Advertised the PeerChat Service.")
	// Time delay for the advertisement to travel
	time.Sleep(time.Second * 5)
	logrus.Debugf("Service Time-to-Live is %s", ttl)
	peerchan, err := p2p.Discovery.FindPeers(p2p.Ctx, service)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("P2P Peer Discovery Failed!")
	}
	logrus.Traceln("Discovered PeerChat Service Peers.")

	go handlePeerDiscovery(p2p.Host, peerchan)
	logrus.Traceln("Started Peer Connection Handler.")
}

func (p2p *P2P) AnnounceConnectivity() {
	cidvalue := generateCID(service)
	logrus.Traceln("Generated the Service CID.")

	// Announce that this host can provide the service CID
	err := p2p.KadDHT.Provide(p2p.Ctx, cidvalue, true)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Announce Service CID!")
	}
	logrus.Debugln("Announced the PeerChat Service.")
	// Time delay for the advertisement to travel
	time.Sleep(time.Second * 5)

	// Find the other providers for the service CID
	peerchan := p2p.KadDHT.FindProvidersAsync(p2p.Ctx, cidvalue, 0)
	logrus.Traceln("Discovered PeerChat Service Peers.")

	// If discovered, connect to the peers
	go handlePeerDiscovery(p2p.Host, peerchan)
	logrus.Debugln("Started Peer Connection Handler.")
}

func setupHost(ctx context.Context) (host.Host, *dht.IpfsDHT) {
	// Host identity
	prvkey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	identity := libp2p.Identity(prvkey)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate P2P Identity Configuration!")
	}
	logrus.Traceln("Generated P2P Identity Configuration.")

	// TLS secured TCP transport
	tlstransport, err := tls.New(prvkey)
	security := libp2p.Security(tls.ID, tlstransport)
	transport := libp2p.Transport(tcp.NewTCPTransport)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate P2P Security and Transport Configurations!")
	}
	logrus.Traceln("Generated P2P Security and Transport Configurations.")

	//Host listener address
	muladdr, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/0")
	listen := libp2p.ListenAddrs(muladdr)
	// Handle any potential error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate P2P Address Listener Configuration!")
	}
	logrus.Traceln("Generated P2P Address Listener Configuration.")

	// Stream multiplexer and connection manager
	muxer := libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport)
	conn := libp2p.ConnectionManager(connmgr.NewConnManager(100, 400, time.Minute))
	logrus.Traceln("Generated P2P Stream Multiplexer, Connection Manager Configurations.")

	// NAT traversal and relay
	nat := libp2p.NATPortMap()
	relay := libp2p.EnableAutoRelay()
	logrus.Traceln("Generated P2P NAT Traversal and Relay Configurations.")

	var kaddht *dht.IpfsDHT
	// Setup a routing configuration with the KadDHT
	routing := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		kaddht = setupKadDHT(ctx, h)
		return kaddht, err
	})
	logrus.Traceln("Generated P2P Routing Configurations.")

	opts := libp2p.ChainOptions(identity, listen, security, transport, muxer, conn, nat, routing, relay)
	// Construct a new libP2P host with the created options
	libhost, err := libp2p.New(ctx, opts)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Create the P2P Host!")
	}

	return libhost, kaddht
}

func setupKadDHT(ctx context.Context, nodehost host.Host) *dht.IpfsDHT {
	dhtmode := dht.Mode(dht.ModeServer)
	bootstrappeers := dht.GetDefaultBootstrapPeerAddrInfos()
	dhtpeers := dht.BootstrapPeers(bootstrappeers...)
	logrus.Traceln("Generated DHT Configuration.")

	// Start a Kademlia DHT on the host in server mode
	kaddht, err := dht.New(ctx, nodehost, dhtmode, dhtpeers)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Create the Kademlia DHT!")
	}

	return kaddht
}

func setupPubSub(ctx context.Context, nodehost host.Host, routingdiscovery *discovery.RoutingDiscovery) *pubsub.PubSub {
	pubsubhandler, err := pubsub.NewGossipSub(ctx, nodehost, pubsub.WithDiscovery(routingdiscovery))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
			"type":  "GossipSub",
		}).Fatalln("PubSub Handler Creation Failed!")
	}

	return pubsubhandler
}

func bootstrapDHT(ctx context.Context, nodehost host.Host, kaddht *dht.IpfsDHT) {
	// Bootstrap the DHT to satisfy the IPFS Router interface
	if err := kaddht.Bootstrap(ctx); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Bootstrap the Kademlia!")
	}
	logrus.Traceln("Set the Kademlia DHT into Bootstrap Mode.")

	var wg sync.WaitGroup
	var connectedbootpeers int
	var totalbootpeers int

	// looping through the bootstrap peers provided by libp2p
	for _, peeraddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peeraddr)
		wg.Add(1)
		// Trying to connect to each bootstrap peer
		go func() {
			defer wg.Done()
			if err := nodehost.Connect(ctx, *peerinfo); err != nil {
				totalbootpeers++
			} else {
				connectedbootpeers++
				totalbootpeers++
			}
		}()
	}

	wg.Wait()
	logrus.Debugf("Connected to %d out of %d Bootstrap Peers.", connectedbootpeers, totalbootpeers)
}

func handlePeerDiscovery(nodehost host.Host, peerchan <-chan peer.AddrInfo) {
	for peer := range peerchan {
		if peer.ID == nodehost.ID() {
			continue
		}

		nodehost.Connect(context.Background(), peer)
	}
}

func generateCID(namestring string) cid.Cid {
	// Hash the service content ID with SHA256
	hash := sha256.Sum256([]byte(namestring))
	// Append the hash with the hashing codec ID for SHA2-256 (0x12),
	// the digest size (0x20) and the hash of the service content ID
	finalhash := append([]byte{0x12, 0x20}, hash[:]...)
	// Encode the fullhash to Base58
	b58string := base58.Encode(finalhash)

	// Generate a Multihash from the base58 string
	mulhash, err := multihash.FromB58String(string(b58string))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate Service CID!")
	}

	// Generate a CID from the Multihash
	cidvalue := cid.NewCidV1(12, mulhash)
	return cidvalue
}