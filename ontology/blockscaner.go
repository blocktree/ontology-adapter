/*
 * Copyright 2018 The openwallet Authors
 * This file is part of the openwallet library.
 *
 * The openwallet library is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The openwallet library is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Lesser General Public License for more details.
 */

package ontology

import (
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/blocktree/go-owcdrivers/ontologyTransaction"
	"github.com/blocktree/openwallet/v2/common"
	"github.com/blocktree/openwallet/v2/log"
	"github.com/blocktree/openwallet/v2/openwallet"
	"github.com/graarh/golang-socketio"
	"github.com/graarh/golang-socketio/transport"
	"github.com/shopspring/decimal"
)

const (
	maxExtractingSize    = 20 //并发的扫描线程数
	RPCServerRest        = 0  //RPC服务，Restful 测试 API
	RPCServerMainnetNode = 1  // RPC服务，主网节点 API
)

//ONTBlockScanner ontology的区块链扫描器
type ONTBlockScanner struct {
	*openwallet.BlockScannerBase

	CurrentBlockHeight   uint64             //当前区块高度
	extractingCH         chan struct{}      //扫描工作令牌
	wm                   *WalletManager     //钱包管理者
	IsScanMemPool        bool               //是否扫描交易池
	RescanLastBlockCount uint64             //重扫上N个区块数量
	socketIO             *gosocketio.Client //socketIO客户端
	RPCServer            int
}

//ExtractResult 扫描完成的提取结果
type ExtractResult struct {
	extractData map[string]*openwallet.TxExtractData
	TxID        string
	BlockHeight uint64
	Success     bool
}

//SaveResult 保存结果
type SaveResult struct {
	TxID        string
	BlockHeight uint64
	Success     bool
}

//NewONTBlockScanner 创建区块链扫描器
func NewONTBlockScanner(wm *WalletManager) *ONTBlockScanner {
	bs := ONTBlockScanner{
		BlockScannerBase: openwallet.NewBlockScannerBase(),
	}

	bs.extractingCH = make(chan struct{}, maxExtractingSize)
	bs.wm = wm
	bs.IsScanMemPool = false
	bs.RescanLastBlockCount = 0
	bs.RPCServer = RPCServerRest

	//设置扫描任务
	bs.SetTask(bs.ScanBlockTask)

	return &bs
}

//SetRescanBlockHeight 重置区块链扫描高度
func (bs *ONTBlockScanner) SetRescanBlockHeight(height uint64) error {
	height = height - 1
	if height < 0 {
		return errors.New("block height to rescan must greater than 0.")
	}

	hash, err := bs.wm.GetBlockHash(height)
	if err != nil {
		return err
	}

	bs.wm.Blockscanner.SaveLocalNewBlock(height, hash)

	return nil
}

