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

package kvscheduler

import (
	"sort"

	"github.com/ligato/cn-infra/logging"
	kvs "github.com/ligato/vpp-agent/plugins/kvscheduler/api"
	"github.com/ligato/vpp-agent/plugins/kvscheduler/internal/graph"
	"github.com/ligato/vpp-agent/plugins/kvscheduler/internal/utils"
)

// applyValueArgs collects all arguments to applyValue method.
type applyValueArgs struct {
	graphW graph.RWAccess
	txn    *preProcessedTxn
	kv     kvForTxn

	isRetry bool
	dryRun  bool

	// set inside of the recursive chain of applyValue-s
	isUpdate  bool
	isDerived bool

	// failed base values
	failed map[string]bool // key -> retriable?

	// handling of dependency cycles
	branch utils.KeySet
}

// addFailed adds entry into the *failed* map.
func (args *applyValueArgs) addFailed(key string, retriable bool) {
	prevRetriable, alreadyFailed := args.failed[key]
	args.failed[key] = retriable && (!alreadyFailed || prevRetriable)
}

// executeTransaction executes pre-processed transaction.
// If <dry-run> is enabled, Add/Delete/Update/Modify operations will not be executed
// and the graph will be returned to its original state at the end.
func (s *Scheduler) executeTransaction(txn *preProcessedTxn, dryRun bool) (executed kvs.RecordedTxnOps, failed map[string]bool) {
	downstreamResync := txn.args.txnType == kvs.NBTransaction && txn.args.nb.resyncType == kvs.DownstreamResync
	graphW := s.graph.Write(!downstreamResync)
	failed = make(map[string]bool)      // non-derived values in a failed state
	branch := utils.NewMapBasedKeySet() // branch of current recursive calls to applyValue used to handle cycles

	// for dry-run revert back the original content of the *lastError* map in the end
	if dryRun {
		prevLastError := make(map[string]error)
		for key, err := range s.lastError {
			prevLastError[key] = err
		}
		defer func() { s.lastError = prevLastError }()
	}

	var revert bool
	prevValues := make([]kvs.KeyValuePair, 0, len(txn.values))
	// execute transaction either in best-effort mode or with revert on the first failure
	for _, kv := range txn.values {
		ops, prevValue, err := s.applyValue(
			&applyValueArgs{
				graphW:  graphW,
				txn:     txn,
				kv:      kv,
				dryRun:  dryRun,
				isRetry: txn.args.txnType == kvs.RetryFailedOps,
				failed:  failed,
				branch:  branch,
			})
		executed = append(executed, ops...)
		prevValues = append(prevValues, kvs.KeyValuePair{})
		copy(prevValues[1:], prevValues)
		prevValues[0] = prevValue
		if err != nil {
			if txn.args.txnType == kvs.NBTransaction && txn.args.nb.revertOnFailure {
				// refresh failed value and trigger reverting
				delete(failed, kv.key) // do not retry unless reverting fails
				failedKey := utils.NewSingletonKeySet(kv.key)
				s.refreshGraph(graphW, failedKey, nil)
				graphW.Save() // certainly not dry-run
				revert = true
				break
			}
		}
	}

	if revert {
		// record graph state in-between failure and revert
		graphW.Release()
		graphW = s.graph.Write(true)

		// revert back to previous values
		for _, kvPair := range prevValues {
			ops, _, _ := s.applyValue(
				&applyValueArgs{
					graphW: graphW,
					txn:    txn,
					kv: kvForTxn{
						key:      kvPair.Key,
						value:    kvPair.Value,
						origin:   kvs.FromNB,
						isRevert: true,
					},
					dryRun: dryRun,
					failed: failed,
					branch: branch,
				})
			executed = append(executed, ops...)
		}
	}

	// get rid of uninteresting intermediate pending Add/Delete operations
	executed = s.compressTxnOps(executed)

	graphW.Release()
	return executed, failed
}

