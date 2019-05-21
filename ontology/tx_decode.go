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
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blocktree/go-owcdrivers/btcTransaction"
	"github.com/blocktree/go-owcdrivers/ontologyTransaction"
	"github.com/blocktree/openwallet/log"
	"github.com/blocktree/openwallet/openwallet"
)

type TransactionDecoder struct {
	openwallet.TransactionDecoderBase
	wm *WalletManager //钱包管理者
}

//NewTransactionDecoder 交易单解析器
func NewTransactionDecoder(wm *WalletManager) *TransactionDecoder {
	decoder := TransactionDecoder{}
	decoder.wm = wm
	return &decoder
}

//CreateRawTransaction 创建交易单
func (decoder *TransactionDecoder) CreateRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction) error {
	return decoder.CreateONTRawTransaction(wrapper, rawTx)
}

//SignRawTransaction 签名交易单
func (decoder *TransactionDecoder) SignRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction) error {
	return decoder.SignONTRawTransaction(wrapper, rawTx)
}

//VerifyRawTransaction 验证交易单，验证交易单并返回加入签名后的交易单
func (decoder *TransactionDecoder) VerifyRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction) error {
	return decoder.VerifyONTRawTransaction(wrapper, rawTx)
}

func (decoder *TransactionDecoder) SubmitRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction) (*openwallet.Transaction, error) {
	if len(rawTx.RawHex) == 0 {
		return nil, fmt.Errorf("transaction hex is empty")
	}

	if !rawTx.IsCompleted {
		return nil, fmt.Errorf("transaction is not completed validation")
	}

	txid, err := decoder.wm.SendRawTransaction(rawTx.RawHex)
	if err != nil {
		return nil, err
	}

	rawTx.TxID = txid
	rawTx.IsSubmit = true

	decimals := int32(0)

	tx := openwallet.Transaction{
		From:       rawTx.TxFrom,
		To:         rawTx.TxTo,
		Amount:     rawTx.TxAmount,
		Coin:       rawTx.Coin,
		TxID:       rawTx.TxID,
		Decimal:    decimals,
		AccountID:  rawTx.Account.AccountID,
		Fees:       rawTx.Fees,
		SubmitTime: time.Now().Unix(),
	}

	tx.WxID = openwallet.GenTransactionWxID(&tx)

	return &tx, nil
}

