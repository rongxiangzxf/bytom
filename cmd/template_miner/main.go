package main

import (
	"encoding/json"
	"log"
	"os"
	// "time"

	"github.com/bytom/api"
	// "github.com/bytom/consensus"
	"github.com/bytom/consensus/difficulty"
	// "github.com/bytom/mining"
	"github.com/bytom/protocol/bc"
	"github.com/bytom/protocol/bc/types"
	"github.com/bytom/protocol/state"
	"github.com/bytom/util"
)

const (
	maxNonce   = ^uint64(0) // 2^64 - 1
	miningAddr = ""
)

func checkReward(bh string) {
	type Req struct {
		BlockHash string `json:"block_hash"`
	}

	type Resp struct {
		BlockHeader *types.BlockHeader `json:"block_header"`
		Reward      uint64             `json:"reward"`
	}

	data, _ := util.ClientCall("/get-block-header", Req{BlockHash: bh})
	rawData, err := json.Marshal(data)
	if err != nil {
		log.Fatalln(err)
	}

	resp := &Resp{}
	if err = json.Unmarshal(rawData, resp); err != nil {
		log.Fatalln(err)
	}
	log.Println("Reward:", resp.Reward)
}

func doWork(bh *types.BlockHeader, seed *bc.Hash) bool {
	for i := uint64(0); i <= maxNonce; i++ {
		bh.Nonce = i
		log.Printf("nonce = %v\n", i)
		headerHash := bh.Hash()
		if difficulty.CheckProofOfWork(&headerHash, seed, bh.Bits) {
			log.Printf("Mining succeed! Proof hash: %v\n", headerHash.String())
			return true
		}
	}
	return false
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

func constructBlock(gbtResp *api.GbtResp) *types.Block {
	view := state.NewUtxoViewpoint()
	txStatus := bc.NewTransactionStatus()
	txStatus.SetStatus(0, false)
	txEntries := []*bc.Tx{nil}
	gasUsed := uint64(0)
	txFee := uint64(0)

	block := &types.Block{
		BlockHeader: types.BlockHeader{
			Version:           gbtResp.Version,
			Height:            gbtResp.Height,
			PreviousBlockHash: gbtResp.PreBlkHash,
			Timestamp:         gbtResp.CurTime,
			Bits:              gbtResp.Bits,
			BlockCommitment:   types.BlockCommitment{},
		},
	}

	bcBlock := &bc.Block{BlockHeader: &bc.BlockHeader{Height: nextBlockHeight}}
	b.Transactions = []*types.Tx{nil}

	txs := gbtResp.Transactions
	sort.Sort(byTime(txs))

	// TODO
	/*
	   for _, txDesc := range txs {
	   		tx := txDesc.Tx.Tx
	   		gasOnlyTx := false

	   		if err := c.GetTransactionsUtxo(view, []*bc.Tx{tx}); err != nil {
	   			log.WithField("error", err).Error("mining block generate skip tx due to")
	   			txPool.RemoveTransaction(&tx.ID)
	   			continue
	   		}

	   		gasStatus, err := validation.ValidateTx(tx, bcBlock)
	   		if err != nil {
	   			if !gasStatus.GasValid {
	   				log.WithField("error", err).Error("mining block generate skip tx due to")
	   				txPool.RemoveTransaction(&tx.ID)
	   				continue
	   			}
	   			gasOnlyTx = true
	   		}

	   		if gasUsed+uint64(gasStatus.GasUsed) > consensus.MaxBlockGas {
	   			break
	   		}

	   		if err := view.ApplyTransaction(bcBlock, tx, gasOnlyTx); err != nil {
	   			log.WithField("error", err).Error("mining block generate skip tx due to")
	   			txPool.RemoveTransaction(&tx.ID)
	   			continue
	   		}

	   		txStatus.SetStatus(len(b.Transactions), gasOnlyTx)
	   		b.Transactions = append(b.Transactions, txDesc.Tx)
	   		txEntries = append(txEntries, tx)
	   		gasUsed += uint64(gasStatus.GasUsed)
	   		txFee += txDesc.Fee

	   		if gasUsed == consensus.MaxBlockGas {
	   			break
	   		}
	   	}

	   	if gbtResp.CoinbaseTxn != nil {
	   		block.Transactions[0] = gbtResp.CoinbaseTxn
	   	} else {
	   		// creater coinbase transaction
	   		b.Transactions[0], err = createCoinbaseTx(accountManager, txFee, nextBlockHeight)
	   		if err != nil {
	   			return nil, errors.Wrap(err, "fail on createCoinbaseTx")
	   		}
	   	}
	*/
	txEntries[0] = b.Transactions[0].Tx

	block.BlockHeader.BlockCommitment.TransactionsMerkleRoot, err = bc.TxMerkleRoot(txEntries)
	if err != nil {
		panic(err)
	}
	block.BlockHeader.BlockCommitment.TransactionStatusHash, err = bc.TxStatusMerkleRoot(txStatus.VerifyStatus)
	if err != nil {
		panic(err)
	}

	return block
}

func tweakBlock(b *types.Block, gbtResp *api.GbtResp) {
}

func main() {
	data, _ := util.ClientCall("/get-block-template", &struct{}{})
	if data == nil {
		os.Exit(1)
	}
	rawData, err := json.Marshal(data)
	if err != nil {
		log.Fatalln(err)
	}

	gbtResp := &api.GbtResp{}
	if err = json.Unmarshal(rawData, gbtResp); err != nil {
		log.Fatalln(err)
	}

	b := constructBlock(gbtResp)
	tweakBlock(b, gbtResp)

	log.Println("Mining at height:", b.BlockHeader.Height)
	if doWork(&b.BlockHeader, &gbtResp.Seed) {
		util.ClientCall("/submit-block", b)

		headerHash := b.BlockHeader.Hash()
		checkReward(headerHash.String())
	}
}