// applyValue applies new value received from NB or SB.
// It returns the list of executed operations.
func (s *Scheduler) applyValue(args *applyValueArgs) (executed kvs.RecordedTxnOps, prevValue kvs.KeyValuePair, err error) {
	// dependency cycle detection
	if cycle := args.branch.Has(args.kv.key); cycle {
		return executed, prevValue, err
	}
	args.branch.Add(args.kv.key)
	defer args.branch.Del(args.kv.key)

	// create new revision of the node for the given key-value pair
	node := args.graphW.SetNode(args.kv.key)

	// remember previous value for a potential revert
	prevValue = kvs.KeyValuePair{Key: node.GetKey(), Value: node.GetValue()}

	// update node flags
	prevUpdate := getNodeLastUpdate(node)
	if !args.isUpdate {
		// with update it is not certain if any update is actually needed,
		// so let applyUpdate() to refresh LastUpdateFlag
		node.SetFlags(&LastUpdateFlag{args.txn.seqNum})
	}
	if !args.isUpdate {
		if !args.isDerived {
			lastChangeFlag := &LastChangeFlag{
				txnSeqNum: args.txn.seqNum,
				value:     args.kv.value,
				origin:    args.kv.origin,
				revert:    args.kv.isRevert,
			}
			switch args.txn.args.txnType {
			case kvs.NBTransaction:
				lastChangeFlag.retryEnabled = args.txn.args.nb.retryFailed
				lastChangeFlag.retryPeriod = args.txn.args.nb.retryPeriod
				lastChangeFlag.retryExpBackoff = args.txn.args.nb.expBackoffRetry
			case kvs.RetryFailedOps:
				prevLastChange := getNodeLastChange(node)
				lastChangeFlag.retryEnabled = prevLastChange.retryEnabled
				lastChangeFlag.retryPeriod = prevLastChange.retryPeriod
				lastChangeFlag.retryExpBackoff = prevLastChange.retryExpBackoff
			}
			node.SetFlags(lastChangeFlag)
		} else {
			node.SetFlags(&DerivedFlag{})
		}
		node.SetFlags(&OriginFlag{args.kv.origin})
	}

	// if the value is already "broken" by this transaction, do not try to update
	// anymore, unless this is a revert
	// (needs to be refreshed first in the post-processing stage)
	prevErr := s.getNodeLastError(args.kv.key)
	if !args.kv.isRevert && prevErr != nil &&
		prevUpdate != nil && prevUpdate.txnSeqNum == args.txn.seqNum {
		return executed, prevValue, prevErr
	}

	// prepare operation description - fill attributes that we can even before executing the operation
	txnOp := s.preRecordTxnOp(args, node)

	// determine the operation type
	if args.isUpdate {
		txnOp.Operation = kvs.Update // triggered from within recursive applyValue-s
	} else if args.kv.value == nil {
		txnOp.Operation = kvs.Delete
	} else if node.GetValue() == nil || isNodePending(node) {
		txnOp.Operation = kvs.Add
	} else {
		txnOp.Operation = kvs.Modify
	}

	// remaining txnOp attributes to fill:
	//		IsPending  bool
	//		NewErr     error

	switch txnOp.Operation {
	case kvs.Delete:
		executed, err = s.applyDelete(node, txnOp, args, false)
	case kvs.Add:
		executed, err = s.applyAdd(node, txnOp, args)
	case kvs.Modify:
		executed, err = s.applyModify(node, txnOp, args)
	case kvs.Update:
		executed, err = s.applyUpdate(node, txnOp, args)
	}

	return executed, prevValue, err
}