func (decoder *TransactionDecoder) CreateONTRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction) error {

	var (
		txState  ontologyTransaction.TxState
		gasPrice = ontologyTransaction.DefaultGasPrice
		gasLimit = ontologyTransaction.DefaultGasLimit
	)

	addresses, err := wrapper.GetAddressList(0, 2000, "AccountID", rawTx.Account.AccountID)

	if err != nil {
		return err
	}

	if len(addresses) == 0 {
		return fmt.Errorf("No addresses found in wallet [%s]", rawTx.Account.AccountID)
	}

	addressesBalanceList := make([]AddrBalance, 0, len(addresses))

	for i, addr := range addresses {
		balance, err := decoder.wm.RPCClient.getBalance(addr.Address)

		if err != nil {
			return err
		}
		balance.index = i
		addressesBalanceList = append(addressesBalanceList, *balance)
	}

	sort.Slice(addressesBalanceList, func(i int, j int) bool {
		return addressesBalanceList[i].ONTBalance.Cmp(addressesBalanceList[j].ONTBalance) >= 0
	})

	if rawTx.FeeRate != "" {
		feeprice, err := convertIntStringToBigInt(rawTx.FeeRate)
		if err != nil {
			return errors.New("fee rate passed through error")
		}
		gasPrice = feeprice.Uint64()
	} else {
		if decoder.wm.Config.GasPriceType == 0 {
			gasPrice = decoder.wm.Config.GasPriceFixed
		} else {
			gasPrice, err = decoder.wm.RPCClient.getGasPrice()
			if err != nil {
				return err
			}
		}
	}

	if decoder.wm.Config.GasLimit != 0 {
		gasLimit = decoder.wm.Config.GasLimit
	}

	fee := big.NewInt(int64(gasLimit * gasPrice))

	var amountStr, to string
	for k, v := range rawTx.To {
		to = k
		amountStr = v
		break
	}
	keySignList := make([]*openwallet.KeySignature, 1, 1)

	if rawTx.Coin.Contract.Address == ontologyTransaction.ONGContractAddress {
		amount, err := convertFlostStringToBigInt(amountStr)
		if err != nil {
			return errors.New("ONG can be divided,with 100000000 smallest unit equls 1 ONG")
		}

		if amount.Cmp(big.NewInt(0)) == 0 { // ONG unbound
			txState.AssetType = ontologyTransaction.AssetONGWithdraw
			txState.From = to
			txState.To = to

			for i, a := range addressesBalanceList {
				if a.Address != to {
					continue
				}
				if a.ONGUnbound.Cmp(big.NewInt(0)) == 0 {
					log.Error("No unbound ONG to withdraw in address : "+to, err)
					return err
				}

				if a.ONGUnbound.Cmp(fee) <= 0 {
					log.Error("Unbound ONG is not enough to withdraw in address : "+to, err)
					return err
				}

				txState.Amount = amount.Sub(a.ONGUnbound, fee).Uint64()
				keySignList = append(keySignList, &openwallet.KeySignature{
					Address: &openwallet.Address{
						AccountID:   addresses[addressesBalanceList[i].index].AccountID,
						Address:     addresses[addressesBalanceList[i].index].Address,
						PublicKey:   addresses[addressesBalanceList[i].index].PublicKey,
						Alias:       addresses[addressesBalanceList[i].index].Alias,
						Tag:         addresses[addressesBalanceList[i].index].Tag,
						Index:       addresses[addressesBalanceList[i].index].Index,
						HDPath:      addresses[addressesBalanceList[i].index].HDPath,
						WatchOnly:   addresses[addressesBalanceList[i].index].WatchOnly,
						Symbol:      addresses[addressesBalanceList[i].index].Symbol,
						Balance:     addresses[addressesBalanceList[i].index].Balance,
						IsMemo:      addresses[addressesBalanceList[i].index].IsMemo,
						Memo:        addresses[addressesBalanceList[i].index].Memo,
						CreatedTime: addresses[addressesBalanceList[i].index].CreatedTime,
					},
				})
				break
			}

			if amount.Cmp(big.NewInt(0)) == 0 {
				log.Error("Address : "+to+" not found!", err)
				return err
			}

		} else { // ONG transaction
			txState.AssetType = ontologyTransaction.AssetONG
			txState.Amount = amount.Uint64()
			txState.To = to
			count := big.NewInt(0)
			countList := []uint64{}
			for _, a := range addressesBalanceList {
				if a.ONGBalance.Cmp(amount) < 0 {
					count.Add(count, a.ONGBalance)
					if count.Cmp(amount) >= 0 {
						countList = append(countList, a.ONGBalance.Sub(a.ONGBalance, count.Sub(count, amount)).Uint64())
						log.Error("The ONG of the account is enough,"+
							" but cannot be sent in just one transaction!\n"+
							"the amount can be sent in "+string(len(countList))+
							"times with amounts :\n"+strings.Replace(strings.Trim(fmt.Sprint(countList), "[]"), " ", ",", -1), err)
						return err
					} else {
						countList = append(countList, a.ONGBalance.Uint64())
					}
					continue
				}
				txState.From = a.Address
				break
			}

			if txState.From == "" {
				log.Error("No enough ONG to send!", err)
				return err
			}
		}
	} else if rawTx.Coin.Contract.Address == ontologyTransaction.ONTContractAddress { // ONT transaction
		amount, err := convertIntStringToBigInt(amountStr)
		if err != nil {
			return errors.New("ONT is the smallest unit which cannot be divided,the amount input should never be a float number")
		}
		txState.AssetType = ontologyTransaction.AssetONT
		txState.Amount = amount.Uint64()
		txState.To = to
		count := big.NewInt(0)
		countList := []uint64{}
		for _, a := range addressesBalanceList {
			if a.ONTBalance.Cmp(amount) < 0 {
				count.Add(count, a.ONTBalance)
				if count.Cmp(amount) >= 0 {
					countList = append(countList, a.ONTBalance.Sub(a.ONTBalance, count.Sub(count, amount)).Uint64())
					log.Error("The ONT of the account is enough,"+
						" but cannot be sent in just one transaction!\n"+
						"the amount can be sent in "+string(len(countList))+
						"times with amounts :\n"+strings.Replace(strings.Trim(fmt.Sprint(countList), "[]"), " ", ",", -1), err)
					return err
				} else {
					countList = append(countList, a.ONTBalance.Uint64())
				}
				continue
			}
			txState.From = a.Address
			if a.ONGBalance.Cmp(fee) < 0 {
				log.Error("No enough ONG to send ONT on address :"+a.Address, err)
				return err
			}
			break
		}

		if txState.From == "" {
			log.Error("No enough ONT to send!", err)
			return err
		}
	} else { // other contract
		log.Error("Contract [%v] is not supported yet!", rawTx.Coin.Contract.Address, err)
		return err
	}

	feeInONG, _ := convertBigIntToFloatDecimal(fee.String())
	rawTx.Fees = feeInONG.String()
	rawTx.TxFrom = []string{txState.From}
	rawTx.TxTo = []string{txState.To}
	if txState.AssetType == ontologyTransaction.AssetONT {
		rawTx.TxAmount = strconv.FormatUint(txState.Amount, 10)
	} else if txState.AssetType == ontologyTransaction.AssetONG {
		ongAmount, _ := convertBigIntToFloatDecimal(strconv.FormatUint(txState.Amount, 10))
		rawTx.TxAmount = ongAmount.String()

	} else {
		// other token
	}
	emptyTrans, err := ontologyTransaction.CreateEmptyRawTransaction(gasPrice, gasLimit, txState)
	if err != nil {
		return err
	}

	transHash, err := ontologyTransaction.CreateRawTransactionHashForSig(emptyTrans)
	if err != nil {
		return err
	}
	rawTx.RawHex = emptyTrans

	if rawTx.Signatures == nil {
		rawTx.Signatures = make(map[string][]*openwallet.KeySignature)
	}

	keySigs := make([]*openwallet.KeySignature, 0)

	addr, err := wrapper.GetAddress(transHash.GetNormalTxAddress())
	if err != nil {
		return err
	}
	signature := openwallet.KeySignature{
		EccType: decoder.wm.Config.CurveType,
		Nonce:   "",
		Address: addr,
		Message: transHash.GetTxHashHex(),
	}

	keySigs = append(keySigs, &signature)

	rawTx.Signatures[rawTx.Account.AccountID] = keySigs

	rawTx.FeeRate = big.NewInt(int64(gasPrice)).String()

	rawTx.IsBuilt = true

	return nil
}