//ScanBlockTask 扫描任务
func (bs *ONTBlockScanner) ScanBlockTask() {

	//获取本地区块高度
	blockHeader, err := bs.GetScannedBlockHeader()
	if err != nil {
		log.Std.Info("block scanner can not get new block height; unexpected error: %v", err)
	}

	currentHeight := blockHeader.Height
	currentHash := blockHeader.Hash

	for {

		if !bs.Scanning {
			//区块扫描器已暂停，马上结束本次任务
			return
		}

		//获取最大高度
		maxHeight, err := bs.wm.GetBlockHeight()
		maxHeight--
		if err != nil {
			//下一个高度找不到会报异常
			log.Std.Info("block scanner can not get rpc-server block height; unexpected error: %v", err)
			break
		}

		//是否已到最新高度
		if currentHeight >= maxHeight {
			log.Std.Info("block scanner has scanned full chain data. Current height: %d", maxHeight)
			break
		}

		//继续扫描下一个区块
		currentHeight = currentHeight + 1

		log.Std.Info("block scanner scanning height: %d ...", currentHeight)

		hash, err := bs.wm.GetBlockHash(currentHeight)
		if err != nil {
			//下一个高度找不到会报异常
			log.Std.Info("block scanner can not get new block hash; unexpected error: %v", err)
			break
		}

		block, err := bs.wm.GetBlock(hash)
		if err != nil {
			log.Std.Info("block scanner can not get new block data; unexpected error: %v", err)

			//记录未扫区块
			unscanRecord := openwallet.NewUnscanRecord(currentHeight, "", err.Error(), bs.wm.Symbol())
			bs.SaveUnscanRecord(unscanRecord)
			log.Std.Info("block height: %d extract failed.", currentHeight)
			continue
		}

		isFork := false

		//判断hash是否上一区块的hash
		if currentHash != block.PrevBlockHash {

			log.Std.Info("block has been fork on height: %d.", currentHeight)
			log.Std.Info("block height: %d local hash = %s ", currentHeight-1, currentHash)
			log.Std.Info("block height: %d mainnet hash = %s ", currentHeight-1, block.PrevBlockHash)

			log.Std.Info("delete recharge records on block height: %d.", currentHeight-1)

			//查询本地分叉的区块
			forkBlock, _ := bs.GetLocalBlock(uint32(currentHeight - 1))

			//删除上一区块链的所有充值记录
			//bs.DeleteRechargesByHeight(currentHeight - 1)
			//删除上一区块链的未扫记录
			bs.wm.Blockscanner.DeleteUnscanRecord(uint32(currentHeight - 1))
			currentHeight = currentHeight - 2 //倒退2个区块重新扫描
			if currentHeight <= 0 {
				currentHeight = 1
			}

			localBlock, err := bs.GetLocalBlock(uint32(currentHeight))
			if err != nil {
				log.Std.Error("block scanner can not get local block; unexpected error: %v", err)

				//查找core钱包的RPC
				log.Info("block scanner prev block height:", currentHeight)

				prevHash, err := bs.wm.GetBlockHash(currentHeight)
				if err != nil {
					log.Std.Error("block scanner can not get prev block; unexpected error: %v", err)
					break
				}

				localBlock, err = bs.wm.GetBlock(prevHash)
				if err != nil {
					log.Std.Error("block scanner can not get prev block; unexpected error: %v", err)
					break
				}

			}

			//重置当前区块的hash
			currentHash = localBlock.Hash

			log.Std.Info("rescan block on height: %d, hash: %s .", currentHeight, currentHash)

			//重新记录一个新扫描起点
			bs.wm.Blockscanner.SaveLocalNewBlock(localBlock.Height, localBlock.Hash)

			isFork = true

			if forkBlock != nil {

				//通知分叉区块给观测者，异步处理
				bs.newBlockNotify(forkBlock, isFork)
			}

		} else {

			err = bs.BatchExtractTransaction(block.Height, block.Hash, block.Transactions)
			if err != nil {
				log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", err)
			}

			//重置当前区块的hash
			currentHash = hash

			//保存本地新高度
			bs.wm.Blockscanner.SaveLocalNewBlock(currentHeight, currentHash)
			bs.SaveLocalBlock(block)

			isFork = false

			//通知新区块给观测者，异步处理
			bs.newBlockNotify(block, isFork)
		}

	}

	//重扫前N个块，为保证记录找到
	for i := currentHeight - bs.RescanLastBlockCount; i < currentHeight; i++ {
		bs.scanBlock(i)
	}

	if bs.IsScanMemPool {
		//扫描交易内存池
		bs.ScanTxMemPool()
	}

	//重扫失败区块
	bs.RescanFailedRecord()

}

//ScanBlock 扫描指定高度区块
func (bs *ONTBlockScanner) ScanBlock(height uint64) error {

	block, err := bs.scanBlock(height)
	if err != nil {
		return err
	}
	bs.newBlockNotify(block, false)
	return nil

}
func (bs *ONTBlockScanner) scanBlock(height uint64) (*Block, error) {
	hash, err := bs.wm.GetBlockHash(height)
	if err != nil {
		//下一个高度找不到会报异常
		log.Std.Info("block scanner can not get new block hash; unexpected error: %v", err)
		return nil, err
	}

	block, err := bs.wm.GetBlock(hash)
	if err != nil {
		log.Std.Info("block scanner can not get new block data; unexpected error: %v", err)

		//记录未扫区块
		unscanRecord := openwallet.NewUnscanRecord(height, "", err.Error(), bs.wm.Symbol())
		bs.SaveUnscanRecord(unscanRecord)
		log.Std.Info("block height: %d extract failed.", height)
		return nil, err
	}
	log.Std.Info("block scanner scanning height: %d ...", block.Height)

	err = bs.BatchExtractTransaction(block.Height, block.Hash, block.Transactions)

	if err != nil {
		log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", err)
	}

	return block, nil
}

//ScanTxMemPool 扫描交易内存池
func (bs *ONTBlockScanner) ScanTxMemPool() {

	log.Std.Info("block scanner scanning mempool ...")

	//提取未确认的交易单
	txIDsInMemPool, err := bs.wm.GetTxIDsInMemPool()
	if err != nil {
		log.Std.Info("block scanner can not get mempool data; unexpected error: %v", err)
		return
	}

	err = bs.BatchExtractTransaction(0, "", txIDsInMemPool)
	if err != nil {
		log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", err)
	}

}

//rescanFailedRecord 重扫失败记录
func (bs *ONTBlockScanner) RescanFailedRecord() {

	var (
		blockMap = make(map[uint64][]string)
	)

	list, err := bs.GetUnscanRecords()
	if err != nil {
		log.Std.Info("block scanner can not get rescan data; unexpected error: %v", err)
	}

	//组合成批处理
	for _, r := range list {

		if _, exist := blockMap[r.BlockHeight]; !exist {
			blockMap[r.BlockHeight] = make([]string, 0)
		}

		if len(r.TxID) > 0 {
			arr := blockMap[r.BlockHeight]
			arr = append(arr, r.TxID)

			blockMap[r.BlockHeight] = arr
		}
	}

	for height, txs := range blockMap {

		var hash string

		log.Std.Info("block scanner rescanning height: %d ...", height)

		if len(txs) == 0 {

			hash, err := bs.wm.GetBlockHash(height)
			if err != nil {
				//下一个高度找不到会报异常
				log.Std.Info("block scanner can not get new block hash; unexpected error: %v", err)
				continue
			}

			block, err := bs.wm.GetBlock(hash)
			if err != nil {
				log.Std.Info("block scanner can not get new block data; unexpected error: %v", err)
				continue
			}

			txs = block.Transactions
		}

		err = bs.BatchExtractTransaction(height, hash, txs)
		if err != nil {
			log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", err)
			continue
		}

		//删除未扫记录
		bs.wm.Blockscanner.DeleteUnscanRecord(uint32(height))
	}

	//删除未没有找到交易记录的重扫记录
	bs.wm.Blockscanner.DeleteUnscanRecordNotFindTX()
}