// applyDelete either deletes value or moves it to the pending state.
func (s *Scheduler) applyDelete(node graph.NodeRW, txnOp *kvs.RecordedTxnOp, args *applyValueArgs, pending bool) (executed kvs.RecordedTxnOps, err error) {
	if !args.dryRun {
		defer args.graphW.Save()
	}

	if node.GetValue() == nil {
		// remove value that does not exist => noop
		args.graphW.DeleteNode(args.kv.key)
		return executed, nil
	}

	if isNodePending(node) {
		// removing value that was pending => just remove from the in-memory graph
		args.graphW.DeleteNode(args.kv.key)
		s.lastError[node.GetKey()] = nil
		return kvs.RecordedTxnOps{txnOp}, nil
	}

	if pending {
		// already mark as pending so that other nodes will not view it as satisfied
		// dependency during removal
		node.SetFlags(&PendingFlag{})
	}

	// remove derived values
	var derivedVals []kvForTxn
	for _, derivedNode := range getDerivedNodes(node) {
		derivedVals = append(derivedVals, kvForTxn{
			key:      derivedNode.GetKey(),
			value:    nil, // delete
			origin:   args.kv.origin,
			isRevert: args.kv.isRevert,
		})
	}
	derExecs, wasErr := s.applyDerived(derivedVals, args, false)
	executed = append(executed, derExecs...)

	// continue even if removal of a derived value has failed ...

	// update values that depend on this kv-pair
	executed = append(executed, s.runUpdates(node, args)...)

	// execute delete operation
	descriptor := s.registry.GetDescriptorForKey(node.GetKey())
	handler := &descriptorHandler{descriptor}
	if !args.dryRun && descriptor != nil {
		if args.kv.origin != kvs.FromSB {
			err = handler.delete(node.GetKey(), node.GetValue(), node.GetMetadata())
		}
		s.lastError[node.GetKey()] = err
		if err != nil {
			wasErr = err
			// propagate error to the base value
			args.addFailed(getNodeBase(node).GetKey(), handler.isRetriableFailure(err))
			s.propagateError(args.graphW, node, err, kvs.Delete)

		}
		if canNodeHaveMetadata(node) && descriptor.WithMetadata {
			node.SetMetadata(nil)
		}
	} else {
		s.lastError[node.GetKey()] = nil // for dry-run assume success
	}

	// cleanup the error flag if removal was successful
	if wasErr == nil {
		node.DelFlags(ErrorFlagName)
	}

	// remove non-pending derived value regardless of errors, base-value only
	// if removal was completely successful
	if !pending && (wasErr == nil || isNodeDerived(node)) {
		args.graphW.DeleteNode(args.kv.key)
	}

	txnOp.NewErr = err
	txnOp.IsPending = pending
	executed = append(executed, txnOp)
	return executed, wasErr
}

// applyAdd adds new value which previously didn't exist or was pending.
func (s *Scheduler) applyAdd(node graph.NodeRW, txnOp *kvs.RecordedTxnOp, args *applyValueArgs) (executed kvs.RecordedTxnOps, err error) {
	if !args.dryRun {
		defer args.graphW.Save()
	}
	node.SetValue(args.kv.value)

	// get descriptor
	descriptor := s.registry.GetDescriptorForKey(args.kv.key)
	handler := &descriptorHandler{descriptor}
	if descriptor != nil {
		node.SetFlags(&DescriptorFlag{descriptor.Name})
		node.SetLabel(handler.keyLabel(args.kv.key))
	}

	// build relations with other targets
	derives := handler.derivedValues(node.GetKey(), node.GetValue())
	dependencies := handler.dependencies(node.GetKey(), node.GetValue())
	node.SetTargets(constructTargets(dependencies, derives))

	if !isNodeReady(node) {
		// if not ready, nothing to do
		node.SetFlags(&PendingFlag{})
		node.DelFlags(ErrorFlagName)
		txnOp.IsPending = true
		s.lastError[node.GetKey()] = nil
		return kvs.RecordedTxnOps{txnOp}, nil
	}

	// execute add operation
	if !args.dryRun && descriptor != nil {
		var (
			err      error
			metadata interface{}
		)

		if args.kv.origin != kvs.FromSB {
			metadata, err = handler.add(node.GetKey(), node.GetValue())
		} else {
			// already added in SB
			metadata = args.kv.metadata
		}
		s.lastError[node.GetKey()] = err

		if err != nil {
			// propate error to the base value
			args.addFailed(getNodeBase(node).GetKey(), handler.isRetriableFailure(err))
			s.propagateError(args.graphW, node, err, kvs.Add)
			// add failed => keep value pending
			node.SetFlags(&PendingFlag{})
			txnOp.IsPending = true
			txnOp.NewErr = err
			return kvs.RecordedTxnOps{txnOp}, err
		}

		// add metadata to the map
		if canNodeHaveMetadata(node) && descriptor.WithMetadata {
			node.SetMetadataMap(descriptor.Name)
			node.SetMetadata(metadata)
		}
	} else {
		s.lastError[node.GetKey()] = nil // for dry-run assume success
	}

	// finalize node and save before going to derived values + dependencies
	node.DelFlags(ErrorFlagName, PendingFlagName)
	executed = append(executed, txnOp)
	if !args.dryRun {
		args.graphW.Save()
	}

	// update values that depend on this kv-pair
	executed = append(executed, s.runUpdates(node, args)...)

	// created derived values
	var derivedVals []kvForTxn
	for _, derivedVal := range derives {
		derivedVals = append(derivedVals, kvForTxn{
			key:      derivedVal.Key,
			value:    derivedVal.Value,
			origin:   args.kv.origin,
			isRevert: args.kv.isRevert,
		})
	}
	derExecs, wasErr := s.applyDerived(derivedVals, args, true)
	executed = append(executed, derExecs...)

	return executed, wasErr
}

