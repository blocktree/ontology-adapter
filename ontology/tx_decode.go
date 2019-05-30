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

	"github.com/blocktree/go-owcdrivers/ontologyTransaction"
	"github.com/blocktree/openwallet/log"
	"github.com/blocktree/openwallet/openwallet"
)

type TransactionDecoder struct {
	*openwallet.TransactionDecoderBase
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
					return fmt.Errorf("No unbound ONG to withdraw in address : " + to)
				}

				if a.ONGUnbound.Cmp(fee) <= 0 {
					return fmt.Errorf("Unbound ONG is not enough to withdraw in address : " + to)
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
				return fmt.Errorf("Address : " + to + " not found!")
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
						return fmt.Errorf("The ONG of the account is enough," +
							" but cannot be sent in just one transaction!\n" +
							"the amount can be sent in " + string(len(countList)) +
							"times with amounts :\n" + strings.Replace(strings.Trim(fmt.Sprint(countList), "[]"), " ", ",", -1))

					} else {
						countList = append(countList, a.ONGBalance.Uint64())
					}
					continue
				}
				txState.From = a.Address
				break
			}

			if txState.From == "" {
				return fmt.Errorf("No enough ONG to send!")
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
					return fmt.Errorf("The ONT of the account is enough," +
						" but cannot be sent in just one transaction!\n" +
						"the amount can be sent in " + string(len(countList)) +
						"times with amounts :\n" + strings.Replace(strings.Trim(fmt.Sprint(countList), "[]"), " ", ",", -1))
				} else {
					countList = append(countList, a.ONTBalance.Uint64())
				}
				continue
			}
			txState.From = a.Address
			if a.ONGBalance.Cmp(fee) < 0 {
				return fmt.Errorf("No enough ONG to send ONT on address :" + a.Address)
			}
			break
		}

		if txState.From == "" {
			return fmt.Errorf("No enough ONT to send!")

		}
	} else { // other contract
		return fmt.Errorf("Contract " + rawTx.Coin.Contract.Address + " is not supported yet!")
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
	emptyTrans, transHash, err := ontologyTransaction.CreateRawTransactionAndHash(gasPrice, gasLimit, txState)
	if err != nil {
		return err
	}

	rawTx.RawHex = emptyTrans

	if rawTx.Signatures == nil {
		rawTx.Signatures = make(map[string][]*openwallet.KeySignature)
	}

	keySigs := make([]*openwallet.KeySignature, 0)

	for _, address := range transHash.Addresses {
		addr, err := wrapper.GetAddress(address)
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
	}

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

	//keySignatures := rawTx.Signatures[rawTx.Account.AccountID]

	for accountID, keySignatures := range rawTx.Signatures {

		decoder.wm.Log.Debug("accountID:", accountID)

		if keySignatures != nil {
			for _, keySignature := range keySignatures {

				childKey, err := key.DerivedKeyWithPath(keySignature.Address.HDPath, keySignature.EccType)
				keyBytes, err := childKey.GetPrivateKeyBytes()
				if err != nil {
					return err
				}
				// log.Debug("privateKey:", hex.EncodeToString(keyBytes))

				// //privateKeys = append(privateKeys, keyBytes)

				// log.Debug("hash:", txHash.GetTxHashHex())

				//签名交易
				/////////交易单哈希签名
				sigPub, err := ontologyTransaction.SignRawTransactionHash(keySignature.Message, keyBytes)
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
		rawTx.Signatures[accountID] = keySignatures
	}


	log.Info("transaction hash sign success")



	return nil
}

func (decoder *TransactionDecoder) VerifyONTRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction) error {

	var (
		emptyTrans = rawTx.RawHex
		sigPubs    = make([]ontologyTransaction.SigPub, 0)
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

			sigPubs = append(sigPubs, signaturePubkey)

			log.Debug("Signature:", keySignature.Signature)
			log.Debug("PublicKey:", keySignature.Address.PublicKey)
		}
	}

	pass, signedTrans, err := ontologyTransaction.VerifyAndCombineRawTransaction(emptyTrans, sigPubs)
	if err != nil {
		return fmt.Errorf("transaction compose signatures failed")
	}

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

//CreateSummaryRawTransactionWithError 创建汇总交易，返回能原始交易单数组（包含带错误的原始交易单）
func (decoder *TransactionDecoder) CreateSummaryRawTransactionWithError(wrapper openwallet.WalletDAI, sumRawTx *openwallet.SummaryRawTransaction) ([]*openwallet.RawTransactionWithError, error) {
	raTxWithErr := make([]*openwallet.RawTransactionWithError, 0)
	rawTxs, err := decoder.CreateSummaryRawTransaction(wrapper, sumRawTx)
	if err != nil {
		return nil, err
	}
	for _, tx := range rawTxs {
		raTxWithErr = append(raTxWithErr, &openwallet.RawTransactionWithError{
			RawTx: tx,
			Error: nil,
		})
	}
	return raTxWithErr, nil
}

type feeSupport struct {
	address string
	amount  int64
}

type feeSupports struct {
	fs []feeSupport
}

func (fs feeSupports) getFeeAddress(amount int64) string {
	address := ""
	payed := int64(0)
	for index := 0; index < len(fs.fs); index++ {
		payed += fs.fs[index].amount
		if payed >= amount {
			address = fs.fs[index].address
		}
	}

	return address
}

func (decoder *TransactionDecoder) CreateSummaryRawTransaction(wrapper openwallet.WalletDAI, sumRawTx *openwallet.SummaryRawTransaction) ([]*openwallet.RawTransaction, error) {
	var (
		rawTxArray      = make([]*openwallet.RawTransaction, 0)
		accountID       = sumRawTx.Account.AccountID
		minTransfer     = big.NewInt(0)
		retainedBalance = big.NewInt(0)

		gasPrice = ontologyTransaction.DefaultGasPrice
		gasLimit = ontologyTransaction.DefaultGasLimit

		feesSupportAccount *openwallet.AssetsAccount
		err                error
		// txState  ontologyTransaction.TxState
		// gasPrice = ontologyTransaction.DefaultGasPrice
		// gasLimit = ontologyTransaction.DefaultGasLimit
	)

	// 如果有提供手续费账户，检查账户是否存在
	if feesAcount := sumRawTx.FeesSupportAccount; feesAcount != nil {
		account, supportErr := wrapper.GetAssetsAccountInfo(feesAcount.AccountID)
		if supportErr != nil {
			return nil, openwallet.Errorf(openwallet.ErrAccountNotFound, "can not find fees support account")
		}

		feesSupportAccount = account
	}

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
	if sumRawTx.Coin.Contract.Address == ontologyTransaction.ONGContractAddress {
		minTransfer = big.NewInt(int64(convertFromAmount(sumRawTx.MinTransfer)))
		retainedBalance = big.NewInt(int64(convertFromAmount(sumRawTx.RetainedBalance)))
	} else if sumRawTx.Coin.Contract.Address == ontologyTransaction.ONTContractAddress {
		if sumRawTx.MinTransfer == "" {
			minTransfer = big.NewInt(0)
		} else {
			mt, err := strconv.ParseInt(sumRawTx.MinTransfer, 10, 64)
			if err != nil {
				return nil, errors.New("minTransfer invalid!")
			}
			minTransfer = big.NewInt(mt)
		}

		if sumRawTx.RetainedBalance == "" {
			retainedBalance = big.NewInt(0)
		} else {
			rb, err := strconv.ParseInt(sumRawTx.RetainedBalance, 10, 64)
			if err != nil {
				return nil, errors.New("minTransfer invalid!")
			}
			retainedBalance = big.NewInt(rb)
		}

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

	addrBalanceArray, feeEnough, err := decoder.wm.Blockscanner.GetBalanceByAddressAndContract(fee, sumRawTx.Coin.Contract.Address, searchAddrs...) //GetBalanceByAddress(searchAddrs...)
	if err != nil {
		return nil, err
	}
	extraFee := int64(0)
	for _, enough := range feeEnough {
		if !enough {
			extraFee++
		}
	}

	feeSupports := feeSupports{}

	if extraFee != 0 && feesSupportAccount == nil {
		return nil, fmt.Errorf("[%s] have not enough ONG to pay summary fees", accountID)
	} else if feesSupportAccount != nil {
		feeSupportAddresses, err := wrapper.GetAddressList(0, 2000, "AccountID", feesSupportAccount.AccountID)

		if err != nil {
			return nil, fmt.Errorf("Failed to get fee support account!")
		}

		if len(feeSupportAddresses) == 0 {
			return nil, fmt.Errorf("No addresses found in fee support account!")
		}

		ongContains := big.NewInt(0)
		for _, addr := range feeSupportAddresses {
			balance, err := decoder.wm.RPCClient.getBalance(addr.Address)

			if err != nil {
				return nil, err
			}
			ongContains.Add(ongContains, balance.ONGBalance)
			feeSupports.fs = append(feeSupports.fs, feeSupport{address: addr.Address, amount: balance.ONGBalance.Int64()})
		}

		if ongContains.Int64() < extraFee*int64(gasLimit*gasPrice) {
			return nil, fmt.Errorf("No enough ONG found in fee support account!")
		}
	}
	feeExtra := int64(0)
	for i, addrBalance := range addrBalanceArray {

		if sumRawTx.Coin.Contract.Address == ontologyTransaction.ONGContractAddress {
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
			if feeEnough[i] {
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
					addrBalance,
					"")
				if createErr != nil {
					return nil, createErr
				}

				//创建成功，添加到队列
				rawTxArray = append(rawTxArray, rawTx)
			} else {
				feeExtra += int64(gasLimit * gasPrice)
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
					addrBalance,
					feeSupports.getFeeAddress(feeExtra))
				if createErr != nil {
					return nil, createErr
				}

				//创建成功，添加到队列
				rawTxArray = append(rawTxArray, rawTx)
			}

		} else if sumRawTx.Coin.Contract.Address == ontologyTransaction.ONTContractAddress {
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
			if feeEnough[i] {
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
					addrBalance,
					"")
				if createErr != nil {
					return nil, createErr
				}

				//创建成功，添加到队列
				rawTxArray = append(rawTxArray, rawTx)
			} else {
				feeExtra += int64(gasLimit * gasPrice)

				//创建一笔交易单
				rawTx := &openwallet.RawTransaction{
					Coin:    sumRawTx.Coin,
					Account: feesSupportAccount,
					To: map[string]string{
						sumRawTx.SummaryAddress: sumAmount,
					},
					Required: 1,
				}
				createErr := decoder.createRawTransaction(
					wrapper,
					rawTx,
					addrBalance,
					feeSupports.getFeeAddress(feeExtra))
				if createErr != nil {
					return nil, createErr
				}

				//创建成功，添加到队列
				rawTxArray = append(rawTxArray, rawTx)
			}

		}

	}
	return rawTxArray, nil
}