func (decoder *TransactionDecoder) SignONTRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction) error {
	key, err := wrapper.HDKey()
	if err != nil {
		return nil
	}

	keySignatures := rawTx.Signatures[rawTx.Account.AccountID]

	if keySignatures != nil {
		for _, keySignature := range keySignatures {

			childKey, err := key.DerivedKeyWithPath(keySignature.Address.HDPath, keySignature.EccType)
			keyBytes, err := childKey.GetPrivateKeyBytes()
			if err != nil {
				return err
			}
			log.Debug("privateKey:", hex.EncodeToString(keyBytes))

			//privateKeys = append(privateKeys, keyBytes)
			txHash := ontologyTransaction.TxHash{
				Hash: keySignature.Message,
				Normal: &ontologyTransaction.NormalTx{
					Address: keySignature.Address.Address,
					SigType: btcTransaction.SigHashAll,
				},
			}

			log.Debug("hash:", txHash.GetTxHashHex())

			//签名交易
			/////////交易单哈希签名
			sigPub, err := ontologyTransaction.SignRawTransactionHash(txHash.GetTxHashHex(), keyBytes)
			if err != nil {
				return fmt.Errorf("transaction hash sign failed, unexpected error: %v", err)
			} else {

				//for i, s := range sigPub {
				//	log.Info("第", i+1, "个签名结果")
				//	log.Info()
				//	log.Info("对应的公钥为")
				//	log.Info(hex.EncodeToString(s.Pubkey))
				//}

				// txHash.Normal.SigPub = *sigPub
			}

			keySignature.Signature = hex.EncodeToString(sigPub.Signature)
		}
	}

	log.Info("transaction hash sign success")

	rawTx.Signatures[rawTx.Account.AccountID] = keySignatures

	return nil
}

