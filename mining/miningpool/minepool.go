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

// MiningPool is the support struct for p2p mining pool
type MiningPool struct {
	mutex          sync.RWMutex
	template       *mining.BlockTemplate
	submitCh       chan *submitMsg
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
			m.generateTemplate()

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
				m.generateTemplate()
			}
			submitMsg.reply <- err
		}
	}
}

// generateTemplate generates a block template to mine
func (m *MiningPool) generateTemplate() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.template != nil && *m.chain.BestBlockHash() == m.template.Block.PreviousBlockHash {
		m.template.Block.Timestamp = uint64(time.Now().Unix())
		return
	}

	template, err := mining.NewBlockTemplate(m.chain, m.txPool, m.accountManager)
	if err != nil {
		log.Errorf("miningpool: failed on create new block template: %v", err)
		return
	}
	m.template = template
}

// GetWork will return a block header for p2p mining
func (m *MiningPool) GetWork() (*types.BlockHeader, error) {
	if m.template != nil {
		m.mutex.RLock()
		defer m.mutex.RUnlock()
		bh := m.template.Block.BlockHeader
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

	if m.template == nil || bh.PreviousBlockHash != m.template.Block.PreviousBlockHash {
		return errors.New("pending mining block has been changed")
	}

	m.template.Block.Nonce = bh.Nonce
	m.template.Block.Timestamp = bh.Timestamp
	isOrphan, err := m.chain.ProcessBlock(m.template.Block)
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

	if m.template.Block == nil || b.PreviousBlockHash != m.template.Block.PreviousBlockHash {
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
