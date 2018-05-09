// Copyright (c) 2018, The commanderu developers
// Copyright (c) 2017, TJI
// See LICENSE for details.

package notification

import (
	"strings"
	"sync"
	"time"

	"github.com/commanderu/cdr/chaincfg/chainhash"
	"github.com/commanderu/cdr/cdrjson"
	"github.com/commanderu/cdr/cdrutil"
	"github.com/commanderu/cdr/rpcclient"
	"github.com/commanderu/cdr/wire"
	"github.com/commanderu/cdrata/blockdata"
	"github.com/commanderu/cdrata/db/cdrsqlite"
	"github.com/commanderu/cdrata/explorer"
	"github.com/commanderu/cdrata/mempool"
	"github.com/commanderu/cdrata/stakedb"
	"github.com/commanderu/cdrwallet/wallet/udb"
)

// RegisterNodeNtfnHandlers registers with cdr to receive new block,
// transaction and winning ticket notifications.
func RegisterNodeNtfnHandlers(cdrClient *rpcclient.Client) *ContextualError {
	var err error
	// Register for block connection and chain reorg notifications.
	if err = cdrClient.NotifyBlocks(); err != nil {
		return newContextualError("block notification "+
			"registration failed", err)
	}

	// Register for tx accepted into mempool ntfns
	if err = cdrClient.NotifyNewTransactions(true); err != nil {
		return newContextualError("new transaction verbose notification registration failed", err)
	}

	// For OnNewTickets
	//  Commented since there is a bug in rpcclient/notify.go
	// cdrClient.NotifyNewTickets()

	if err = cdrClient.NotifyWinningTickets(); err != nil {
		return newContextualError("winning ticket "+
			"notification registration failed", err)
	}

	// Register a Tx filter for addresses (receiving).  The filter applies to
	// OnRelevantTxAccepted.
	// TODO: register outpoints (third argument).
	// if len(addresses) > 0 {
	// 	if err = cdrClient.LoadTxFilter(true, addresses, nil); err != nil {
	// 		return newContextualError("load tx filter failed", err)
	// 	}
	// }

	return nil
}

type blockHashHeight struct {
	hash   chainhash.Hash
	height int64
}

type collectionQueue struct {
	sync.Mutex
	q            chan *blockHashHeight
	syncHandlers []func(hash *chainhash.Hash)
}

// NewCollectionQueue creates a new collectionQueue with a queue channel large
// enough for 10 million block pointers.
func NewCollectionQueue() *collectionQueue {
	return &collectionQueue{
		q: make(chan *blockHashHeight, 1e7),
	}
}

func (q *collectionQueue) SetSynchronousHandlers(syncHandlers []func(hash *chainhash.Hash)) {
	q.syncHandlers = syncHandlers
}

// ProcessBlocks receives new *blockHashHeights, calls the synchronous handlers,
// then signals to the monitors that a new block was mined.
func (q *collectionQueue) ProcessBlocks() {
	// process queued blocks one at a time
	for bh := range q.q {
		hash := bh.hash
		height := bh.height

		start := time.Now()

		// Run synchronous block connected handlers in order
		for _, h := range q.syncHandlers {
			h(&hash)
		}

		log.Debugf("Synchronous handlers of collectionQueue.ProcessBlocks() completed in %v", time.Since(start))

		// Signal to mempool monitors that a block was mined
		select {
		case NtfnChans.NewTxChan <- &mempool.NewTx{
			Hash: nil,
			T:    time.Now(),
		}:
		default:
		}

		select {
		case NtfnChans.ExpNewTxChan <- &explorer.NewMempoolTx{
			Hex: "",
		}:
		default:
		}

		// API status update handler
		select {
		case NtfnChans.UpdateStatusNodeHeight <- uint32(height):
		default:
		}
	}
}

// func (q *collectionQueue) PushBlock(b *blockHashHeight) {
// 	q.blockQueue = append(q.blockQueue, b)
// }

// func (q *collectionQueue) PopBlock() *blockHashHeight {
// 	if len(q.blockQueue) == 0 {
// 		return nil
// 	}
// 	b := q.blockQueue[0]
// 	q.blockQueue = q.blockQueue[1:]
// 	return b
// }