func (decoder *TransactionDecoder) VerifyONTRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction) error {

	var (
		emptyTrans = rawTx.RawHex
		transHash  = make([]ontologyTransaction.TxHash, 0)
	)

	for accountID, keySignatures := range rawTx.Signatures {
		log.Debug("accountID Signatures:", accountID)
		for _, keySignature := range keySignatures {

			signature, _ := hex.DecodeString(keySignature.Signature)
			pubkey, _ := hex.DecodeString(keySignature.Address.PublicKey)

			signaturePubkey := ontologyTransaction.SigPub{
				Signature: signature,
				PublicKey: pubkey,
			}

			//sigPub = append(sigPub, signaturePubkey)

			txHash := ontologyTransaction.TxHash{
				Hash: keySignature.Message,
				Normal: &ontologyTransaction.NormalTx{
					Address: keySignature.Address.Address,
					SigType: btcTransaction.SigHashAll,
					SigPub:  signaturePubkey,
				},
			}

			transHash = append(transHash, txHash)

			log.Debug("Signature:", keySignature.Signature)
			log.Debug("PublicKey:", keySignature.Address.PublicKey)
		}
	}

	signedTrans, err := ontologyTransaction.InsertSignatureIntoEmptyTransaction(emptyTrans, transHash[0].Normal.SigPub)
	if err != nil {
		return fmt.Errorf("transaction compose signatures failed")
	}

	pass := ontologyTransaction.VerifyRawTransaction(signedTrans)

	if pass {
		log.Debug("transaction verify passed")
		rawTx.IsCompleted = true
		rawTx.RawHex = signedTrans
	} else {
		log.Debug("transaction verify failed")
		rawTx.IsCompleted = false
	}

	return nil
}

