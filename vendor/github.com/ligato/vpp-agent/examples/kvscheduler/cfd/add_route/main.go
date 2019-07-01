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

	"github.com/ligato/vpp-agent/plugins/orchestrator"
	kvs "github.com/ligato/vpp-agent/plugins/kvscheduler"
	kvs_api "github.com/ligato/vpp-agent/plugins/kvscheduler/api"
	"github.com/ligato/vpp-agent/clientv2/linux/localclient"
	vpp_ifplugin "github.com/ligato/vpp-agent/plugins/vppv2/ifplugin"
	vpp_l3plugin "github.com/ligato/vpp-agent/plugins/vppv2/l3plugin"
	"github.com/ligato/vpp-agent/api/models/vpp/interfaces"
	"github.com/ligato/vpp-agent/api/models/vpp/l3"
)

/*
	This is a simple example for testing kvscheduler.
*/
func main() {
	ep := &ExamplePlugin{
		Orchestrator:  &orchestrator.DefaultPlugin,
		KVScheduler:   &kvs.DefaultPlugin,
		VPPIfPlugin:   &vpp_ifplugin.DefaultPlugin,
		VPPL3Plugin:   &vpp_l3plugin.DefaultPlugin,
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
	Orchestrator  *orchestrator.Plugin
	KVScheduler   *kvs.Scheduler
	VPPIfPlugin   *vpp_ifplugin.IfPlugin
	VPPL3Plugin   *vpp_l3plugin.L3Plugin
}

// String returns plugin name
func (p *ExamplePlugin) String() string {
	return "simple-example"
}

// Init handles initialization phase.
func (p *ExamplePlugin) Init() error {
	return nil
}

// AfterInit handles phase after initialization.
func (p *ExamplePlugin) AfterInit() error {
	ch := make(chan *kvs_api.BaseValueStatus, 100)
	p.KVScheduler.WatchValueStatus(ch, nil)
	go watchValueStatus(ch)
	go testLocalClientWithScheduler(p.KVScheduler)
	return nil
}

// Close cleans up the resources.
func (p *ExamplePlugin) Close() error {
	return nil
}

func watchValueStatus(ch <-chan *kvs_api.BaseValueStatus) {
	for {
		select {
		case status := <-ch:
			fmt.Printf("Value status change: %v\n", status.String())
		}
	}
}

func testLocalClientWithScheduler(kvscheduler kvs_api.KVScheduler) {
	// initial resync
	time.Sleep(time.Second * 2)
	fmt.Println("=== RESYNC (using bridge domain) ===")

	txn := localclient.DataResyncRequest("example")
	err := txn.
		StaticRoute(myRoute).
		Send().ReceiveReply()
	if err != nil {
		fmt.Println(err)
		return
	}

	time.Sleep(time.Second * 60)

	txn2 := localclient.DataChangeRequest("example")
	err = txn2.Put().
		VppInterface(myTap).
		Send().ReceiveReply()
	if err != nil {
		fmt.Println(err)
		return
	}
}

var (
	myTap = &vpp_interfaces.Interface{
		Name:        "my-tap",
		Type:        vpp_interfaces.Interface_TAP,
		Enabled:     true,
		IpAddresses: []string{"192.168.1.1/24"},
		Link: &vpp_interfaces.Interface_Tap{
			Tap: &vpp_interfaces.TapLink{
				Version: 1,
			},
		},
	}

	myRoute = &vpp_l3.Route{
		DstNetwork: "192.168.0.0/16",
		NextHopAddr: "192.168.1.100",
		OutgoingInterface: "my-tap",
	}
)