//newBlockNotify 获得新区块后，通知给观测者
func (bs *ONTBlockScanner) newBlockNotify(block *Block, isFork bool) {
	header := block.BlockHeader()
	header.Fork = isFork
	bs.NewBlockNotify(header)
}

//BatchExtractTransaction 批量提取交易单
//bitcoin 1M的区块链可以容纳3000笔交易，批量多线程处理，速度更快
func (bs *ONTBlockScanner) BatchExtractTransaction(blockHeight uint64, blockHash string, txs []string) error {

	var (
		quit       = make(chan struct{})
		done       = 0 //完成标记
		failed     = 0
		shouldDone = len(txs) //需要完成的总数
	)

	if len(txs) == 0 {
		return nil
	}

	//生产通道
	producer := make(chan ExtractResult)
	defer close(producer)

	//消费通道
	worker := make(chan ExtractResult)
	defer close(worker)

	//保存工作
	saveWork := func(height uint64, result chan ExtractResult) {
		//回收创建的地址
		for gets := range result {

			if gets.Success {

				notifyErr := bs.newExtractDataNotify(height, gets.extractData)
				//saveErr := bs.SaveRechargeToWalletDB(height, gets.Recharges)
				if notifyErr != nil {
					failed++ //标记保存失败数
					log.Std.Info("newExtractDataNotify unexpected error: %v", notifyErr)
				}
			} else {
				//记录未扫区块
				unscanRecord := openwallet.NewUnscanRecord(height, "", "", bs.wm.Symbol())
				bs.SaveUnscanRecord(unscanRecord)
				log.Std.Info("block height: %d extract failed.", height)
				failed++ //标记保存失败数
			}
			//累计完成的线程数
			done++
			if done == shouldDone {
				//log.Std.Info("done = %d, shouldDone = %d ", done, len(txs))
				close(quit) //关闭通道，等于给通道传入nil
			}
		}
	}

	//提取工作
	extractWork := func(eblockHeight uint64, eBlockHash string, mTxs []string, eProducer chan ExtractResult) {
		for _, txid := range mTxs {
			bs.extractingCH <- struct{}{}
			//shouldDone++
			go func(mBlockHeight uint64, mTxid string, end chan struct{}, mProducer chan<- ExtractResult) {

				//导出提出的交易
				mProducer <- bs.ExtractTransaction(mBlockHeight, eBlockHash, mTxid, bs.ScanTargetFunc)
				//释放
				<-end

			}(eblockHeight, txid, bs.extractingCH, eProducer)
		}
	}

	/*	开启导出的线程	*/

	//独立线程运行消费
	go saveWork(blockHeight, worker)

	//独立线程运行生产
	go extractWork(blockHeight, blockHash, txs, producer)

	//以下使用生产消费模式
	bs.extractRuntime(producer, worker, quit)

	if failed > 0 {
		return fmt.Errorf("block scanner saveWork failed")
	} else {
		return nil
	}

	//return nil
}

//extractRuntime 提取运行时
func (bs *ONTBlockScanner) extractRuntime(producer chan ExtractResult, worker chan ExtractResult, quit chan struct{}) {

	var (
		values = make([]ExtractResult, 0)
	)

	for {

		var activeWorker chan<- ExtractResult
		var activeValue ExtractResult

		//当数据队列有数据时，释放顶部，传输给消费者
		if len(values) > 0 {
			activeWorker = worker
			activeValue = values[0]

		}

		select {

		//生成者不断生成数据，插入到数据队列尾部
		case pa := <-producer:
			values = append(values, pa)
		case <-quit:
			//退出
			//log.Std.Info("block scanner have been scanned!")
			return
		case activeWorker <- activeValue:
			//wm.Log.Std.Info("Get %d", len(activeValue))
			values = values[1:]
		}
	}

	//return

}

