// Code generated by adapter-generator. DO NOT EDIT.

package adapter

import (
	"github.com/gogo/protobuf/proto"
	. "github.com/ligato/vpp-agent/plugins/kvscheduler/api"
	"github.com/ligato/vpp-agent/plugins/vppv2/model/nat"
)

////////// type-safe key-value pair with metadata //////////

type NAT44GlobalKVWithMetadata struct {
	Key      string
	Value    *nat.Nat44Global
	Metadata interface{}
	Origin   ValueOrigin
}

////////// type-safe Descriptor structure //////////

type NAT44GlobalDescriptor struct {
	Name               string
	KeySelector        KeySelector
	ValueTypeName      string
	KeyLabel           func(key string) string
	ValueComparator    func(key string, oldValue, newValue *nat.Nat44Global) bool
	NBKeyPrefix        string
	WithMetadata       bool
	MetadataMapFactory MetadataMapFactory
	Add                func(key string, value *nat.Nat44Global) (metadata interface{}, err error)
	Delete             func(key string, value *nat.Nat44Global, metadata interface{}) error
	Modify             func(key string, oldValue, newValue *nat.Nat44Global, oldMetadata interface{}) (newMetadata interface{}, err error)
	ModifyWithRecreate func(key string, oldValue, newValue *nat.Nat44Global, metadata interface{}) bool
	Update             func(key string, value *nat.Nat44Global, metadata interface{}) error
	IsRetriableFailure func(err error) bool
	Dependencies       func(key string, value *nat.Nat44Global) []Dependency
	DerivedValues      func(key string, value *nat.Nat44Global) []KeyValuePair
	Dump               func(correlate []NAT44GlobalKVWithMetadata) ([]NAT44GlobalKVWithMetadata, error)
	DumpDependencies   []string /* descriptor name */
}

////////// Descriptor adapter //////////

type NAT44GlobalDescriptorAdapter struct {
	descriptor *NAT44GlobalDescriptor
}

func NewNAT44GlobalDescriptor(typedDescriptor *NAT44GlobalDescriptor) *KVDescriptor {
	adapter := &NAT44GlobalDescriptorAdapter{descriptor: typedDescriptor}
	descriptor := &KVDescriptor{
		Name:               typedDescriptor.Name,
		KeySelector:        typedDescriptor.KeySelector,
		ValueTypeName:      typedDescriptor.ValueTypeName,
		KeyLabel:           typedDescriptor.KeyLabel,
		NBKeyPrefix:        typedDescriptor.NBKeyPrefix,
		WithMetadata:       typedDescriptor.WithMetadata,
		MetadataMapFactory: typedDescriptor.MetadataMapFactory,
		IsRetriableFailure: typedDescriptor.IsRetriableFailure,
		DumpDependencies:   typedDescriptor.DumpDependencies,
	}
	if typedDescriptor.ValueComparator != nil {
		descriptor.ValueComparator = adapter.ValueComparator
	}
	if typedDescriptor.Add != nil {
		descriptor.Add = adapter.Add
	}
	if typedDescriptor.Delete != nil {
		descriptor.Delete = adapter.Delete
	}
	if typedDescriptor.Modify != nil {
		descriptor.Modify = adapter.Modify
	}
	if typedDescriptor.ModifyWithRecreate != nil {
		descriptor.ModifyWithRecreate = adapter.ModifyWithRecreate
	}
	if typedDescriptor.Update != nil {
		descriptor.Update = adapter.Update
	}
	if typedDescriptor.Dependencies != nil {
		descriptor.Dependencies = adapter.Dependencies
	}
	if typedDescriptor.DerivedValues != nil {
		descriptor.DerivedValues = adapter.DerivedValues
	}
	if typedDescriptor.Dump != nil {
		descriptor.Dump = adapter.Dump
	}
	return descriptor
}

func (da *NAT44GlobalDescriptorAdapter) ValueComparator(key string, oldValue, newValue proto.Message) bool {
	typedOldValue, err1 := castNAT44GlobalValue(key, oldValue)
	typedNewValue, err2 := castNAT44GlobalValue(key, newValue)
	if err1 != nil || err2 != nil {
		return false
	}
	return da.descriptor.ValueComparator(key, typedOldValue, typedNewValue)
}

func (da *NAT44GlobalDescriptorAdapter) Add(key string, value proto.Message) (metadata Metadata, err error) {
	typedValue, err := castNAT44GlobalValue(key, value)
	if err != nil {
		return nil, err
	}
	return da.descriptor.Add(key, typedValue)
}

