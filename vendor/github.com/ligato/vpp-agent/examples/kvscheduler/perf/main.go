//  Copyright (c) 2018 Cisco and/or its affiliates.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at:
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/ligato/cn-infra/agent"

	interfaces "github.com/ligato/vpp-agent/api/models/vpp/interfaces"
	l3 "github.com/ligato/vpp-agent/api/models/vpp/l3"
	"github.com/ligato/vpp-agent/clientv2/linux/localclient"
	"github.com/ligato/vpp-agent/plugins/orchestrator"
	"github.com/ligato/vpp-agent/plugins/vpp/ifplugin"
	"github.com/ligato/vpp-agent/plugins/vpp/l3plugin"
	ipsec "github.com/ligato/vpp-agent/api/models/vpp/ipsec"
)

/*
	Replicated GRPC example.
*/

func main() {
	ep := &ExamplePlugin{
		Orchestrator: &orchestrator.DefaultPlugin,
		VPPIfPlugin:  &ifplugin.DefaultPlugin,
		VPPL3Plugin:  &l3plugin.DefaultPlugin,
	}

	a := agent.NewAgent(
		agent.AllPlugins(ep),
	)
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}

// ExamplePlugin is the main plugin which
// handles resync and changes in this example.
type ExamplePlugin struct {
	VPPIfPlugin  *ifplugin.IfPlugin
	VPPL3Plugin  *l3plugin.L3Plugin
	Orchestrator *orchestrator.Plugin
}

// String returns plugin name
func (p *ExamplePlugin) String() string {
	return "perf"
}

// Init handles initialization phase.
func (p *ExamplePlugin) Init() error {
	return nil
}

// AfterInit handles phase after initialization.
func (p *ExamplePlugin) AfterInit() error {
	go testLocalClientWithScheduler()
	return nil
}

// Close cleans up the resources.
func (p *ExamplePlugin) Close() error {
	return nil
}

func testLocalClientWithScheduler() {
	// initial resync
	time.Sleep(time.Second * 2)
	fmt.Println("=== RESYNC ===")

	resyncTxn := localclient.DataResyncRequest("example")
	err := resyncTxn.
		VppInterface(memIFRed).
		VppInterface(memIFBlack).
		Send().ReceiveReply()
	if err != nil {
		fmt.Println(err)
		return
	}

	// data change
	time.Sleep(time.Second * 10)
	fmt.Println("=== CHANGE #1 ===")

	for i := 1; i <= 10000; i++ {
		txn := localclient.DataChangeRequest("example")
		err = txn.
			Put().
			VppInterface(ipsecTunnel(i)).
			StaticRoute(ipsecRoute(i)).
			Send().ReceiveReply()
		if err != nil {
			fmt.Println(err)
			return
		}
	}
	fmt.Println("DONE ALL")
}

func ipsecTunnel(tunID int) *interfaces.Interface {
	ipsecInfo := &interfaces.Interface_Ipsec{
		Ipsec: &interfaces.IPSecLink{
			LocalIp:         "100.100.100.100",
			RemoteIp:        "20." + gen3octets(uint32(tunID)),
			LocalSpi:        uint32(tunID),
			RemoteSpi:       uint32(tunID),
			CryptoAlg:       ipsec.CryptoAlg_AES_CBC_256,
			LocalCryptoKey:  "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
			RemoteCryptoKey: "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
			IntegAlg:        ipsec.IntegAlg_SHA_512_256,
			LocalIntegKey:   "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
			RemoteIntegKey:  "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
		},
	}
	ipsecTunnelName := fmt.Sprintf("ipsec-%d", tunID)
	ipsecTunnel := &interfaces.Interface{
		Name:    ipsecTunnelName,
		Type:    interfaces.Interface_IPSEC_TUNNEL,
		Enabled: true,
		Mtu:     9000,
		Unnumbered: &interfaces.Interface_Unnumbered{
			InterfaceWithIp: "red",
		},
		Link: ipsecInfo,
	}
	return ipsecTunnel
}

func gen3octets(num uint32) string {
	return fmt.Sprintf("%d.%d.%d",
		(num>>16)&0xFF,
		(num>>8)&0xFF,
		(num)&0xFF)
}

func ipsecRoute(tunID int) *l3.Route {
	ipsecTunnelName := fmt.Sprintf("ipsec-%d", tunID)
	route := &l3.Route{
		DstNetwork:        "30." + gen3octets(uint32(tunID)) + "/32",
		NextHopAddr:       "172.2.0.1",
		OutgoingInterface: ipsecTunnelName,
	}
	return route
}

var (
	memifRedInfo = &interfaces.Interface_Memif{
		Memif: &interfaces.MemifLink{
			Id:             1000,
			Master:         false,
			SocketFilename: "/var/run/memif_k8s-master.sock",
		},
	}
	memIFRed = &interfaces.Interface{
		Name:        "red",
		Type:        interfaces.Interface_MEMIF,
		Enabled:     true,
		IpAddresses: []string{"100.100.100.100/24"},
		Mtu:         9000,
		Link:        memifRedInfo,
	}
	memifBlackInfo = &interfaces.Interface_Memif{
		Memif: &interfaces.MemifLink{
			Id:             1001,
			Master:         false,
			SocketFilename: "/var/run/memif_k8s-master.sock",
		},
	}
	memIFBlack = &interfaces.Interface{
		Name:        "black",
		Type:        interfaces.Interface_MEMIF,
		Enabled:     true,
		IpAddresses: []string{"20.20.20.100/24"},
		Mtu:         9000,
		Link:        memifBlackInfo,
	}
)