//ExtractTransaction 提取交易单
func (bs *ONTBlockScanner) ExtractTransaction(blockHeight uint64, blockHash string, txid string, scanAddressFunc openwallet.BlockScanTargetFunc) ExtractResult {

	var (
		result = ExtractResult{
			BlockHeight: blockHeight,
			TxID:        txid,
			extractData: make(map[string]*openwallet.TxExtractData),
		}
	)

	//log.Std.Debug("block scanner scanning tx: %s ...", txid)
	trx, err := bs.wm.GetTransaction(txid)

	if err != nil {
		log.Std.Info("block scanner can not extract transaction data; unexpected error: %v", err)
		result.Success = false
		return result
	}

	//优先使用传入的高度
	if blockHeight > 0 && trx.BlockHeight == 0 {
		trx.BlockHeight = blockHeight
		trx.BlockHash = blockHash
	}
	bs.extractTransaction(trx, &result, bs.ScanTargetFuncV2)
	return result

}

// 从最小单位的 amount 转为带小数点的表示
func convertToAmount(amount uint64) string {
	amountStr := fmt.Sprintf("%d", amount)
	d, _ := decimal.NewFromString(amountStr)
	w, _ := decimal.NewFromString("1000000000")
	d = d.Div(w)
	return d.String()
}

// amount 字符串转为最小单位的表示
func convertFromAmount(amountStr string) uint64 {
	d, _ := decimal.NewFromString(amountStr)
	w, _ := decimal.NewFromString("1000000000")
	d = d.Mul(w)
	r, _ := strconv.ParseInt(d.String(), 10, 64)
	return uint64(r)
}

//ExtractTransactionData 提取交易单
func (bs *ONTBlockScanner) extractTransaction(trx *Transaction, result *ExtractResult, scanAddressFunc openwallet.BlockScanTargetFuncV2) {
	var (
		success = true
	)
	createAt := time.Now().Unix()
	if trx == nil {
		//记录哪个区块哪个交易单没有完成扫描
		success = false
	} else {

		if success && len(trx.Notifys) != 0 {

			for _, notify := range trx.Notifys {
				if notify.Method != "transfer" {
					continue
				}
				targetResult := scanAddressFunc(openwallet.ScanTargetParam {
					ScanTarget:     notify.From,
					Symbol:         bs.wm.Symbol(),
					ScanTargetType: openwallet.ScanTargetTypeAccountAddress,
				})


				if targetResult.Exist {
					input := openwallet.TxInput{}
					input.TxID = trx.TxID
					input.Address = notify.From
					input.Symbol = bs.wm.Symbol()
					input.Amount = notify.Amount
					if notify.IsFee {
						input.TxType = 1
					}

					if notify.ContractAddress == ontologyTransaction.ONTContractAddress {
						input.Coin = openwallet.Coin{
							Symbol:     bs.wm.Symbol(),
							IsContract: true,
							ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONTContractAddress),
							Contract: openwallet.SmartContract{
								ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONTContractAddress),
								Symbol:     bs.wm.Symbol(),
								Address:    ontologyTransaction.ONTContractAddress,
								Token:      "ONT",
								Name:       bs.wm.FullName(),
								Decimals:   0,
							},
						}
					} else if notify.ContractAddress == ontologyTransaction.ONGContractAddress {
						input.Coin = openwallet.Coin{
							Symbol:     bs.wm.Symbol(),
							IsContract: true,
							ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONGContractAddress),
							Contract: openwallet.SmartContract{
								ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONGContractAddress),
								Symbol:     bs.wm.Symbol(),
								Address:    ontologyTransaction.ONGContractAddress,
								Token:      "ONG",
								Name:       bs.wm.FullName(),
								Decimals:   9,
							},
						}
					} else {
						//
					}
					input.Index = 0
					input.Sid = openwallet.GenTxInputSID(trx.TxID, input.Coin.Symbol, input.Coin.Contract.Address, 0)
					input.CreateAt = createAt
					input.BlockHeight = trx.BlockHeight
					input.BlockHash = trx.BlockHash

					ed := result.extractData[targetResult.SourceKey]
					if ed == nil {
						ed = openwallet.NewBlockExtractData()
						result.extractData[targetResult.SourceKey] = ed
					}

					ed.TxInputs = append(ed.TxInputs, &input)

					targetResult1 := scanAddressFunc(openwallet.ScanTargetParam {
						ScanTarget:     notify.To,
						Symbol:         bs.wm.Symbol(),
						ScanTargetType: openwallet.ScanTargetTypeAccountAddress,
					})


					if targetResult.Exist && targetResult.SourceKey == targetResult1.SourceKey{

						output := openwallet.TxOutPut{}
						output.Received = true
						output.TxID = trx.TxID
						output.Address = notify.To
						output.Symbol = bs.wm.Symbol()
						output.Amount = notify.Amount
						if notify.ContractAddress == ontologyTransaction.ONTContractAddress {
							output.Coin = openwallet.Coin{
								Symbol:     bs.wm.Symbol(),
								IsContract: true,
								ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONTContractAddress),
								Contract: openwallet.SmartContract{
									ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONTContractAddress),
									Symbol:     bs.wm.Symbol(),
									Address:    ontologyTransaction.ONTContractAddress,
									Token:      "ONT",
									Name:       bs.wm.FullName(),
									Decimals:   0,
								},
							}
						} else if notify.ContractAddress == ontologyTransaction.ONGContractAddress {
							output.Coin = openwallet.Coin{
								Symbol:     bs.wm.Symbol(),
								IsContract: true,
								ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONGContractAddress),
								Contract: openwallet.SmartContract{
									ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONGContractAddress),
									Symbol:     bs.wm.Symbol(),
									Address:    ontologyTransaction.ONGContractAddress,
									Token:      "ONG",
									Name:       bs.wm.FullName(),
									Decimals:   9,
								},
							}
						} else {
							//
						}
						output.Index = 0
						output.Sid = openwallet.GenTxOutPutSID(trx.TxID, output.Coin.Symbol, output.Coin.Contract.Address, 0)
						output.CreateAt = createAt
						output.BlockHeight = trx.BlockHeight
						output.BlockHash = trx.BlockHash

						ed := result.extractData[targetResult.SourceKey]

						if ed == nil {
							ed = openwallet.NewBlockExtractData()
							result.extractData[targetResult.SourceKey] = ed
						}

						ed.TxOutputs = append(ed.TxOutputs, &output)
						continue
					} else {
						output := openwallet.TxOutPut{}
						output.Address = notify.To
						ed.TxOutputs = append(ed.TxOutputs, &output)
					}

				}

				targetResult = scanAddressFunc(openwallet.ScanTargetParam {
					ScanTarget:     notify.To,
					Symbol:         bs.wm.Symbol(),
					ScanTargetType: openwallet.ScanTargetTypeAccountAddress,
				})


				if targetResult.Exist {
					output := openwallet.TxOutPut{}
					output.Received = true
					output.TxID = trx.TxID
					output.Address = notify.To
					output.Symbol = bs.wm.Symbol()
					output.Amount = notify.Amount
					if notify.ContractAddress == ontologyTransaction.ONTContractAddress {
						output.Coin = openwallet.Coin{
							Symbol:     bs.wm.Symbol(),
							IsContract: true,
							ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONTContractAddress),
							Contract: openwallet.SmartContract{
								ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONTContractAddress),
								Symbol:     bs.wm.Symbol(),
								Address:    ontologyTransaction.ONTContractAddress,
								Token:      "ONT",
								Name:       bs.wm.FullName(),
								Decimals:   0,
							},
						}
					} else if notify.ContractAddress == ontologyTransaction.ONGContractAddress {
						output.Coin = openwallet.Coin{
							Symbol:     bs.wm.Symbol(),
							IsContract: true,
							ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONGContractAddress),
							Contract: openwallet.SmartContract{
								ContractID: openwallet.GenContractID(bs.wm.Symbol(), ontologyTransaction.ONGContractAddress),
								Symbol:     bs.wm.Symbol(),
								Address:    ontologyTransaction.ONGContractAddress,
								Token:      "ONG",
								Name:       bs.wm.FullName(),
								Decimals:   9,
							},
						}
					} else {
						//
					}
					output.Index = 0
					output.Sid = openwallet.GenTxOutPutSID(trx.TxID, output.Coin.Symbol, output.Coin.Contract.Address, 0)
					output.CreateAt = createAt
					output.BlockHeight = trx.BlockHeight
					output.BlockHash = trx.BlockHash

					ed := result.extractData[targetResult.SourceKey]

					if ed == nil {
						ed = openwallet.NewBlockExtractData()
						result.extractData[targetResult.SourceKey] = ed
					}

					ed.TxOutputs = append(ed.TxOutputs, &output)
					input := openwallet.TxInput{}
					input.Address = notify.From
					ed.TxInputs = append(ed.TxInputs, &input)
				}
			}

		}

		success = true

	}
	result.Success = success
}

