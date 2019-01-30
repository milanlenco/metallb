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
	"time"

	"github.com/ligato/cn-infra/datasync"
	"github.com/ligato/cn-infra/logging"

	kvs "github.com/ligato/vpp-agent/plugins/kvscheduler/api"
	"github.com/ligato/vpp-agent/plugins/kvscheduler/internal/utils"
)

// sbNotif encapsulates data for SB notification.
type sbNotif struct {
	value    kvs.KeyValuePair
	metadata kvs.Metadata
}

// txnResult represents transaction result.
type txnResult struct {
	err       error
	txnSeqNum uint64
}

// nbTxn encapsulates data for NB transaction.
type nbTxn struct {
	value           map[string]datasync.LazyValue // key -> lazy value
	resyncType      kvs.ResyncType
	verboseRefresh  bool
	isBlocking      bool
	retryFailed     bool
	retryPeriod     time.Duration
	expBackoffRetry bool
	revertOnFailure bool
	description     string
	resultChan      chan txnResult
}

// retryOps encapsulates data for retry of failed operations.
type retryOps struct {
	txnSeqNum uint64
	keys      utils.KeySet
	period    time.Duration
}

// queuedTxn represents transaction queued for execution.
type queuedTxn struct {
	txnType kvs.TxnType

	sb    *sbNotif
	nb    *nbTxn
	retry *retryOps
}

// enqueueTxn adds transaction into the FIFO queue (channel) for execution.
func (s *Scheduler) enqueueTxn(txn *queuedTxn) error {
	if txn.txnType == kvs.NBTransaction && txn.nb.isBlocking {
		select {
		case <-s.ctx.Done():
			return kvs.ErrClosedScheduler
		case s.txnQueue <- txn:
			return nil
		}
	}
	select {
	case <-s.ctx.Done():
		return kvs.ErrClosedScheduler
	case s.txnQueue <- txn:
		return nil
	default:
		return kvs.ErrTxnQueueFull
	}
}

// dequeueTxn pull the oldest queued transaction.
func (s *Scheduler) dequeueTxn() (txn *queuedTxn, canceled bool) {
	select {
	case <-s.ctx.Done():
		return nil, true
	case txn = <-s.txnQueue:
		return txn, false
	}
}

// enqueueRetry schedules retry for failed operations.
func (s *Scheduler) enqueueRetry(args *retryOps) {
	go s.delayRetry(args)
}

// delayRetry postpones retry until a given time period has elapsed.
func (s *Scheduler) delayRetry(args *retryOps) {
	s.wg.Add(1)
	defer s.wg.Done()

	select {
	case <-s.ctx.Done():
		return
	case <-time.After(args.period):
		err := s.enqueueTxn(&queuedTxn{txnType: kvs.RetryFailedOps, retry: args})
		if err != nil {
			s.Log.WithFields(logging.Fields{
				"txnSeqNum": args.txnSeqNum,
				"err":       err,
			}).Warn("Failed to enqueue re-try for failed operations")
			s.enqueueRetry(args) // try again with the same time period
		}
	}
}
