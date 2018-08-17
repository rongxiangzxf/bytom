package miningpool

import (
	"errors"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/bytom/account"
	"github.com/bytom/consensus"
	chainjson "github.com/bytom/encoding/json"
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
	mutex        sync.RWMutex
	block        *types.Block
	GbtWorkState *GbtWorkState
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

// blockUpdater is the goroutine to keep updating mining block
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

	template, err := mining.NewBlockTemplate(m.chain, m.txPool, m.accountManager)
	if err != nil {
		log.Errorf("miningpool: failed on create new block template: %v", err)
		return
	}

	// TODO: what time?
	now := time.Now()
	m.GbtWorkState = &GbtWorkState{
		lastTxUpdate:  now,
		lastGenerated: now,
		prevHash:      &template.Block.BlockHeader.PreviousBlockHash,
		template:      template,
	}
	m.block = template.Block
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
	sync.RWMutex
	lastTxUpdate  time.Time
	lastGenerated time.Time
	prevHash      *bc.Hash
	minTimestamp  time.Time
	template      *mining.BlockTemplate
	// notifyMap     map[chainhash.Hash]map[int64]chan struct{}
	// timeSource    blockchain.MedianTimeSource
}

// TODO
// This function MUST be called with the state locked.
func (state *GbtWorkState) UpdateBlockTemplate(useCoinbaseValue bool) error {
	return nil
}

type GbtResult struct {
	Bits          uint64             `json:"bits"`
	CurTime       uint64             `json:"curtime"`
	Height        uint64             `json:"height"`
	PreBlkHash    *bc.Hash           `json:"previousblockhash"`
	MaxBlockGas   uint64             `json:"max_block_gas"`
	Transactions  []*types.Tx        `json:"transactions"`
	Version       uint64             `json:"version"`
	CoinbaseAux   chainjson.HexBytes `json:"coinbaseaux,omitempty"`
	CoinbaseTxn   *types.Tx          `json:"coinbasetxn,omitempty"`   // TODO
	CoinbaseValue uint64             `json:"coinbasevalue,omitempty"` // TODO
	Seed          *bc.Hash           `json:"seed"`
	WorkID        uint64             `json:"workid,omitempty"` // TODO
}

// This function MUST be called with the state locked.
func (state *GbtWorkState) BlockTemplateResult(useCoinbaseValue bool, submitOld *bool) (*GbtResult, error) {
	template := state.template
	if template == nil {
		return nil, errors.New("block template not ready yet.")
	}

	// TODO
	// msgBlock := template.Block
	// header := &msgBlock.Header
	// adjustedTime := state.timeSource.AdjustedTime()
	// maxTime := adjustedTime.Add(time.Second * blockchain.MaxTimeOffsetSeconds)
	// if header.Timestamp.After(maxTime) {
	// 	return nil, &btcjson.RPCError{
	// 		Code: btcjson.ErrRPCOutOfRange,
	// 		Message: fmt.Sprintf("The template time is after the "+
	// 			"maximum allowed time for a block - template "+
	// 			"time %v, maximum time %v", adjustedTime,
	// 			maxTime),
	// 	}
	// }

	gbtResult := &GbtResult{
		Bits:         template.Block.BlockHeader.Bits,
		CurTime:      template.Block.BlockHeader.Timestamp,
		Height:       template.Block.BlockHeader.Height,
		PreBlkHash:   &template.Block.BlockHeader.PreviousBlockHash,
		MaxBlockGas:  consensus.MaxBlockGas,
		Transactions: template.Block.Transactions[1:], // TODO: descp for RPC result
		Version:      template.Block.BlockHeader.Version,
		CoinbaseAux:  []byte{},
		// CoinbaseTxn   *types.Tx          `json:"coinbasetxn,omitempty"`   // TODO
		// CoinbaseValue uint64             `json:"coinbasevalue,omitempty"` // TODO
		Seed: template.Seed,
		// TODO:
		// WorkID
		// WeightLimit:  blockchain.MaxBlockWeight,
		// SigOpLimit:   blockchain.MaxBlockSigOpsCost,
		// SizeLimit:    wire.MaxBlockPayload,
		// LongPollID:   templateID,
		// SubmitOld:    submitOld,
		// Target:       targetDifficulty,
		// MinTime:      state.minTimestamp.Unix(),
		// MaxTime:      maxTime.Unix(),
		// Mutable:      gbtMutableFields,
		// NonceRange:   gbtNonceRange,
		// Capabilities: gbtCapabilities,
	}

	// TODO
	// If the generated block template includes transactions with witness
	// data, then include the witness commitment in the GBT result.
	// if template.WitnessCommitment != nil {
	// 	reply.DefaultWitnessCommitment = hex.EncodeToString(template.WitnessCommitment)
	// }

	if useCoinbaseValue {
		// TODO
		// gbtResult.CoinbaseAux = gbtCoinbaseAux
		// reply.CoinbaseValue = &msgBlock.Transactions[0].TxOut[0].Value
		gbtResult.CoinbaseValue = template.Block.Transactions[0].TxData.Outputs[0].OutputCommitment.AssetAmount.Amount
	} else {
		// Ensure the template has a valid payment address associated
		// with it when a full coinbase is requested.
		if !template.ValidPayAddress {
			return nil, errors.New("A coinbase transaction has been requested, " +
				"but the server has not been configured with any payment addresses.")
		}

		// TODO
		// Serialize the transaction for conversion to hex.
		// tx := msgBlock.Transactions[0]
		// txBuf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		// if err := tx.Serialize(txBuf); err != nil {
		// 	context := "Failed to serialize transaction"
		// 	return nil, internalRPCError(err.Error(), context)
		// }

		// resultTx := btcjson.GetBlockTemplateResultTx{
		// 	Data:    hex.EncodeToString(txBuf.Bytes()),
		// 	Hash:    tx.TxHash().String(),
		// 	Depends: []int64{},
		// 	Fee:     template.Fees[0],
		// 	SigOps:  template.SigOpCosts[0],
		// }

		gbtResult.CoinbaseTxn = template.Block.Transactions[0]
	}

	return gbtResult, nil
}