// applyModify applies new value to existing non-pending value.
func (s *Scheduler) applyModify(node graph.NodeRW, txnOp *kvs.RecordedTxnOp, args *applyValueArgs) (executed kvs.RecordedTxnOps, err error) {
	if !args.dryRun {
		defer args.graphW.Save()
	}

	// compare new value with the old one
	descriptor := s.registry.GetDescriptorForKey(args.kv.key)
	handler := &descriptorHandler{descriptor}
	equivalent := handler.equivalentValues(node.GetKey(), node.GetValue(), args.kv.value)

	// re-create the value if required by the descriptor
	recreate := !equivalent &&
		args.kv.origin != kvs.FromSB &&
		handler.modifyWithRecreate(args.kv.key, node.GetValue(), args.kv.value, node.GetMetadata())

	if recreate {
		// record operation as two - delete followed by add
		delOp := s.preRecordTxnOp(args, node)
		delOp.Operation = kvs.Delete
		delOp.NewValue = nil
		addOp := s.preRecordTxnOp(args, node)
		addOp.Operation = kvs.Add
		addOp.PrevValue = nil
		addOp.WasPending = true
		// remove obsolete value
		delExec, err := s.applyDelete(node, delOp, args, true)
		executed = append(executed, delExec...)
		if err != nil {
			return executed, err
		}
		// add the new revision of the value
		addExec, err := s.applyAdd(node, addOp, args)
		executed = append(executed, addExec...)
		return executed, err
	}

	// save the new value
	prevValue := node.GetValue()
	node.SetValue(args.kv.value)

	// get the set of derived keys before modification
	prevDerived := getDerivedKeys(node)

	// set new targets
	derives := handler.derivedValues(node.GetKey(), node.GetValue())
	dependencies := handler.dependencies(node.GetKey(), node.GetValue())
	node.SetTargets(constructTargets(dependencies, derives))

	// remove obsolete derived values
	var obsoleteDerVals []kvForTxn
	prevDerived.Subtract(getDerivedKeys(node))
	for _, obsolete := range prevDerived.Iterate() {
		obsoleteDerVals = append(obsoleteDerVals, kvForTxn{
			key:      obsolete,
			value:    nil, // delete
			origin:   args.kv.origin,
			isRevert: args.kv.isRevert,
		})
	}
	derExecs, wasErr := s.applyDerived(obsoleteDerVals, args, false)
	executed = append(executed, derExecs...)

	// if the new dependencies are not satisfied => delete and set as pending with the new value
	if !isNodeReady(node) {
		delExec, err := s.applyDelete(node, txnOp, args, true)
		executed = append(executed, delExec...)
		if err != nil {
			wasErr = err
		}
		return executed, wasErr
	}

	// execute modify operation
	if !args.dryRun && !equivalent && descriptor != nil {
		var newMetadata interface{}

		// call Modify handler
		if args.kv.origin != kvs.FromSB {
			newMetadata, err = handler.modify(node.GetKey(), prevValue, node.GetValue(), node.GetMetadata())
		} else {
			// already modified in SB
			newMetadata = args.kv.metadata
		}
		s.lastError[node.GetKey()] = err

		if err != nil {
			// propagate error to the base value
			s.propagateError(args.graphW, node, err, kvs.Modify)
			args.addFailed(getNodeBase(node).GetKey(), handler.isRetriableFailure(err))
			// record transaction operation
			txnOp.NewErr = err
			executed = append(executed, txnOp)
			return executed, err
		}

		// update metadata
		if canNodeHaveMetadata(node) && descriptor.WithMetadata {
			node.SetMetadata(newMetadata)
		}
	} else {
		s.lastError[node.GetKey()] = nil // for dry-run assume success
	}

	// if new value is equivalent, but the value is in failed state from previous txn => run update
	if equivalent && wasErr == nil && s.getNodeLastError(node.GetKey()) != nil {
		txnOp.Operation = kvs.Update

		// call Update handler
		if !args.dryRun && args.kv.origin != kvs.FromSB {
			err = handler.update(node.GetKey(), node.GetValue(), node.GetMetadata())
		}
		s.lastError[node.GetKey()] = err

		if err != nil {
			// propagate error to the base value
			s.propagateError(args.graphW, node, err, kvs.Update)
			args.addFailed(getNodeBase(node).GetKey(), handler.isRetriableFailure(err))
			// record transaction operation
			txnOp.NewErr = err
			executed = append(executed, txnOp)
			return executed, err
		}
	}

	if !equivalent || txnOp.Operation == kvs.Update {
		// if the value was modified, or update was executed (to clear error) => record operation
		executed = append(executed, txnOp)
	}

	// finalize node and save before going to new/modified derived values + dependencies
	if wasErr == nil {
		node.DelFlags(ErrorFlagName)
	}
	if !args.dryRun {
		args.graphW.Save()
	}

	// update values that depend on this kv-pair
	if !equivalent {
		executed = append(executed, s.runUpdates(node, args)...)
	}

	// modify/add derived values
	var derivedVals []kvForTxn
	for _, derivedVal := range derives {
		derivedVals = append(derivedVals, kvForTxn{
			key:      derivedVal.Key,
			value:    derivedVal.Value,
			origin:   args.kv.origin,
			isRevert: args.kv.isRevert,
		})
	}
	derExecs, err = s.applyDerived(derivedVals, args, true)
	executed = append(executed, derExecs...)
	if err != nil {
		wasErr = err
	}

	return executed, wasErr
}

