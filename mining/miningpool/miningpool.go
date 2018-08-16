package miningpool

import (
	"errors"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/bytom/account"
	"github.com/bytom/mining"
	"github.com/bytom/protocol"
	"github.com/bytom/protocol/bc"
	"github.com/bytom/protocol/bc/types"
)

const (
	blockUpdateMS   = 1000
	maxSubmitChSize = 50
)

// submitMsg contains either a solved blockHeader work or a solved block
// generated according to a blocktemplate and cannot contains both.
// It also checks whether the submitting work/block is valid.
type submitMsg struct {
	work  *types.BlockHeader
	block *types.Block
	reply chan error
}

// MiningPool is the support struct for p2p mine pool
type MiningPool struct {
	mutex        sync.RWMutex
	block        *types.Block
	gbtWorkState *GbtWorkState
	submitCh     chan *submitMsg

	chain          *protocol.Chain
	accountManager *account.Manager
	txPool         *protocol.TxPool
	newBlockCh     chan *bc.Hash
}

// NewMiningPool will create a new MiningPool
func NewMiningPool(c *protocol.Chain, accountManager *account.Manager, txPool *protocol.TxPool, newBlockCh chan *bc.Hash) *MiningPool {
	m := &MiningPool{
		submitCh:       make(chan *submitMsg, maxSubmitChSize),
		chain:          c,
		accountManager: accountManager,
		txPool:         txPool,
		newBlockCh:     newBlockCh,
	}
	go m.blockUpdater()
	return m
}

// blockUpdater is the goroutine for keep update mining block
func (m *MiningPool) blockUpdater() {
	ticker := time.NewTicker(time.Millisecond * blockUpdateMS)
	for {
		select {
		case <-ticker.C:
			m.generateBlock()

		case submitMsg := <-m.submitCh:
			var err error
			if submitMsg.work != nil && submitMsg.block == nil {
				err = m.submitWork(submitMsg.work)
			} else if submitMsg.work == nil && submitMsg.block != nil {
				err = m.submitBlock(submitMsg.block)
			} else {
				err = errors.New("miningpool: submitMsg error")
			}
			if err == nil {
				m.generateBlock()
			}
			submitMsg.reply <- err
		}
	}
}

// generateBlock generates a block template to mine
func (m *MiningPool) generateBlock() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.block != nil && *m.chain.BestBlockHash() == m.block.PreviousBlockHash {
		m.block.Timestamp = uint64(time.Now().Unix())
		return
	}

	block, err := mining.NewBlockToMine(m.chain, m.txPool, m.accountManager)
	if err != nil {
		log.Errorf("miningpool: failed on create new block to mine: %v", err)
		return
	}

	m.block = block
	// TODO
	bh := m.block.BlockHeader
	now := time.Now()
	seed, err := m.chain.CalcNextSeed(&bh.PreviousBlockHash)
	if err != nil {
		log.Errorf("miningpool: failed on calc next seed: %v", err)
		return
	}

	/*
	   {
	     "status": "success",
	     "data": {
	       "hash": "ce4fe9431cd0225b3a811f8f8ec922f2b07a921bb12a8dddae9a85540072c770",
	       "size": 546,
	       "version": 1,
	       "height": 0,
	       "previous_block_hash": "0000000000000000000000000000000000000000000000000000000000000000",
	       "timestamp": 1528945000,
	       "nonce": 9253507043297,
	       "bits": 2305843009214532600,
	       "difficulty": "20",
	       "transaction_merkle_root": "58e45ceb675a0b3d7ad3ab9d4288048789de8194e9766b26d8f42fdb624d4390",
	       "transaction_status_hash": "c9c377e5192668bc0a367e4a4764f11e7c725ecced1d7b6a492974fab1b6d5bc",
	       "transactions": [
	         {
	           "id": "158d7d7c6a8d2464725d508fafca76f0838d998eacaacb42ccc58cfb0c155352",
	           "version": 1,
	           "size": 151,
	           "time_range": 0,
	           "inputs": [
	             {
	               "type": "coinbase",
	               "asset_id": "0000000000000000000000000000000000000000000000000000000000000000",
	               "asset_definition": {},
	               "amount": 0,
	               "arbitrary": "496e666f726d6174696f6e20697320706f7765722e202d2d204a616e2f31312f323031332e20436f6d707574696e6720697320706f7765722e202d2d204170722f32342f323031382e",
	               "input_id": "953b17a15c82cc524e0e25230736a512809dc1a5fe6c0b29747fa2de2e2d64b4"
	             }
	           ],
	           "outputs": [
	             {
	               "type": "control",
	               "id": "e3325bf07c4385af4b60ad6ecc682ee0773f9b96e1cfbbae9f0f12b86b5f1093",
	               "position": 0,
	               "asset_id": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	               "asset_definition": {},
	               "amount": 140700041250000000,
	               "control_program": "00148c9d063ff74ee6d9ffa88d83aeb038068366c4c4",
	               "address": "sm1q3jwsv0lhfmndnlag3kp6avpcq6pkd3xyxg7z8f"
	             }
	           ],
	           "status_fail": false,
	           "mux_id": "d5156f4477fcb694388e6aed7ca390e5bc81bb725ce7461caa241777c1f62236"
	         }
	       ]
	     }
	   }
	*/

	m.gbtWorkState = &GbtWorkState{
		lastTxUpdate:  now,
		lastGenerated: now,
		prevHash:      &bh.PreviousBlockHash,
		template: &mining.BlockTemplate{
			Block: m.block,
			Seed:  *seed,
		},
		// minTimestamp:  time.Time,
	}
}