// MakeNodeNtfnHandlers defines the cdr notification handlers
func MakeNodeNtfnHandlers() (*rpcclient.NotificationHandlers, *collectionQueue) {
	blockQueue := NewCollectionQueue()
	go blockQueue.ProcessBlocks()
	return &rpcclient.NotificationHandlers{
		OnBlockConnected: func(blockHeaderSerialized []byte, transactions [][]byte) {
			blockHeader := new(wire.BlockHeader)
			err := blockHeader.FromBytes(blockHeaderSerialized)
			if err != nil {
				log.Error("Failed to serialize blockHeader in new block notification.")
			}
			height := int32(blockHeader.Height)
			hash := blockHeader.BlockHash()

			// queue this block
			blockQueue.q <- &blockHashHeight{
				hash:   hash,
				height: int64(height),
			}
		},
		OnReorganization: func(oldHash *chainhash.Hash, oldHeight int32,
			newHash *chainhash.Hash, newHeight int32) {
			// Send reorg data to cdrsqlite's monitor
			select {
			case NtfnChans.ReorgChanWiredDB <- &cdrsqlite.ReorgData{
				OldChainHead:   *oldHash,
				OldChainHeight: oldHeight,
				NewChainHead:   *newHash,
				NewChainHeight: newHeight,
			}:
			default:
			}
			// Send reorg data to blockdata's monitor (so that it stops collecting)
			select {
			case NtfnChans.ReorgChanBlockData <- &blockdata.ReorgData{
				OldChainHead:   *oldHash,
				OldChainHeight: oldHeight,
				NewChainHead:   *newHash,
				NewChainHeight: newHeight,
			}:
			default:
			}
			// Send reorg data to stakedb's monitor
			select {
			case NtfnChans.ReorgChanStakeDB <- &stakedb.ReorgData{
				OldChainHead:   *oldHash,
				OldChainHeight: oldHeight,
				NewChainHead:   *newHash,
				NewChainHeight: newHeight,
			}:
			default:
			}
		},

		OnWinningTickets: func(blockHash *chainhash.Hash, blockHeight int64,
			tickets []*chainhash.Hash) {
			var txstr []string
			for _, t := range tickets {
				txstr = append(txstr, t.String())
			}
			log.Tracef("Winning tickets: %v", strings.Join(txstr, ", "))
		},
		// maturing tickets. Thanks for fixing the tickets type bug, jolan!
		OnNewTickets: func(hash *chainhash.Hash, height int64, stakeDiff int64,
			tickets []*chainhash.Hash) {
			for _, tick := range tickets {
				log.Tracef("Mined new ticket: %v", tick.String())
			}
		},
		// OnRelevantTxAccepted is invoked when a transaction containing a
		// registered address is inserted into mempool.
		OnRelevantTxAccepted: func(transaction []byte) {
			rec, err := udb.NewTxRecord(transaction, time.Now())
			if err != nil {
				return
			}
			tx := cdrutil.NewTx(&rec.MsgTx)
			txHash := rec.Hash
			select {
			case NtfnChans.RelevantTxMempoolChan <- tx:
				log.Debugf("Detected transaction %v in mempool containing registered address.",
					txHash.String())
			default:
			}
		},

		// OnTxAcceptedVerbose is invoked same as OnTxAccepted but is used here
		// for the mempool monitors to avoid an extra call to cdr for
		// the tx details
		OnTxAcceptedVerbose: func(txDetails *cdrjson.TxRawResult) {
			select {
			case NtfnChans.ExpNewTxChan <- &explorer.NewMempoolTx{
				Time: time.Now().Unix(),
				Hex:  txDetails.Hex,
			}:
			default:
				log.Warn("expNewTxChan buffer full!")
			}

			hash, _ := chainhash.NewHashFromStr(txDetails.Txid)
			select {
			case NtfnChans.NewTxChan <- &mempool.NewTx{
				Hash: hash,
				T:    time.Now(),
			}:
			default:
				log.Warn("NewTxChan buffer full!")
			}
		},
	}, blockQueue
}