// applyUpdate updates given value since dependencies have changed.
func (s *Scheduler) applyUpdate(node graph.NodeRW, txnOp *kvs.RecordedTxnOp, args *applyValueArgs) (executed kvs.RecordedTxnOps, err error) {
	descriptor := s.registry.GetDescriptorForKey(args.kv.key)
	handler := &descriptorHandler{descriptor}

	// add node if dependencies are now all met
	if isNodePending(node) {
		if !isNodeReady(node) {
			// nothing to do - do NOT refresh LastUpdateFlag
			return executed, nil
		}
		node.SetFlags(&LastUpdateFlag{args.txn.seqNum})
		addOp := s.preRecordTxnOp(args, node)
		addOp.Operation = kvs.Add
		executed, err = s.applyAdd(node, addOp, args)
	} else {
		node.SetFlags(&LastUpdateFlag{args.txn.seqNum})
		// node is not pending
		if !isNodeReady(node) {
			// delete value and flag node as pending if some dependency is no longer satisfied
			delOp := s.preRecordTxnOp(args, node)
			delOp.Operation = kvs.Delete
			delOp.NewValue = nil
			executed, err = s.applyDelete(node, delOp, args, true)
		} else {
			// execute Update operation
			if !args.dryRun {
				err = handler.update(node.GetKey(), node.GetValue(), node.GetMetadata())
				s.lastError[node.GetKey()] = err
				if err != nil {
					// propagate error to the base value
					txnOp.NewErr = err
					s.propagateError(args.graphW, node, err, kvs.Update)
					args.addFailed(getNodeBase(node).GetKey(), handler.isRetriableFailure(err))
				}
			} else {
				s.lastError[node.GetKey()] = nil // for dry-run assume success
			}
			executed = append(executed, txnOp)
		}
	}

	return executed, err
}

// applyDerived (re-)applies the given list of derived values.
func (s *Scheduler) applyDerived(derivedVals []kvForTxn, args *applyValueArgs, check bool) (executed kvs.RecordedTxnOps, err error) {
	var wasErr error

	// order derivedVals by key (just for deterministic behaviour which simplifies testing)
	sort.Slice(derivedVals, func(i, j int) bool { return derivedVals[i].key < derivedVals[j].key })

	for _, derived := range derivedVals {
		if check && !s.validDerivedKV(args.graphW, derived, args.txn.seqNum) {
			continue
		}
		ops, _, err := s.applyValue(
			&applyValueArgs{
				graphW:    args.graphW,
				txn:       args.txn,
				kv:        derived,
				isRetry:   args.isRetry,
				dryRun:    args.dryRun,
				isDerived: true, // <- is derived
				failed:    args.failed,
				branch:    args.branch,
			})
		if err != nil {
			wasErr = err
		}
		executed = append(executed, ops...)
	}
	return executed, wasErr
}