func (decoder *TransactionDecoder) CreateSummaryRawTransaction(wrapper openwallet.WalletDAI, sumRawTx *openwallet.SummaryRawTransaction) ([]*openwallet.RawTransaction, error) {
	var (
		rawTxArray      = make([]*openwallet.RawTransaction, 0)
		accountID       = sumRawTx.Account.AccountID
		minTransfer     = big.NewInt(0)
		retainedBalance = big.NewInt(0)

		gasPrice = ontologyTransaction.DefaultGasPrice
		gasLimit = ontologyTransaction.DefaultGasLimit
		err      error
		// txState  ontologyTransaction.TxState
		// gasPrice = ontologyTransaction.DefaultGasPrice
		// gasLimit = ontologyTransaction.DefaultGasLimit
	)

	if sumRawTx.FeeRate != "" {
		feeprice, err := convertIntStringToBigInt(sumRawTx.FeeRate)
		if err != nil {
			return nil, errors.New("fee rate passed through error")
		}
		gasPrice = feeprice.Uint64()
	} else {
		if decoder.wm.Config.GasPriceType == 0 {
			gasPrice = decoder.wm.Config.GasPriceFixed
		} else {
			gasPrice, err = decoder.wm.RPCClient.getGasPrice()
			if err != nil {
				return nil, err
			}
		}
	}
	if sumRawTx.Coin.ContractID == ontologyTransaction.ONGContractAddress {
		minTransfer = big.NewInt(int64(convertFromAmount(sumRawTx.MinTransfer)))
		retainedBalance = big.NewInt(int64(convertFromAmount(sumRawTx.RetainedBalance)))
	} else if sumRawTx.Coin.ContractID == ontologyTransaction.ONTContractAddress {
		mt, err := strconv.ParseInt(sumRawTx.MinTransfer, 10, 64)
		if err != nil {
			return nil, errors.New("minTransfer invalid!")
		}
		minTransfer = big.NewInt(mt)
		rb, err := strconv.ParseInt(sumRawTx.RetainedBalance, 10, 64)
		if err != nil {
			return nil, errors.New("minTransfer invalid!")
		}
		retainedBalance = big.NewInt(rb)
	} else {
		//
	}

	fee := big.NewInt(int64(gasLimit * gasPrice))

	if minTransfer.Cmp(retainedBalance) < 0 {
		return nil, fmt.Errorf("mini transfer amount must be greater than address retained balance")
	}

	//获取wallet
	addresses, err := wrapper.GetAddressList(sumRawTx.AddressStartIndex, sumRawTx.AddressLimit,
		"AccountID", sumRawTx.Account.AccountID)
	if err != nil {
		return nil, err
	}

	if len(addresses) == 0 {
		return nil, fmt.Errorf("[%s] have not addresses", accountID)
	}

	searchAddrs := make([]string, 0)
	for _, address := range addresses {
		searchAddrs = append(searchAddrs, address.Address)
	}

	addrBalanceArray, err := decoder.wm.Blockscanner.GetBalanceByAddressAndContract(fee, sumRawTx.Coin.ContractID, searchAddrs...) //GetBalanceByAddress(searchAddrs...)
	if err != nil {
		return nil, err
	}

	for _, addrBalance := range addrBalanceArray {

		if sumRawTx.Coin.ContractID == ontologyTransaction.ONGContractAddress {
			//
			abbi, _ := strconv.ParseInt(addrBalance.Balance, 10, 64)
			addrBalance_BI := big.NewInt(abbi)

			if addrBalance_BI.Cmp(big.NewInt(0)) == 0 || addrBalance_BI.Cmp(minTransfer) < 0 {
				continue
			}
			//计算汇总数量 = 余额 - 保留余额
			sumAmount_BI := new(big.Int)
			sumAmount_BI.Sub(addrBalance_BI, retainedBalance)

			sumAmount_BI.Sub(sumAmount_BI, fee)

			if sumAmount_BI.Cmp(big.NewInt(0)) <= 0 {
				continue
			}
			sumAmount := convertToAmount(sumAmount_BI.Uint64())
			fees := convertToAmount(fee.Uint64())

			//log.Debugf("balance: %v", addrBalance.Balance)
			log.Debugf("fees: %v", fees)
			log.Debugf("sumAmount: %v", sumAmount)

			//创建一笔交易单
			rawTx := &openwallet.RawTransaction{
				Coin:    sumRawTx.Coin,
				Account: sumRawTx.Account,
				To: map[string]string{
					sumRawTx.SummaryAddress: sumAmount,
				},
				Required: 1,
			}
			createErr := decoder.createRawTransaction(
				wrapper,
				rawTx,
				addrBalance)
			if createErr != nil {
				return nil, createErr
			}

			//创建成功，添加到队列
			rawTxArray = append(rawTxArray, rawTx)
		} else if sumRawTx.Coin.ContractID == ontologyTransaction.ONTContractAddress {
			//检查余额是否超过最低转账
			abbi, _ := strconv.ParseInt(addrBalance.Balance, 10, 64)
			addrBalance_BI := big.NewInt(abbi)

			if addrBalance_BI.Cmp(minTransfer) < 0 {
				continue
			}
			//计算汇总数量 = 余额 - 保留余额
			sumAmount_BI := new(big.Int)
			sumAmount_BI.Sub(addrBalance_BI, retainedBalance)

			sumAmount := sumAmount_BI.String()
			fees := convertToAmount(fee.Uint64())

			log.Debugf("balance: %v", addrBalance.Balance)
			log.Debugf("fees: %v", fees)
			log.Debugf("sumAmount: %v", sumAmount)

			//创建一笔交易单
			rawTx := &openwallet.RawTransaction{
				Coin:    sumRawTx.Coin,
				Account: sumRawTx.Account,
				To: map[string]string{
					sumRawTx.SummaryAddress: sumAmount,
				},
				Required: 1,
			}

			createErr := decoder.createRawTransaction(
				wrapper,
				rawTx,
				addrBalance)
			if createErr != nil {
				return nil, createErr
			}

			//创建成功，添加到队列
			rawTxArray = append(rawTxArray, rawTx)
		}

	}
	return rawTxArray, nil
}

