package main

import (
	"encoding/json"
	"log"
	"os"

	// "github.com/bytom/api"
	// "github.com/bytom/consensus"
	// "github.com/bytom/consensus/difficulty"
	// "github.com/bytom/protocol/bc"
	"github.com/bytom/protocol/bc/types"
	"github.com/bytom/util"
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

func main() {
	data, _ := util.ClientCall("/get-block-template", struct{}{})
	if data == nil {
		os.Exit(1)
	}
	rawData, err := json.Marshal(data)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(rawData)
	// resp := &api.GetWorkResp{}
	// if err = json.Unmarshal(rawData, resp); err != nil {
	// 	log.Fatalln(err)
	// }

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