func modifyExtractData(data *openwallet.TxExtractData) []*openwallet.TxExtractData {
	var eds []*openwallet.TxExtractData
	for index := 0; index < len(data.TxInputs); index++ {
		ed := &openwallet.TxExtractData{}
		if data.TxInputs[index].Symbol == "" {
			ed.TxOutputs = append(ed.TxOutputs, data.TxOutputs[index])
			tx := &openwallet.Transaction{
				From:        []string{data.TxInputs[index].Address + ":" + data.TxOutputs[index].Amount},
				To:          []string{data.TxOutputs[index].Address + ":" + data.TxOutputs[index].Amount},
				Amount:      data.TxOutputs[index].Amount,
				Fees:        "0",
				Coin:        data.TxOutputs[index].Coin,
				BlockHash:   data.TxOutputs[index].BlockHash,
				BlockHeight: data.TxOutputs[index].BlockHeight,
				TxID:        data.TxOutputs[index].TxID,
				Decimal:     0,
				Status:      "1",
			}
			tx.WxID = openwallet.GenTransactionWxID(tx)
			ed.Transaction = tx
		} else if data.TxOutputs[index].Symbol == "" {
			ed.TxInputs = append(ed.TxInputs, data.TxInputs[index])
			tx := &openwallet.Transaction{
				From:        []string{data.TxInputs[index].Address + ":" + data.TxInputs[index].Amount},
				To:          []string{data.TxOutputs[index].Address + ":" + data.TxInputs[index].Amount},
				Amount:      data.TxInputs[index].Amount,
				Fees:        "0",
				Coin:        data.TxInputs[index].Coin,
				BlockHash:   data.TxInputs[index].BlockHash,
				BlockHeight: data.TxInputs[index].BlockHeight,
				TxID:        data.TxInputs[index].TxID,
				Decimal:     0,
				Status:      "1",
				TxType:      data.TxInputs[index].TxType,
			}
			tx.WxID = openwallet.GenTransactionWxID(tx)
			ed.Transaction = tx
		} else {
			ed.TxInputs = append(ed.TxInputs, data.TxInputs[index])
			ed.TxOutputs = append(ed.TxOutputs, data.TxOutputs[index])
			tx := &openwallet.Transaction{
				From:        []string{data.TxInputs[index].Address + ":" + data.TxInputs[index].Amount},
				To:          []string{data.TxOutputs[index].Address + ":" + data.TxInputs[index].Amount},
				Amount:      "0",
				Fees:        "0",
				Coin:        data.TxInputs[index].Coin,
				BlockHash:   data.TxInputs[index].BlockHash,
				BlockHeight: data.TxInputs[index].BlockHeight,
				TxID:        data.TxInputs[index].TxID,
				Decimal:     0,
				Status:      "1",
				TxType:      data.TxInputs[index].TxType,
			}
			tx.WxID = openwallet.GenTransactionWxID(tx)
			ed.Transaction = tx
		}

		eds = append(eds, ed)
	}
	return eds
}

