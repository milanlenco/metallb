// Copyright (c) 2018 Cisco and/or its affiliates.
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

package descriptor

import (
	"bytes"
	"net"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"

	"github.com/ligato/cn-infra/logging"
	scheduler "github.com/ligato/vpp-agent/plugins/kvscheduler/api"
	ifdescriptor "github.com/ligato/vpp-agent/plugins/vppv2/ifplugin/descriptor"
	"github.com/ligato/vpp-agent/plugins/vppv2/l3plugin/descriptor/adapter"
	"github.com/ligato/vpp-agent/plugins/vppv2/l3plugin/vppcalls"
	"github.com/ligato/vpp-agent/plugins/vppv2/model/interfaces"
	"github.com/ligato/vpp-agent/plugins/vppv2/model/l3"
)

const (
	// StaticRouteDescriptorName is the name of the descriptor for static routes.
	StaticRouteDescriptorName = "vpp-static-route"

	// dependency labels
	routeOutInterfaceDep = "interface-exists"

	// static route weight by default
	defaultWeight = 1
)

// RouteDescriptor teaches KVScheduler how to configure VPP routes.
type RouteDescriptor struct {
	log          logging.Logger
	routeHandler vppcalls.RouteVppAPI
	scheduler    scheduler.KVScheduler
}

// NewRouteDescriptor creates a new instance of the Route descriptor.
func NewRouteDescriptor(scheduler scheduler.KVScheduler,
	routeHandler vppcalls.RouteVppAPI, log logging.PluginLogger) *RouteDescriptor {

	return &RouteDescriptor{
		scheduler:    scheduler,
		routeHandler: routeHandler,
		log:          log.NewLogger("static-route-descriptor"),
	}
}

// GetDescriptor returns descriptor suitable for registration (via adapter) with
// the KVScheduler.
func (d *RouteDescriptor) GetDescriptor() *adapter.StaticRouteDescriptor {
	return &adapter.StaticRouteDescriptor{
		Name: StaticRouteDescriptorName,
		KeySelector: func(key string) bool {
			_, _, _, _, isRouteKey := l3.ParseRouteKey(key)
			return isRouteKey
		},
		ValueTypeName:   proto.MessageName(&l3.StaticRoute{}),
		ValueComparator: d.EquivalentRoutes,
		NBKeyPrefix:     l3.RoutePrefix,
		Add:             d.Add,
		Delete:          d.Delete,
		ModifyWithRecreate: func(key string, oldValue, newValue *l3.StaticRoute, metadata interface{}) bool {
			return true
		},
		IsRetriableFailure: d.IsRetriableFailure,
		Dependencies:       d.Dependencies,
		DerivedValues:      d.DerivedValues,
		Dump:               d.Dump,
		DumpDependencies:   []string{ifdescriptor.InterfaceDescriptorName},
	}
}

// EquivalentRoutes is case-insensitive comparison function for l3.StaticRoute.
func (d *RouteDescriptor) EquivalentRoutes(key string, oldRoute, newRoute *l3.StaticRoute) bool {

	if oldRoute.GetType() != newRoute.GetType() ||
		oldRoute.GetVrfId() != newRoute.GetVrfId() ||
		oldRoute.GetViaVrfId() != newRoute.GetViaVrfId() ||
		oldRoute.GetOutgoingInterface() != newRoute.GetOutgoingInterface() ||
		getWeight(oldRoute) != getWeight(newRoute) ||
		oldRoute.GetPreference() != newRoute.GetPreference() {
		return false
	}

	// compare dst networks
	if !equalNetworks(oldRoute.DstNetwork, newRoute.DstNetwork) {
		return false
	}

	// compare gw addresses (next hop)
	if !equalAddrs(getGwAddr(oldRoute), getGwAddr(newRoute)) {
		return false
	}

	return true
}

// IsRetriableFailure returns <false> for errors related to invalid configuration.
func (d *RouteDescriptor) IsRetriableFailure(err error) bool {
	return false // nothing retriable
}

