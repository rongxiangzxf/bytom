package miningpool

import (
	// "errors"
	"sync"
	"time"

	// log "github.com/sirupsen/logrus"

	// "github.com/bytom/account"
	"github.com/bytom/mining"
	// "github.com/bytom/protocol"
	"github.com/bytom/protocol/bc"
	// "github.com/bytom/protocol/bc/types"
)

// TODO
// gbtWorkState houses state that is used in between multiple RPC invocations to
// getblocktemplate.
type gbtWorkState struct {
	sync.Mutex
	lastTxUpdate  time.Time
	lastGenerated time.Time
	prevHash      *bc.Hash
	minTimestamp  time.Time
	template      *mining.BlockTemplate
	// notifyMap     map[chainhash.Hash]map[int64]chan struct{}
	// timeSource    blockchain.MedianTimeSource
}
