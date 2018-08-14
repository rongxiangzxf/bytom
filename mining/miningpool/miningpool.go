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

type submitBlockMsg struct {
	blockHeader *types.BlockHeader
	reply       chan error
}

// MiningPool is the support struct for p2p mine pool
type MiningPool struct {
	mutex        sync.RWMutex
	block        *types.Block
	gbtWorkState *GbtWorkState
	submitCh     chan *submitBlockMsg

	chain          *protocol.Chain
	accountManager *account.Manager
	txPool         *protocol.TxPool
	newBlockCh     chan *bc.Hash
}

// NewMiningPool will create a new MiningPool
func NewMiningPool(c *protocol.Chain, accountManager *account.Manager, txPool *protocol.TxPool, newBlockCh chan *bc.Hash) *MiningPool {
	m := &MiningPool{
		submitCh:       make(chan *submitBlockMsg, maxSubmitChSize),
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
			err := m.submitWork(submitMsg.blockHeader)
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

	block, err := mining.NewBlockTemplate(m.chain, m.txPool, m.accountManager)
	if err != nil {
		log.Errorf("miningpool: failed on create NewBlockTemplate: %v", err)
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
	m.gbtWorkState = &GbtWorkState{
		lastTxUpdate:  now,
		lastGenerated: now,
		prevHash:      &bh.PreviousBlockHash,
		template: &mining.BlockTemplate{
			BlockHeader: &bh,
			Seed:        seed,
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
	m.submitCh <- &submitBlockMsg{blockHeader: bh, reply: reply}
	err := <-reply
	if err != nil {
		log.WithFields(log.Fields{"err": err, "height": bh.Height}).Warning("submitWork failed")
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

func (m *MiningPool) SubmitBlock(bt mining.BlockTemplate) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	b := &types.Block{
		BlockHeader:  *bt.BlockHeader,
		Transactions: bt.Transactions,
	}

	log.Info(b.PreviousBlockHash)
	log.Info(b.BlockHeader.PreviousBlockHash)
	log.Info(b.Timestamp)
	log.Info(b.BlockHeader.Timestamp)
	log.Info(b.Nonce)
	log.Info(b.BlockHeader.Nonce)
	log.Info(bt.Seed)

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
	m.generateBlock()
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