func (decoder *TransactionDecoder) createRawTransaction(wrapper openwallet.WalletDAI, rawTx *openwallet.RawTransaction, addrBalance *openwallet.Balance, payer string) error {
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
		txState.Payer = payer
		txState.From = addrBalance.Address

	} else if rawTx.Coin.Contract.Address == ontologyTransaction.ONTContractAddress { // ONT transaction
		amount, err := convertIntStringToBigInt(amountStr)
		if err != nil {
			return errors.New("ONT is the smallest unit which cannot be divided,the amount input should never be a float number")
		}
		txState.AssetType = ontologyTransaction.AssetONT
		txState.Amount = amount.Uint64()
		txState.To = to
		txState.Payer = payer
		txState.From = addrBalance.Address

	} else { // other contract
		return fmt.Errorf("Contract " + rawTx.Coin.Contract.Address + " is not supported yet!")
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
	emptyTrans, transHash, err := ontologyTransaction.CreateRawTransactionAndHash(gasPrice, gasLimit, txState)
	if err != nil {
		return err
	}

	rawTx.RawHex = emptyTrans

	signatures := rawTx.Signatures
	if signatures == nil {
		signatures = make(map[string][]*openwallet.KeySignature)
	}

	for _, address := range transHash.Addresses {
		addr, err := wrapper.GetAddress(address)
		if err != nil {
			return err
		}
		signature := &openwallet.KeySignature{
			EccType: decoder.wm.Config.CurveType,
			Nonce:   "",
			Address: addr,
			Message: transHash.GetTxHashHex(),
		}

		keySigs := signatures[addr.AccountID]
		if keySigs == nil {
			keySigs = make([]*openwallet.KeySignature, 0)
		}

		//装配签名
		keySigs = append(keySigs, signature)

		signatures[addr.AccountID] = keySigs
	}

	rawTx.Signatures = signatures

	rawTx.FeeRate = big.NewInt(int64(gasPrice)).String()

	rawTx.IsBuilt = true

	return nil
}

func (decoder *TransactionDecoder) GetRawTransactionFeeRate() (feeRate string, unit string, err error) {
	var (
		gasPrice = decoder.wm.Config.GasPriceFixed
		gasLimit = decoder.wm.Config.GasLimit
	)
	if decoder.wm.Config.GasPriceType == 0 {
		gasPrice = decoder.wm.Config.GasPriceFixed
	} else {
		gasPrice, err = decoder.wm.RPCClient.getGasPrice()
		if err != nil {
			return "", "", err
		}
	}

	return convertToAmount(gasLimit * gasPrice), "TX", nil
}