// GetWork will return a block header for p2p mining
func (m *MiningPool) GetWork() (*types.BlockHeader, error) {
	if m.block != nil {
		m.mutex.RLock()
		defer m.mutex.RUnlock()
		bh := m.block.BlockHeader
		return &bh, nil
	}
	return nil, errors.New("no block is ready for mining")
}

// SubmitWork will try to submit the result to the blockchain
func (m *MiningPool) SubmitWork(bh *types.BlockHeader) error {
	reply := make(chan error, 1)
	m.submitCh <- &submitMsg{work: bh, reply: reply}
	err := <-reply
	if err != nil {
		log.WithFields(log.Fields{"err": err, "height": bh.Height}).Warning("submitWork failed")
	}
	return err
}

// SubmitBlock will try to submit the result to the blockchain
func (m *MiningPool) SubmitBlock(b *types.Block) error {
	reply := make(chan error, 1)
	m.submitCh <- &submitMsg{block: b, reply: reply}
	err := <-reply
	if err != nil {
		log.WithFields(log.Fields{"err": err, "height": b.BlockHeader.Height}).Warning("submitBlock failed")
	}
	return err
}

func (m *MiningPool) submitWork(bh *types.BlockHeader) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.block == nil || bh.PreviousBlockHash != m.block.PreviousBlockHash {
		return errors.New("pending mining block has been changed")
	}

	m.block.Nonce = bh.Nonce
	m.block.Timestamp = bh.Timestamp
	isOrphan, err := m.chain.ProcessBlock(m.block)
	if err != nil {
		return err
	}
	if isOrphan {
		return errors.New("submit result is orphan")
	}

	blockHash := bh.Hash()
	m.newBlockCh <- &blockHash
	return nil
}

func (m *MiningPool) submitBlock(b *types.Block) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// TODO
	if m.block == nil || b.PreviousBlockHash != m.block.PreviousBlockHash {
		return errors.New("pending mining block has been changed")
	}

	isOrphan, err := m.chain.ProcessBlock(b)
	if err != nil {
		return err
	}
	if isOrphan {
		return errors.New("block submitted is orphan")
	}

	blockHash := b.BlockHeader.Hash()
	m.newBlockCh <- &blockHash

	return nil
}

// TODO
// gbtWorkState houses state that is used in between multiple RPC invocations to
// getblocktemplate.
type GbtWorkState struct {
	mutex         sync.RWMutex
	lastTxUpdate  time.Time
	lastGenerated time.Time
	prevHash      *bc.Hash
	minTimestamp  time.Time
	template      *mining.BlockTemplate
	// notifyMap     map[chainhash.Hash]map[int64]chan struct{}
	// timeSource    blockchain.MedianTimeSource
}

func (ws *GbtWorkState) getBlockTemplate() *mining.BlockTemplate {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()

	return ws.template
}

func (m *MiningPool) GetBlockTemplate() *mining.BlockTemplate {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.gbtWorkState != nil {
		return m.gbtWorkState.getBlockTemplate()
	} else {
		return nil
	}
}
