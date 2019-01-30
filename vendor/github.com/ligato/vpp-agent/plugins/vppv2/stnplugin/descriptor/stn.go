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
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/ligato/cn-infra/logging"
	scheduler "github.com/ligato/vpp-agent/plugins/kvscheduler/api"
	ifDescriptor "github.com/ligato/vpp-agent/plugins/vppv2/ifplugin/descriptor"
	"github.com/ligato/vpp-agent/plugins/vppv2/model/interfaces"
	"github.com/ligato/vpp-agent/plugins/vppv2/model/stn"
	"github.com/ligato/vpp-agent/plugins/vppv2/stnplugin/descriptor/adapter"
	"github.com/ligato/vpp-agent/plugins/vppv2/stnplugin/vppcalls"
	"github.com/pkg/errors"
)

const (
	// STNDescriptorName is the name of the descriptor for VPP STN rules
	STNDescriptorName = "vpp-stn-rules"

	// dependency labels
	stnInterfaceDep = "stn-interface-exists"
)

// A list of non-retriable errors:
var (
	// ErrSTNWithoutInterface is returned when VPP STN rule has undefined interface.
	ErrSTNWithoutInterface = errors.New("VPP STN rule defined without interface")

	// ErrSTNWithoutIPAddress is returned when VPP STN rule has undefined IP address.
	ErrSTNWithoutIPAddress = errors.New("VPP STN rule defined without IP address")
)

// STNDescriptor teaches KVScheduler how to configure VPP STN rules.
type STNDescriptor struct {
	// dependencies
	log        logging.Logger
	stnHandler vppcalls.StnVppAPI
}

// NewSTNDescriptor creates a new instance of the STN descriptor.
func NewSTNDescriptor(stnHandler vppcalls.StnVppAPI, log logging.PluginLogger) *STNDescriptor {
	return &STNDescriptor{
		log:        log.NewLogger("stn-descriptor"),
		stnHandler: stnHandler,
	}
}

// GetDescriptor returns descriptor suitable for registration (via adapter) with
// the KVScheduler.
func (d *STNDescriptor) GetDescriptor() *adapter.STNDescriptor {
	return &adapter.STNDescriptor{
		Name:               STNDescriptorName,
		KeySelector:        d.IsSTNKey,
		ValueTypeName:      proto.MessageName(&stn.Rule{}),
		ValueComparator:    d.EquivalentSTNs,
		NBKeyPrefix:        stn.Prefix,
		Add:                d.Add,
		Delete:             d.Delete,
		ModifyWithRecreate: d.ModifyWithRecreate,
		IsRetriableFailure: d.IsRetriableFailure,
		Dependencies:       d.Dependencies,
		Dump:               d.Dump,
		DumpDependencies:   []string{ifDescriptor.InterfaceDescriptorName},
	}
}

// IsSTNKey returns true if the key is identifying VPP STN rule configuration.
func (d *STNDescriptor) IsSTNKey(key string) bool {
	_, _, isSTNKey := stn.ParseKey(key)
	return isSTNKey
}

// EquivalentSTNs is case-insensitive comparison function for stn.Rule.
func (d *STNDescriptor) EquivalentSTNs(key string, oldSTN, newSTN *stn.Rule) bool {
	// parameters compared by proto equal
	if proto.Equal(oldSTN, newSTN) {
		return true
	}
	return false
}

// IsRetriableFailure returns <false> for errors related to invalid configuration.
func (d *STNDescriptor) IsRetriableFailure(err error) bool {
	nonRetriable := []error{
		ErrSTNWithoutInterface,
		ErrSTNWithoutIPAddress,
	}
	for _, nonRetriableErr := range nonRetriable {
		if err == nonRetriableErr {
			return false
		}
	}
	return true
}

// Add adds new STN rule.
func (d *STNDescriptor) Add(key string, stn *stn.Rule) (metadata interface{}, err error) {
	// remove mask from IP address if necessary
	ipParts := strings.Split(stn.IpAddress, "/")
	if len(ipParts) > 1 {
		d.log.Debugf("STN IP address %s is defined with mask, removing it")
		stn.IpAddress = ipParts[0]
	}

	// validate the configuration
	err = d.validateSTNConfig(stn)
	if err != nil {
		d.log.Error(err)
		return nil, err
	}

	// add STN rule
	err = d.stnHandler.AddSTNRule(stn)
	if err != nil {
		d.log.Error(err)
	}
	return nil, err
}

// Delete removes VPP STN rule.
func (d *STNDescriptor) Delete(key string, stn *stn.Rule, metadata interface{}) error {
	err := d.stnHandler.DeleteSTNRule(stn)
	if err != nil {
		d.log.Error(err)
	}
	return err
}

// ModifyWithRecreate always returns true - STN rules are always modified via re-creation.
func (d *STNDescriptor) ModifyWithRecreate(key string, oldSTN, newSTN *stn.Rule, metadata interface{}) bool {
	return true
}

// Dependencies for STN rule are represented by interface
func (d *STNDescriptor) Dependencies(key string, stn *stn.Rule) (dependencies []scheduler.Dependency) {
	dependencies = append(dependencies, scheduler.Dependency{
		Label: stnInterfaceDep,
		Key:   interfaces.InterfaceKey(stn.Interface),
	})
	return dependencies
}

// Dump returns all configured VPP STN rules.
func (d *STNDescriptor) Dump(correlate []adapter.STNKVWithMetadata) (dump []adapter.STNKVWithMetadata, err error) {
	stnRules, err := d.stnHandler.DumpSTNRules()
	if err != nil {
		d.log.Error(err)
		return dump, err
	}
	for _, stnRule := range stnRules {
		dump = append(dump, adapter.STNKVWithMetadata{
			Key:    stn.Key(stnRule.Rule.Interface, stnRule.Rule.IpAddress),
			Value:  stnRule.Rule,
			Origin: scheduler.FromNB, // all STN rules are configured from NB
		})
	}

	return dump, nil
}

// validateSTNConfig validates VPP STN rule configuration.
func (d *STNDescriptor) validateSTNConfig(stn *stn.Rule) error {
	if stn.Interface == "" {
		return ErrSTNWithoutInterface
	}
	if stn.IpAddress == "" {
		return ErrSTNWithoutIPAddress
	}
	return nil
}