func (da *NAT44GlobalDescriptorAdapter) Modify(key string, oldValue, newValue proto.Message, oldMetadata Metadata) (newMetadata Metadata, err error) {
	oldTypedValue, err := castNAT44GlobalValue(key, oldValue)
	if err != nil {
		return nil, err
	}
	newTypedValue, err := castNAT44GlobalValue(key, newValue)
	if err != nil {
		return nil, err
	}
	typedOldMetadata, err := castNAT44GlobalMetadata(key, oldMetadata)
	if err != nil {
		return nil, err
	}
	return da.descriptor.Modify(key, oldTypedValue, newTypedValue, typedOldMetadata)
}

func (da *NAT44GlobalDescriptorAdapter) Delete(key string, value proto.Message, metadata Metadata) error {
	typedValue, err := castNAT44GlobalValue(key, value)
	if err != nil {
		return err
	}
	typedMetadata, err := castNAT44GlobalMetadata(key, metadata)
	if err != nil {
		return err
	}
	return da.descriptor.Delete(key, typedValue, typedMetadata)
}

func (da *NAT44GlobalDescriptorAdapter) ModifyWithRecreate(key string, oldValue, newValue proto.Message, metadata Metadata) bool {
	oldTypedValue, err := castNAT44GlobalValue(key, oldValue)
	if err != nil {
		return true
	}
	newTypedValue, err := castNAT44GlobalValue(key, newValue)
	if err != nil {
		return true
	}
	typedMetadata, err := castNAT44GlobalMetadata(key, metadata)
	if err != nil {
		return true
	}
	return da.descriptor.ModifyWithRecreate(key, oldTypedValue, newTypedValue, typedMetadata)
}

func (da *NAT44GlobalDescriptorAdapter) Update(key string, value proto.Message, metadata Metadata) error {
	typedValue, err := castNAT44GlobalValue(key, value)
	if err != nil {
		return err
	}
	typedMetadata, err := castNAT44GlobalMetadata(key, metadata)
	if err != nil {
		return err
	}
	return da.descriptor.Update(key, typedValue, typedMetadata)
}

func (da *NAT44GlobalDescriptorAdapter) Dependencies(key string, value proto.Message) []Dependency {
	typedValue, err := castNAT44GlobalValue(key, value)
	if err != nil {
		return nil
	}
	return da.descriptor.Dependencies(key, typedValue)
}

func (da *NAT44GlobalDescriptorAdapter) DerivedValues(key string, value proto.Message) []KeyValuePair {
	typedValue, err := castNAT44GlobalValue(key, value)
	if err != nil {
		return nil
	}
	return da.descriptor.DerivedValues(key, typedValue)
}

func (da *NAT44GlobalDescriptorAdapter) Dump(correlate []KVWithMetadata) ([]KVWithMetadata, error) {
	var correlateWithType []NAT44GlobalKVWithMetadata
	for _, kvpair := range correlate {
		typedValue, err := castNAT44GlobalValue(kvpair.Key, kvpair.Value)
		if err != nil {
			continue
		}
		typedMetadata, err := castNAT44GlobalMetadata(kvpair.Key, kvpair.Metadata)
		if err != nil {
			continue
		}
		correlateWithType = append(correlateWithType,
			NAT44GlobalKVWithMetadata{
				Key:      kvpair.Key,
				Value:    typedValue,
				Metadata: typedMetadata,
				Origin:   kvpair.Origin,
			})
	}

	typedDump, err := da.descriptor.Dump(correlateWithType)
	if err != nil {
		return nil, err
	}
	var dump []KVWithMetadata
	for _, typedKVWithMetadata := range typedDump {
		kvWithMetadata := KVWithMetadata{
			Key:      typedKVWithMetadata.Key,
			Metadata: typedKVWithMetadata.Metadata,
			Origin:   typedKVWithMetadata.Origin,
		}
		kvWithMetadata.Value = typedKVWithMetadata.Value
		dump = append(dump, kvWithMetadata)
	}
	return dump, err
}

////////// Helper methods //////////

func castNAT44GlobalValue(key string, value proto.Message) (*nat.Nat44Global, error) {
	typedValue, ok := value.(*nat.Nat44Global)
	if !ok {
		return nil, ErrInvalidValueType(key, value)
	}
	return typedValue, nil
}

func castNAT44GlobalMetadata(key string, metadata Metadata) (interface{}, error) {
	if metadata == nil {
		return nil, nil
	}
	typedMetadata, ok := metadata.(interface{})
	if !ok {
		return nil, ErrInvalidMetadataType(key)
	}
	return typedMetadata, nil
}
