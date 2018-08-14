package main

import (
	"encoding/json"
	"log"
	"os"
	// "time"

	// "github.com/bytom/api"
	// "github.com/bytom/consensus"
	"github.com/bytom/consensus/difficulty"
	"github.com/bytom/mining"
	"github.com/bytom/protocol/bc"
	"github.com/bytom/protocol/bc/types"
	"github.com/bytom/util"
)

const (
	maxNonce = ^uint64(0) // 2^64 - 1
)

func getBlockHeaderByHeight(height uint64) {
	type Req struct {
		BlockHeight uint64 `json:"block_height"`
	}

	type Resp struct {
		BlockHeader *types.BlockHeader `json:"block_header"`
		Reward      uint64             `json:"reward"`
	}

	data, _ := util.ClientCall("/get-block-header", Req{BlockHeight: height})
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

// TODO
func doWork(bh *types.BlockHeader, seed *bc.Hash) bool {
	lastNonce := uint64(0)
	log.Println("Start from nonce:", lastNonce+1)
	for i := uint64(lastNonce + 1); i <= maxNonce; i++ {
		bh.Nonce = i
		log.Printf("nonce = %v\n", i)
		headerHash := bh.Hash()
		if difficulty.CheckProofOfWork(&headerHash, seed, bh.Bits) {
			log.Printf("Mining succeed! Proof hash: %v\n", headerHash.String())
			return true
		}
	}
	log.Println("Stop at nonce:", bh.Nonce)
	return false
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

	// type GbtResp struct {
	// 	Status string `json:"status"`
	// 	Data      mining.BlockTemplate `json:"BlockTemplate"`
	// }
	// bt := &mining.BlockTemplate{}
	bt := &mining.BlockTemplate{}
	if err = json.Unmarshal(rawData, bt); err != nil {
		log.Fatalln(err)
	}
	// log.Println(bt.BlockHeader)
	// log.Println(bt.BlockHeader.Timestamp)

	if doWork(bt.BlockHeader, bt.Seed) {
		// log.Println("bh:", bt.BlockHeader)
		log.Println("Nonce:", bt.BlockHeader.Nonce)
		util.ClientCall("/submit-block", bt)
	}

	// bt.Timestamp = uint64(time.Now().Unix())
	// log.Println(bt.Timestamp)

	// log.Println("Mining at height:", resp.BlockHeader.Height)
	// if lastHeight != resp.BlockHeader.Height {
	// 	lastNonce = ^uint64(0)
	// }
	// if doWork(resp.BlockHeader, resp.Seed) {
	// 	util.ClientCall("/submit-work", &api.SubmitWorkReq{BlockHeader: resp.BlockHeader})
	// 	getBlockHeaderByHeight(resp.BlockHeader.Height)
	// }

	// lastHeight = resp.BlockHeader.Height
	// if !isCrazy {
	// 	return
	// }
}