//newExtractDataNotify 发送通知
func (bs *ONTBlockScanner) newExtractDataNotify(height uint64, extractData map[string]*openwallet.TxExtractData) error {

	for o, _ := range bs.Observers {
		for key, data := range extractData {
			eds := modifyExtractData(data)
			for _, ed := range eds {
				err := o.BlockExtractDataNotify(key, ed)
				if err != nil {
					log.Error("BlockExtractDataNotify unexpected error:", err)
					//记录未扫区块
					unscanRecord := openwallet.NewUnscanRecord(height, "", "ExtractData Notify failed.", bs.wm.Symbol())
					err = bs.SaveUnscanRecord(unscanRecord)
					if err != nil {
						log.Std.Error("block height: %d, save unscan record failed. unexpected error: %v", height, err.Error())
					}

				}
			}

		}
	}

	return nil
}

//DeleteUnscanRecordNotFindTX 删除未没有找到交易记录的重扫记录
func (bs *ONTBlockScanner) DeleteUnscanRecordNotFindTX() error {

	//删除找不到交易单
	reason := "[-5]No information available about transaction"

	if bs.BlockchainDAI == nil {
		return fmt.Errorf("Blockchain DAI is not setup ")
	}

	list, err := bs.BlockchainDAI.GetUnscanRecords(bs.wm.Symbol())
	if err != nil {
		return err
	}

	for _, r := range list {
		if strings.HasPrefix(r.Reason, reason) {
			bs.BlockchainDAI.DeleteUnscanRecordByID(r.ID, bs.wm.Symbol())
		}
	}
	return nil
}

//GetScannedBlockHeader 获取当前已扫区块高度
func (bs *ONTBlockScanner) GetScannedBlockHeader() (*openwallet.BlockHeader, error) {

	var (
		blockHeight uint64 = 0
		hash        string
		err         error
	)

	blockHeight, hash, err = bs.wm.Blockscanner.GetLocalNewBlock()
	if err != nil {
		return nil, err
	}
	//如果本地没有记录，查询接口的高度
	if blockHeight == 0 {
		blockHeight, err = bs.wm.GetBlockHeight()
		if err != nil {

			return nil, err
		}

		//就上一个区块链为当前区块
		blockHeight = blockHeight - 2

		hash, err = bs.wm.GetBlockHash(blockHeight)
		if err != nil {
			return nil, err
		}
	}

	currentBlock, err := bs.wm.GetBlock(hash)
	if err != nil {
		return nil, err
	}

	return currentBlock.BlockHeader(), nil
}

//GetCurrentBlockHeader 获取当前区块高度
func (bs *ONTBlockScanner) GetCurrentBlockHeader() (*openwallet.BlockHeader, error) {

	var (
		blockHeight uint64 = 0
		hash        string
		err         error
	)

	blockHeight, err = bs.wm.GetBlockHeight()
	if err != nil {

		return nil, err
	}

	hash, err = bs.wm.GetBlockHash(blockHeight - 1)
	if err != nil {
		return nil, err
	}
	currentBlock, err := bs.wm.GetBlock(hash)
	if err != nil {
		return nil, err
	}

	return currentBlock.BlockHeader(), nil
}

//GetScannedBlockHeight 获取已扫区块高度
func (bs *ONTBlockScanner) GetScannedBlockHeight() uint64 {
	localHeight, _, _ := bs.wm.Blockscanner.GetLocalNewBlock()
	return localHeight
}

