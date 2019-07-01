// Copyright (c) 2017 Cisco and/or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"log"
	"time"
	"net"

	govppapi "git.fd.io/govpp.git/api"
	"github.com/ligato/cn-infra/agent"
	"github.com/ligato/cn-infra/logging"
	"github.com/ligato/cn-infra/logging/logrus"
	"github.com/ligato/cn-infra/utils/safeclose"
	"github.com/ligato/vpp-agent/cmd/vpp-agent/app"
	"github.com/ligato/vpp-agent/plugins/govppmux"
	ipApi "github.com/ligato/vpp-agent/plugins/vpp/binapi/vpp1810/ip"
	"github.com/ligato/cn-infra/utils/addrs"
	"fmt"
)

// *************************************************************************
// Pre-requisites:
//  * set interface state GigabitEthernet0/8/0 up
//  * set interface ip address GigabitEthernet0/8/0 10.10.1.1/16
// ************************************************************************/

// Main allows running Example Plugin as a statically linked binary with Agent Core Plugins. Close channel and plugins
// required for the example are initialized. Agent is instantiated with generic plugins (etcd, Kafka, Status check,
// HTTP and Log), and GOVPP, and resync plugin, and example plugin which demonstrates GOVPP call functionality.
func main() {
	// Init close channel to stop the example.
	closeChannel := make(chan struct{})

	// Inject dependencies to example plugin
	ep := &ExamplePlugin{
		Log:          logrus.DefaultLogger(),
		closeChannel: closeChannel,
	}
	ep.Deps.VPP = app.DefaultVPP()
	ep.Deps.GoVppMux = &govppmux.DefaultPlugin

	// Start Agent
	a := agent.NewAgent(
		agent.AllPlugins(ep),
		agent.QuitOnClose(closeChannel),
	)
	if err := a.Run(); err != nil {
		log.Fatal()
	}
}

// PluginName represents name of plugin.
const PluginName = "govpp-example"

// ExamplePlugin implements Plugin interface which is used to pass custom plugin instances to the Agent.
type ExamplePlugin struct {
	Deps

	vppChannel   govppapi.Channel // Vpp channel to communicate with VPP
	// Fields below are used to properly finish the example.
	closeChannel chan struct{}
	Log          logging.Logger
}

// Deps is example plugin dependencies.
type Deps struct {
	GoVppMux *govppmux.Plugin
	VPP      app.VPP
}

// Init members of plugin.
func (plugin *ExamplePlugin) Init() (err error) {
	// NewAPIChannel returns a new API channel for communication with VPP via govpp core.
	// It uses default buffer sizes for the request and reply Go channels.
	plugin.vppChannel, err = plugin.Deps.GoVppMux.NewAPIChannel()

	plugin.Log.Info("Default plugin plugin ready")

	go plugin.routePerfTestParallel()

	return err
}

// Close is called by Agent Core when the Agent is shutting down. It is supposed
// to clean up resources that were allocated by the plugin during its lifetime.
func (plugin *ExamplePlugin) Close() error {
	return safeclose.Close(plugin.vppChannel)
}

// String returns plugin name
func (plugin *ExamplePlugin) String() string {
	return PluginName
}

/***********
 * VPPCall *
 ***********/

func (plugin *ExamplePlugin) routePerfTest() {
	time.Sleep(3 * time.Second)

	start := time.Now()
	var total int
	for j := 2; j < 40; j++ {
		for i := 2; i < 250; i++ {
			req := buildRouteReq(fmt.Sprintf("10.11.%d.%d/32", j, i))
			reply := &ipApi.IPAddDelRouteReply{}

			//plugin.Log.Info("Sending data to VPP ...")

			// 1. Send the request and receive a reply directly (in one line).
			err := plugin.vppChannel.SendRequest(req).ReceiveReply(reply)
			if err != nil {
				plugin.Log.Error(err)
			}
			total++
		}
	}

	plugin.Log.Infof("%d IP routes were configured - took: %v", total, time.Now().Sub(start))
	// End the example.
	plugin.Log.Infof("etcd/datasync example finished, sending shutdown ...")
	close(plugin.closeChannel)
}

func (plugin *ExamplePlugin) routePerfTestParallel() {
	time.Sleep(3 * time.Second)

	start := time.Now()
	var total int

	doneCh := make(chan int)
	workerCnt := 10
	worker := func(idx int) {
		vppChannel, _ := plugin.Deps.GoVppMux.NewAPIChannel()
		var configured, routeIdx int

		for j := 2; j < 40; j++ {
			for i := 2; i < 250; i++ {
				routeIdx++
				if routeIdx % workerCnt != idx {
					continue
				}
				req := buildRouteReq(fmt.Sprintf("10.11.%d.%d/32", j, i))
				reply := &ipApi.IPAddDelRouteReply{}

				//plugin.Log.Info("Sending data to VPP ...")

				// 1. Send the request and receive a reply directly (in one line).
				err := vppChannel.SendRequest(req).ReceiveReply(reply)
				if err != nil {
					plugin.Log.Error(err)
				}
				configured++
			}
		}
		plugin.Log.Infof("worker %d configured %d IP routes", idx, configured)
		doneCh <- configured
	}
	for i := 0; i < workerCnt; i++ {
		go worker(i)
	}
	for i := 0; i < workerCnt; i++ {
		configured := <- doneCh
		total += configured
	}

	plugin.Log.Infof("%d IP routes were configured - took: %v", total, time.Now().Sub(start))
	// End the example.
	plugin.Log.Infof("etcd/datasync example finished, sending shutdown ...")
	close(plugin.closeChannel)
}

var nextHop = net.ParseIP("10.10.1.2")

// Auxiliary method to transform agent model data to binary api format
func buildRouteReq(dstIP string) *ipApi.IPAddDelRoute {
	req := &ipApi.IPAddDelRoute{}
	req.IsAdd = 1

	parsedDstIP, _, err := addrs.ParseIPWithPrefix(dstIP)
	if err != nil {
		return nil
	}
	req.IsIPv6 = 0
	req.DstAddress = []byte(parsedDstIP.IP.To4())
	req.NextHopAddress = []byte(nextHop.To4())
	prefix, _ := parsedDstIP.Mask.Size()
	req.DstAddressLength = byte(prefix)

	req.NextHopSwIfIndex = 1
	req.IsMultipath = 1

	return req
}