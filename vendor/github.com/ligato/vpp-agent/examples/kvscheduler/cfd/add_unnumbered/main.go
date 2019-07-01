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
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ligato/cn-infra/agent"

	"github.com/ligato/vpp-agent/plugins/orchestrator"
	kvs "github.com/ligato/vpp-agent/plugins/kvscheduler"
	kvs_api "github.com/ligato/vpp-agent/plugins/kvscheduler/api"
	vpp_ifplugin "github.com/ligato/vpp-agent/plugins/vpp/ifplugin"
	"github.com/ligato/vpp-agent/api/models/vpp/interfaces"
	"github.com/ligato/cn-infra/logging"
)

/*
	This is a simple example for testing kvscheduler.
*/
func main() {
	ep := &ExamplePlugin{
		Orchestrator:  &orchestrator.DefaultPlugin,
		KVScheduler:   &kvs.DefaultPlugin,
		VPPIfPlugin:   &vpp_ifplugin.DefaultPlugin,
	}

	kvs.DefaultPlugin.Log.SetLevel(logging.DebugLevel)

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

	txn := kvscheduler.StartNBTransaction()
	txn.SetValue(vpp_interfaces.InterfaceKey(myGbe.GetName()), myGbe)
	txn.SetValue(vpp_interfaces.InterfaceKey(myUnnumberedTap.GetName()), myUnnumberedTap)
	ctx := context.Background()
	ctx = kvs_api.WithResync(ctx, kvs_api.FullResync, true)
	ctx = kvs_api.WithRetry(ctx, time.Minute, 1, false)
	_, err := txn.Commit(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}

	txn = kvscheduler.StartNBTransaction()
	txn.SetValue(vpp_interfaces.InterfaceKey(myGbeWithoutIP.GetName()), myGbeWithoutIP)
	ctx = context.Background()
	ctx = kvs_api.WithRetry(ctx, time.Minute, 1, false)
	_, err = txn.Commit(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
}

var (
	myGbe = &vpp_interfaces.Interface{
		Name:        "GigabitEthernet0/8/0",
		Type:        vpp_interfaces.Interface_DPDK,
		Enabled:     true,
		IpAddresses: []string{"192.168.1.1/24"},
	}

	myGbeWithoutIP = &vpp_interfaces.Interface{
		Name:        "GigabitEthernet0/8/0",
		Type:        vpp_interfaces.Interface_DPDK,
		Enabled:     true,
	}

	myUnnumberedTap = &vpp_interfaces.Interface{
		Name:        "my-unnumbered-tap",
		Type:        vpp_interfaces.Interface_TAP,
		Enabled:     true,

		Link: &vpp_interfaces.Interface_Tap{
			Tap: &vpp_interfaces.TapLink{
				Version: 1,
			},
		},

		Unnumbered: &vpp_interfaces.Interface_Unnumbered{
			InterfaceWithIp: myGbe.GetName(),
		},
	}
)