// Add adds VPP static route.
func (d *RouteDescriptor) Add(key string, route *l3.StaticRoute) (metadata interface{}, err error) {

	err = d.routeHandler.VppAddRoute(route)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// Delete removes VPP static route.
func (d *RouteDescriptor) Delete(key string, route *l3.StaticRoute, metadata interface{}) error {

	err := d.routeHandler.VppDelRoute(route)
	if err != nil {
		return err
	}

	return nil
}

// Dependencies lists dependencies for a VPP route.
func (d *RouteDescriptor) Dependencies(key string, route *l3.StaticRoute) []scheduler.Dependency {
	var dependencies []scheduler.Dependency
	// the outgoing interface must exist and be UP
	if route.OutgoingInterface != "" {
		dependencies = append(dependencies, scheduler.Dependency{
			Label: routeOutInterfaceDep,
			Key:   interfaces.InterfaceKey(route.OutgoingInterface),
		})
	}
	// GW must be routable
	/*gwAddr := net.ParseIP(getGwAddr(route))
	if gwAddr != nil && !gwAddr.IsUnspecified() {
		dependencies = append(dependencies, scheduler.Dependency{
			Label: routeGwReachabilityDep,
			AnyOf: func(key string) bool {
				dstAddr, ifName, err := l3.ParseStaticLinkLocalRouteKey(key)
				if err == nil && ifName == route.OutgoingInterface && dstAddr.Contains(gwAddr) {
					// GW address is neighbour as told by another link-local route
					return true
				}
				ifName, addr, err := ifmodel.ParseInterfaceAddressKey(key)
				if err == nil && ifName == route.OutgoingInterface && addr.Contains(gwAddr) {
					// GW address is inside the local network of the outgoing interface
					// as given by the assigned IP address
					return true
				}
				return false
			},
		})
	}*/
	return dependencies
}

// DerivedValues derives empty value under StaticLinkLocalRouteKey if route is link-local.
// It is used in dependencies for network reachability of a route gateway (see above).
func (d *RouteDescriptor) DerivedValues(key string, route *l3.StaticRoute) (derValues []scheduler.KeyValuePair) {
	/*if route.Scope == l3.LinuxStaticRoute_LINK {
		derValues = append(derValues, scheduler.KeyValuePair{
			Key:   l3.StaticLinkLocalRouteKey(route.DstNetwork, route.OutgoingInterface),
			Value: &prototypes.Empty{},
		})
	}*/
	return derValues
}

// Dump returns all routes associated with interfaces managed by this agent.
func (d *RouteDescriptor) Dump(correlate []adapter.StaticRouteKVWithMetadata) (
	dump []adapter.StaticRouteKVWithMetadata, err error,
) {
	// Retrieve VPP route configuration
	staticRoutes, err := d.routeHandler.DumpStaticRoutes()
	if err != nil {
		return nil, errors.Errorf("failed to dump VPP routes: %v", err)
	}

	for _, staticRoute := range staticRoutes {
		dump = append(dump, adapter.StaticRouteKVWithMetadata{
			Key:    l3.RouteKey(staticRoute.Route.VrfId, staticRoute.Route.DstNetwork, staticRoute.Route.NextHopAddr),
			Value:  staticRoute.Route,
			Origin: scheduler.UnknownOrigin,
		})
	}

	return dump, nil
}

// equalAddrs compares two IP addresses for equality.
func equalAddrs(addr1, addr2 string) bool {
	a1 := net.ParseIP(addr1)
	a2 := net.ParseIP(addr2)
	if a1 == nil || a2 == nil {
		// if parsing fails, compare as strings
		return strings.ToLower(addr1) == strings.ToLower(addr2)
	}
	return a1.Equal(a2)
}

// getGwAddr returns the GW address chosen in the given route, handling the cases
// when it is left undefined.
func getGwAddr(route *l3.StaticRoute) string {
	if route.GetNextHopAddr() != "" {
		return route.GetNextHopAddr()
	}
	// return zero address
	_, dstIPNet, err := net.ParseCIDR(route.GetDstNetwork())
	if err != nil {
		return ""
	}
	if dstIPNet.IP.To4() == nil {
		return net.IPv6zero.String()
	}
	return net.IPv4zero.String()
}

// getWeight returns static route weight, handling the cases when it is left undefined.
func getWeight(route *l3.StaticRoute) uint32 {
	if route.Weight == 0 {
		return defaultWeight
	}
	return route.Weight
}

// equalNetworks compares two IP networks for equality.
func equalNetworks(net1, net2 string) bool {
	_, n1, err1 := net.ParseCIDR(net1)
	_, n2, err2 := net.ParseCIDR(net2)
	if err1 != nil || err2 != nil {
		// if parsing fails, compare as strings
		return strings.ToLower(net1) == strings.ToLower(net2)
	}
	return n1.IP.Equal(n2.IP) && bytes.Equal(n1.Mask, n2.Mask)
}