func (bs *ONTBlockScanner) ExtractTransactionData(txid string, scanTargetFunc openwallet.BlockScanTargetFunc) (map[string][]*openwallet.TxExtractData, error) {
	// result := bs.ExtractTransaction(0, "", txid, scanAddressFunc)
	// if !result.Success {
	// 	return nil, fmt.Errorf("extract transaction failed")
	// }
	scanAddressFunc := func(t openwallet.ScanTarget) (string, bool) {
		sourceKey, ok := scanTargetFunc(openwallet.ScanTarget{
			Address:          t.Address,
			Symbol:           bs.wm.Symbol(),
			BalanceModelType: bs.wm.BalanceModelType(),
		})
		return sourceKey, ok
	}
	result := bs.ExtractTransaction(0, "", txid, scanAddressFunc)
	if !result.Success {
		return nil, fmt.Errorf("extract transaction failed")
	}
	extData := make(map[string][]*openwallet.TxExtractData)
	for key, data := range result.extractData {
		txs := extData[key]
		if txs == nil {
			txs = make([]*openwallet.TxExtractData, 0)
		}
		txs = append(txs, data)
		extData[key] = txs
	}
	return extData, nil
}

//GetSourceKeyByAddress 获取地址对应的数据源标识
func (bs *ONTBlockScanner) GetSourceKeyByAddress(address string) (string, bool) {
	bs.Mu.RLock()
	defer bs.Mu.RUnlock()

	sourceKey, ok := bs.AddressInScanning[address]
	return sourceKey, ok
}

//GetBlockHeight 获取区块链高度
func (wm *WalletManager) GetBlockHeight() (uint64, error) {
	return wm.RPCClient.getBlockHeight()
}

//GetLocalNewBlock 获取本地记录的区块高度和hash
func (bs *ONTBlockScanner) GetLocalNewBlock() (uint64, string, error) {

	if bs.BlockchainDAI == nil {
		return 0, "", fmt.Errorf("Blockchain DAI is not setup ")
	}

	header, err := bs.BlockchainDAI.GetCurrentBlockHead(bs.wm.Symbol())
	if err != nil {
		return 0, "", err
	}

	return header.Height, header.Hash, nil
}

//SaveLocalNewBlock 记录区块高度和hash到本地
func (bs *ONTBlockScanner) SaveLocalNewBlock(blockHeight uint64, blockHash string) error {

	if bs.BlockchainDAI == nil {
		return fmt.Errorf("Blockchain DAI is not setup ")
	}

	header := &openwallet.BlockHeader{
		Hash:   blockHash,
		Height: blockHeight,
		Fork:   false,
		Symbol: bs.wm.Symbol(),
	}

	return bs.BlockchainDAI.SaveCurrentBlockHead(header)
}

//GetBlockHash 根据区块高度获得区块hash
func (wm *WalletManager) GetBlockHash(height uint64) (string, error) {
	return wm.RPCClient.getBlockHash(height)
}

//GetBlock 获取区块数据
func (wm *WalletManager) GetBlock(hash string) (*Block, error) {
	return wm.RPCClient.getBlock(hash)
}

//GetTxIDsInMemPool 获取待处理的交易池中的交易单IDs
func (wm *WalletManager) GetTxIDsInMemPool() ([]string, error) {

	return nil, nil

}

//GetTransaction 获取交易单
func (wm *WalletManager) GetTransaction(txid string) (*Transaction, error) {
	return wm.RPCClient.getTransaction(txid)
}

//GetAssetsAccountBalanceByAddress 查询账户相关地址的交易记录
func (bs *ONTBlockScanner) GetBalanceByAddress(address ...string) ([]*openwallet.Balance, error) {

	addrsBalance := make([]*openwallet.Balance, 0)

	for _, addr := range address {
		balance, err := bs.wm.RPCClient.getONTBalance(addr)
		if err != nil {
			return nil, err
		}

		addrsBalance = append(addrsBalance, &openwallet.Balance{
			Symbol:  bs.wm.Symbol(),
			Address: addr,
			Balance: balance.ONTBalance.String(),
		})
	}

	return nil, nil

}

func (bs *ONTBlockScanner) GetBalanceByAddressAndContract(fee *big.Int, contractAddress string, address ...string) ([]*openwallet.Balance, []bool, error) {

	addrsBalance := make([]*openwallet.Balance, 0)
	feeEnough := make([]bool, 0)
	for _, addr := range address {
		balance, err := bs.wm.RPCClient.getBalance(addr)
		if err != nil {
			return nil, nil, err
		}

		balanceStr := ""
		symbol := ""
		if contractAddress == ontologyTransaction.ONTContractAddress {
			balanceStr = balance.ONTBalance.String()
			if balanceStr == "0" {
				continue
			}
			symbol = "ONT"
			if balance.ONGBalance.Cmp(fee) < 0 {
				feeEnough = append(feeEnough, false)
			} else {
				feeEnough = append(feeEnough, true)
			}
		} else if contractAddress == ontologyTransaction.ONGContractAddress {
			balanceStr = balance.ONGBalance.String()
			if balanceStr == "0" {
				continue
			}
			symbol = "ONG"
			if balance.ONGBalance.Cmp(fee) < 0 {
				feeEnough = append(feeEnough, false)
			} else {
				feeEnough = append(feeEnough, true)
			}
		} else {
			//
		}

		addrsBalance = append(addrsBalance, &openwallet.Balance{
			Symbol:  symbol,
			Address: addr,
			Balance: balanceStr,
		})
	}

	return addrsBalance, feeEnough, nil

}