// runUpdates triggers updates on all nodes that depend on the given node.
func (s *Scheduler) runUpdates(node graph.Node, args *applyValueArgs) (executed kvs.RecordedTxnOps) {
	depNodes := node.GetSources(DependencyRelation)

	// order depNodes by key (just for deterministic behaviour which simplifies testing)
	sort.Slice(depNodes, func(i, j int) bool { return depNodes[i].GetKey() < depNodes[j].GetKey() })

	for _, depNode := range depNodes {
		if getNodeOrigin(depNode) != kvs.FromNB {
			continue
		}
		ops, _, _ := s.applyValue(
			&applyValueArgs{
				graphW: args.graphW,
				txn:    args.txn,
				kv: kvForTxn{
					key:      depNode.GetKey(),
					value:    depNode.GetValue(),
					origin:   getNodeOrigin(depNode),
					isRevert: args.kv.isRevert,
				},
				isRetry:  args.isRetry,
				dryRun:   args.dryRun,
				isUpdate: true, // <- update
				failed:   args.failed,
				branch:   args.branch,
			})
		executed = append(executed, ops...)
	}
	return executed
}

// compressTxnOps removes uninteresting intermediate pending Add/Delete operations.
func (s *Scheduler) compressTxnOps(executed kvs.RecordedTxnOps) kvs.RecordedTxnOps {
	// compress Add operations
	compressed := make(kvs.RecordedTxnOps, 0, len(executed))
	for i, op := range executed {
		compressedOp := false
		if op.Operation == kvs.Add && op.IsPending && op.NewErr == nil {
			for j := i + 1; j < len(executed); j++ {
				if executed[j].Key == op.Key {
					if executed[j].Operation == kvs.Add {
						// compress
						compressedOp = true
						executed[j].PrevValue = op.PrevValue
						executed[j].PrevErr = op.PrevErr
						executed[j].PrevOrigin = op.PrevOrigin
						executed[j].WasPending = op.WasPending
					}
					break
				}
			}
		}
		if !compressedOp {
			compressed = append(compressed, op)
		}
	}

	// compress Delete operations
	length := len(compressed)
	for i := length - 1; i >= 0; i-- {
		op := compressed[i]
		compressedOp := false
		if op.Operation == kvs.Delete && op.WasPending && op.PrevErr == nil {
			for j := i - 1; j >= 0; j-- {
				if compressed[j].Key == op.Key {
					if compressed[j].Operation == kvs.Delete {
						// compress
						compressedOp = true
						compressed[j].NewValue = op.NewValue
						compressed[j].NewErr = op.NewErr
						compressed[j].NewOrigin = op.NewOrigin
						compressed[j].IsPending = op.IsPending
					}
					break
				}
			}
		}
		if compressedOp {
			copy(compressed[i:], compressed[i+1:])
			length--
		}
	}
	compressed = compressed[:length]
	return compressed
}

// getNodeLastError return errors (or nil) from the last operation executed
// for the given key.
// This is not the same as reading the ErrorFlag, because the flag carries error
// potentially propagated from derived values down to the base.
func (s *Scheduler) getNodeLastError(key string) error {
	err, hasEntry := s.lastError[key]
	if hasEntry {
		return err
	}
	return nil
}

// propagateError propagates error from a given node into its base and saves it
// using the ErrorFlag.
func (s *Scheduler) propagateError(graphW graph.RWAccess, node graph.Node, err error, txnOp kvs.TxnOperation) {
	baseKey := getNodeBase(node).GetKey()
	baseNode := graphW.SetNode(baseKey)
	baseNode.SetFlags(&ErrorFlag{err: err, txnOp: txnOp})
}

// validDerivedKV check validity of a derived KV pair.
func (s *Scheduler) validDerivedKV(graphR graph.ReadAccess, kv kvForTxn, txnSeqNum uint64) bool {
	node := graphR.GetNode(kv.key)
	if kv.value == nil {
		s.Log.WithFields(logging.Fields{
			"txnSeqNum": txnSeqNum,
			"key":       kv.key,
		}).Warn("Derived nil value")
		return false
	}
	if node != nil {
		if !isNodeDerived(node) {
			s.Log.WithFields(logging.Fields{
				"txnSeqNum": txnSeqNum,
				"value":     kv.value,
				"key":       kv.key,
			}).Warn("Skipping derived value colliding with a base value")
			return false
		}
	}
	return true
}