func (decoder *TransactionDecoder) createRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction, addrBalance *openwallet.Balance) error {
	var (
		txState  ontologyTransaction.TxState
		gasPrice = ontologyTransaction.DefaultGasPrice
		gasLimit = ontologyTransaction.DefaultGasLimit
		err      error
	)

	if rawTx.FeeRate != "" {
		feeprice, err := convertIntStringToBigInt(rawTx.FeeRate)
		if err != nil {
			return errors.New("fee rate passed through error")
		}
		gasPrice = feeprice.Uint64()
	} else {
		if decoder.wm.Config.GasPriceType == 0 {
			gasPrice = decoder.wm.Config.GasPriceFixed
		} else {
			gasPrice, err = decoder.wm.RPCClient.getGasPrice()
			if err != nil {
				return err
			}
		}
	}

	if decoder.wm.Config.GasLimit != 0 {
		gasLimit = decoder.wm.Config.GasLimit
	}

	fee := big.NewInt(int64(gasLimit * gasPrice))

	var amountStr, to string
	for k, v := range rawTx.To {
		to = k
		amountStr = v
		break
	}

	if rawTx.Coin.Contract.Address == ontologyTransaction.ONGContractAddress {
		amount, err := convertFlostStringToBigInt(amountStr)
		if err != nil {
			return errors.New("ONG can be divided,with 100000000 smallest unit equls 1 ONG")
		}

		txState.AssetType = ontologyTransaction.AssetONG
		txState.Amount = amount.Uint64()
		txState.To = to

		txState.From = addrBalance.Address

	} else if rawTx.Coin.Contract.Address == ontologyTransaction.ONTContractAddress { // ONT transaction
		amount, err := convertIntStringToBigInt(amountStr)
		if err != nil {
			return errors.New("ONT is the smallest unit which cannot be divided,the amount input should never be a float number")
		}
		txState.AssetType = ontologyTransaction.AssetONT
		txState.Amount = amount.Uint64()
		txState.To = to

		txState.From = addrBalance.Address

	} else { // other contract
		log.Error("Contract [%v] is not supported yet!", rawTx.Coin.Contract.Address, err)
		return err
	}

	feeInONG, _ := convertBigIntToFloatDecimal(fee.String())
	rawTx.Fees = feeInONG.String()
	rawTx.TxFrom = []string{txState.From}
	rawTx.TxTo = []string{txState.To}
	if txState.AssetType == ontologyTransaction.AssetONT {
		rawTx.TxAmount = strconv.FormatUint(txState.Amount, 10)
	} else if txState.AssetType == ontologyTransaction.AssetONG {
		ongAmount, _ := convertBigIntToFloatDecimal(strconv.FormatUint(txState.Amount, 10))
		rawTx.TxAmount = ongAmount.String()

	} else {
		// other token
	}
	emptyTrans, err := ontologyTransaction.CreateEmptyRawTransaction(gasPrice, gasLimit, txState)
	if err != nil {
		return err
	}

	transHash, err := ontologyTransaction.CreateRawTransactionHashForSig(emptyTrans)
	if err != nil {
		return err
	}
	rawTx.RawHex = emptyTrans

	if rawTx.Signatures == nil {
		rawTx.Signatures = make(map[string][]*openwallet.KeySignature)
	}

	keySigs := make([]*openwallet.KeySignature, 0)

	addr, err := wrapper.GetAddress(transHash.GetNormalTxAddress())
	if err != nil {
		return err
	}
	signature := openwallet.KeySignature{
		EccType: decoder.wm.Config.CurveType,
		Nonce:   "",
		Address: addr,
		Message: transHash.GetTxHashHex(),
	}

	keySigs = append(keySigs, &signature)

	rawTx.Signatures[rawTx.Account.AccountID] = keySigs

	rawTx.FeeRate = big.NewInt(int64(gasPrice)).String()

	rawTx.IsBuilt = true

	return nil
}
