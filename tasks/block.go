package tasks

import (
	"fmt"
	"math/big"
	"squirrel/block"
	"squirrel/buffer"
	"squirrel/db"
	"squirrel/log"
	"squirrel/mail"
	"squirrel/rpc"
	"squirrel/tx"
	"squirrel/util"
	"time"
)

// bufferSize is the capacity of pending blocks waiting to be persisted to db.
const bufferSize = 5000

var (
	// bestRPCHeight util.SafeCounter.
	bProgress    = Progress{}
	blockBuffer  buffer.BlockBuffer
	worker       Worker
	blockChannel chan *rpc.RawBlock
)

func fetchBlock() {
	worker.add()
	log.Printf("Create new worker to fetch blocks\n")

	nextHeight := blockBuffer.GetNextPending()
	waited := 0

	defer func() {
		const hint = "Worker for block data persistence terminated"
		log.Printf("%s. Remaining workers=%d\n", hint, worker.num())
	}()

	defer mail.AlertIfErr()

	for {
		// Control size of the blockBuffer.
		if blockBuffer.Size() > bufferSize {
			time.Sleep(time.Millisecond * 20)
			continue
		}

		// If fully synchronized.
		if worker.num() == 1 && nextHeight == blockBuffer.GetHighest()+1 {
			time.Sleep(time.Second)
			waited++
			log.Printf("Waiting for block index: %d(%s)\n", nextHeight, util.SecondsToHuman(uint64(waited)))
			// if waited >= 30 && waited%10 == 0 {
			// 	rpc.SwitchServer()
			// }
		}

		b := rpc.DownloadBlock(nextHeight)

		// Beyond the latest block.
		if b == nil {
			if worker.shouldQuit() {
				return
			}

			// Get the correct next pending block.
			nextHeight = blockBuffer.GetHighest() + 1
			continue
		}

		waited = 0
		blockBuffer.Put(b)

		if worker.num() == 1 {
			nextHeight = blockBuffer.GetHighest() + 1
		} else {
			nextHeight = blockBuffer.GetNextPending()
		}
	}
}

func arrangeBlock(dbHeight int, queue chan<- *rpc.RawBlock) {
	defer mail.AlertIfErr()

	const sleepTime = 20
	height := dbHeight + 1
	delay := 0

	for {
		if b, ok := blockBuffer.Pop(height); ok {
			queue <- b
			height++
			delay = 0
			continue
		}

		time.Sleep(time.Millisecond * time.Duration(sleepTime))
		if blockBuffer.Size() == 0 {
			continue
		}
		delay += sleepTime

		if delay >= 5000 && delay%1000 == 0 {
			log.Printf("Waited for %d seconds for block height [%d] in [arrangeBlock]\n", delay/1000, height)
		}

		if delay%(1000*60) == 0 {
			err := fmt.Errorf("block height %d is missing while downloading blocks", height)
			log.Println(err)

			getMissingBlock(height)
		}
	}
}

func getMissingBlock(height int) {
	log.Printf("Try fetching given block of height: %d\n", height)

	b := rpc.DownloadBlock(height)
	if b != nil {
		blockBuffer.Put(b)
	}
}

func storeBlock(ch <-chan *rpc.RawBlock) {
	defer mail.AlertIfErr()

	const size = 15
	rawBlocks := []*rpc.RawBlock{}

	for block := range ch {
		rawBlocks = append(rawBlocks, block)
		if block.Index%size == 0 ||
			int(block.Index) == blockBuffer.GetHighest() {
			store(rawBlocks)
			rawBlocks = nil
		}
	}
}

func store(rawBlocks []*rpc.RawBlock) {
	maxIndex := int(rawBlocks[len(rawBlocks)-1].Index)
	blocks := block.ParseBlocks(rawBlocks)
	txBulk := tx.ParseTxs(rawBlocks)

	err := db.InsertBlock(maxIndex, blocks, txBulk)
	if err != nil {
		panic(err)
	}

	// Auxiliary signal for tx task.
	TxMaxPkShouldRefresh = true
	AssetTxMaxPkShouldRefresh = true
	Nep5MaxPkShouldRefresh = true
	gasMaxPkShouldRefresh = true
	scMaxPkShouldRefresh = true

	bestHeight := rpc.BestHeight.Get()

	// Refresh bestRPCHeight if necessary.
	if bestHeight < maxIndex {
		bestHeight = maxIndex
		rpc.BestHeight.Set(maxIndex)
	}

	showBlockStorageProgress(int64(maxIndex), int64(bestHeight))
}

func showBlockStorageProgress(maxIndex int64, highestIndex int64) {
	now := time.Now()

	if bProgress.LastOutputTime == (time.Time{}) {
		bProgress.LastOutputTime = now
	}

	if maxIndex < highestIndex &&
		now.Sub(bProgress.LastOutputTime) < time.Second {
		return
	}

	GetEstimatedRemainingTime(maxIndex, highestIndex, &bProgress)
	if bProgress.Percentage.Cmp(big.NewFloat(100)) == 0 {
		bProgress.Finished = true
	}

	log.Printf("%sBlock storage progress: %d/%d, %.4f%%\n",
		bProgress.RemainingTimeStr,
		maxIndex,
		highestIndex,
		bProgress.Percentage,
	)
	bProgress.LastOutputTime = now

	// Send mail if fully synced.
	if bProgress.Finished && !bProgress.MailSent {
		bProgress.MailSent = true

		// If sync lasts shortly, do not send mail.
		if time.Since(bProgress.InitTime) < time.Minute*5 {
			return
		}

		msg := fmt.Sprintf("Block counts: %d", highestIndex)
		mail.SendNotify("Block data Fully Synced", msg)
	}
}