//GetAssetsAccountTransactionsByAddress 查询账户相关地址的交易记录
func (bs *ONTBlockScanner) GetTransactionsByAddress(offset, limit int, coin openwallet.Coin, address ...string) ([]*openwallet.TxExtractData, error) {

	// var (
	// 	array = make([]*openwallet.TxExtractData, 0)
	// )

	// trxs, err := bs.wm.getMultiAddrTransactionsByExplorer(offset, limit, address...)
	// if err != nil {
	// 	return nil, err
	// }

	// key := "account"

	// //提取账户相关的交易单
	// var scanAddressFunc openwallet.BlockScanAddressFunc = func(findAddr string) (string, bool) {
	// 	for _, a := range address {
	// 		if findAddr == a {
	// 			return key, true
	// 		}
	// 	}
	// 	return "", false
	// }

	// //要检查一下tx.BlockHeight是否有值

	// for _, tx := range trxs {

	// 	result := ExtractResult{
	// 		BlockHeight: tx.BlockHeight,
	// 		TxID:        tx.TxID,
	// 		extractData: make(map[string]*openwallet.TxExtractData),
	// 	}

	// 	bs.extractTransaction(tx, &result, scanAddressFunc)
	// 	data := result.extractData
	// 	txExtract := data[key]
	// 	if txExtract != nil {
	// 		array = append(array, txExtract)
	// 	}
	// }

	// return array, nil
	return nil, nil
}

//Run 运行
func (bs *ONTBlockScanner) Run() error {

	bs.BlockScannerBase.Run()

	return nil
}

////Stop 停止扫描
func (bs *ONTBlockScanner) Stop() error {

	bs.BlockScannerBase.Stop()

	return nil
}

//Pause 暂停扫描
func (bs *ONTBlockScanner) Pause() error {

	bs.BlockScannerBase.Pause()

	return nil
}

//Restart 继续扫描
func (bs *ONTBlockScanner) Restart() error {

	bs.BlockScannerBase.Restart()

	return nil
}

/******************* 使用insight socket.io 监听区块 *******************/

//setupSocketIO 配置socketIO监听新区块
func (bs *ONTBlockScanner) setupSocketIO() error {

	log.Info("block scanner use socketIO to listen new data")

	var (
		room = "inv"
	)

	if bs.socketIO == nil {

		apiUrl, err := url.Parse(bs.wm.Config.ServerAPI)
		if err != nil {
			return err
		}
		domain := apiUrl.Hostname()
		port := common.NewString(apiUrl.Port()).Int()
		c, err := gosocketio.Dial(
			gosocketio.GetUrl(domain, port, false),
			transport.GetDefaultWebsocketTransport())
		if err != nil {
			return err
		}

		bs.socketIO = c

	}

	err := bs.socketIO.On("tx", func(h *gosocketio.Channel, args interface{}) {
		//log.Info("block scanner socketIO get new transaction received: ", args)
		txMap, ok := args.(map[string]interface{})
		if ok {
			txid := txMap["txid"].(string)
			errInner := bs.BatchExtractTransaction(0, "", []string{txid})
			if errInner != nil {
				log.Std.Info("block scanner can not extractRechargeRecords; unexpected error: %v", errInner)
			}
		}

	})
	if err != nil {
		return err
	}

	/*
		err = bs.socketIO.On("block", func(h *gosocketio.Channel, args interface{}) {
			log.Info("block scanner socketIO get new block received: ", args)
			hash, ok := args.(string)
			if ok {

				block, errInner := bs.wm.GetBlock(hash)
				if errInner != nil {
					log.Std.Info("block scanner can not get new block data; unexpected error: %v", errInner)
				}

				errInner = bs.scanBlock(block)
				if errInner != nil {
					log.Std.Info("block scanner can not block: %d; unexpected error: %v", block.Height, errInner)
				}
			}

		})
		if err != nil {
			return err
		}
	*/

	err = bs.socketIO.On(gosocketio.OnDisconnection, func(h *gosocketio.Channel) {
		log.Info("block scanner socketIO disconnected")
	})
	if err != nil {
		return err
	}

	err = bs.socketIO.On(gosocketio.OnConnection, func(h *gosocketio.Channel) {
		log.Info("block scanner socketIO connected")
		h.Emit("subscribe", room)
	})
	if err != nil {
		return err
	}

	return nil
}

//SupportBlockchainDAI 支持外部设置区块链数据访问接口
//@optional
func (bs *ONTBlockScanner) SupportBlockchainDAI() bool {
	return true
}
