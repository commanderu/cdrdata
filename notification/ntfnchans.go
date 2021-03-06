// Copyright (c) 2017, TJI
// See LICENSE for details.

package notification

import (
	"github.com/commanderu/cdr/chaincfg/chainhash"
	"github.com/commanderu/cdr/cdrutil"

	"github.com/commanderu/cdrata/blockdata"
	"github.com/commanderu/cdrata/db/cdrsqlite"
	"github.com/commanderu/cdrata/explorer"
	"github.com/commanderu/cdrata/mempool"
	"github.com/commanderu/cdrata/stakedb"
	"github.com/commanderu/cdrata/txhelpers"
)

const (
	// blockConnChanBuffer is the size of the block connected channel buffers.
	blockConnChanBuffer = 64

	// newTxChanBuffer is the size of the new transaction channel buffer, for
	// ANY transactions are added into mempool.
	newTxChanBuffer = 48

	// expNewTxChanBuffer is the size of the new transaction buffer for explorer
	expNewTxChanBuffer = 70

	reorgBuffer = 2

	// relevantMempoolTxChanBuffer is the size of the new transaction channel
	// buffer, for relevant transactions that are added into mempool.
	//relevantMempoolTxChanBuffer = 2048
)

// Channels are package-level variables for simplicity
var NtfnChans struct {
	ConnectChan                       chan *chainhash.Hash
	ReorgChanBlockData                chan *blockdata.ReorgData
	ConnectChanWiredDB                chan *chainhash.Hash
	ReorgChanWiredDB                  chan *cdrsqlite.ReorgData
	ConnectChanStakeDB                chan *chainhash.Hash
	ReorgChanStakeDB                  chan *stakedb.ReorgData
	UpdateStatusNodeHeight            chan uint32
	UpdateStatusDBHeight              chan uint32
	SpendTxBlockChan, RecvTxBlockChan chan *txhelpers.BlockWatchedTx
	RelevantTxMempoolChan             chan *cdrutil.Tx
	NewTxChan                         chan *mempool.NewTx
	ExpNewTxChan                      chan *explorer.NewMempoolTx
}

// MakeNtfnChans create notification channels based on config
func MakeNtfnChans(monitorMempool bool) {
	// If we're monitoring for blocks OR collecting block data, these channels
	// are necessary to handle new block notifications. Otherwise, leave them
	// as nil so that both a send (below) blocks and a receive (in
	// blockConnectedHandler) block. default case makes non-blocking below.
	// quit channel case manages blockConnectedHandlers.
	NtfnChans.ConnectChan = make(chan *chainhash.Hash, blockConnChanBuffer)

	// WiredDB channel for connecting new blocks
	NtfnChans.ConnectChanWiredDB = make(chan *chainhash.Hash, blockConnChanBuffer)

	// Stake DB channel for connecting new blocks - BLOCKING!
	NtfnChans.ConnectChanStakeDB = make(chan *chainhash.Hash)

	// Reorg data channels
	NtfnChans.ReorgChanBlockData = make(chan *blockdata.ReorgData, reorgBuffer)
	NtfnChans.ReorgChanWiredDB = make(chan *cdrsqlite.ReorgData, reorgBuffer)
	NtfnChans.ReorgChanStakeDB = make(chan *stakedb.ReorgData, reorgBuffer)

	// To update app status
	NtfnChans.UpdateStatusNodeHeight = make(chan uint32, blockConnChanBuffer)
	NtfnChans.UpdateStatusDBHeight = make(chan uint32, blockConnChanBuffer)

	// watchaddress
	// if len(cfg.WatchAddresses) > 0 {
	// // recv/SpendTxBlockChan come with connected blocks
	// 	NtfnChans.RecvTxBlockChan = make(chan *txhelpers.BlockWatchedTx, blockConnChanBuffer)
	// 	NtfnChans.SpendTxBlockChan = make(chan *txhelpers.BlockWatchedTx, blockConnChanBuffer)
	// 	NtfnChans.RelevantTxMempoolChan = make(chan *cdrutil.Tx, relevantMempoolTxChanBuffer)
	// }

	if monitorMempool {
		NtfnChans.NewTxChan = make(chan *mempool.NewTx, newTxChanBuffer)
	}

	// New mempool tx chan for explorer
	NtfnChans.ExpNewTxChan = make(chan *explorer.NewMempoolTx, expNewTxChanBuffer)
}

// CloseNtfnChans close all notification channels
func CloseNtfnChans() {
	if NtfnChans.ConnectChan != nil {
		close(NtfnChans.ConnectChan)
	}
	if NtfnChans.ConnectChanWiredDB != nil {
		close(NtfnChans.ConnectChanWiredDB)
	}
	if NtfnChans.ConnectChanStakeDB != nil {
		close(NtfnChans.ConnectChanStakeDB)
	}

	if NtfnChans.ReorgChanBlockData != nil {
		close(NtfnChans.ReorgChanBlockData)
	}
	if NtfnChans.ReorgChanWiredDB != nil {
		close(NtfnChans.ReorgChanWiredDB)
	}
	if NtfnChans.ReorgChanStakeDB != nil {
		close(NtfnChans.ReorgChanStakeDB)
	}

	if NtfnChans.UpdateStatusNodeHeight != nil {
		close(NtfnChans.UpdateStatusNodeHeight)
	}
	if NtfnChans.UpdateStatusDBHeight != nil {
		close(NtfnChans.UpdateStatusDBHeight)
	}

	if NtfnChans.NewTxChan != nil {
		close(NtfnChans.NewTxChan)
	}
	if NtfnChans.RelevantTxMempoolChan != nil {
		close(NtfnChans.RelevantTxMempoolChan)
	}

	if NtfnChans.SpendTxBlockChan != nil {
		close(NtfnChans.SpendTxBlockChan)
	}
	if NtfnChans.RecvTxBlockChan != nil {
		close(NtfnChans.RecvTxBlockChan)
	}

	if NtfnChans.ExpNewTxChan != nil {
		close(NtfnChans.ExpNewTxChan)
	}
}
