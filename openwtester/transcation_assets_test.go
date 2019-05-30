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

package openwtester

import (
	"testing"

	"github.com/blocktree/openwallet/log"
	"github.com/blocktree/openwallet/openw"
	"github.com/blocktree/openwallet/openwallet"
)

func testGetAssetsAccountBalance(tm *openw.WalletManager, walletID, accountID string) {
	balance, err := tm.GetAssetsAccountBalance(testApp, walletID, accountID)
	if err != nil {
		log.Error("GetAssetsAccountBalance failed, unexpected error:", err)
		return
	}
	log.Info("balance:", balance)
}

func testGetAssetsAccountTokenBalance(tm *openw.WalletManager, walletID, accountID string, contract openwallet.SmartContract) {
	balance, err := tm.GetAssetsAccountTokenBalance(testApp, walletID, accountID, contract)
	if err != nil {
		log.Error("GetAssetsAccountTokenBalance failed, unexpected error:", err)
		return
	}
	log.Info("token balance:", balance.Balance)
}

func testCreateTransactionStep(tm *openw.WalletManager, walletID, accountID, to, amount, feeRate string, contract *openwallet.SmartContract) (*openwallet.RawTransaction, error) {

	//err := tm.RefreshAssetsAccountBalance(testApp, accountID)
	//if err != nil {
	//	log.Error("RefreshAssetsAccountBalance failed, unexpected error:", err)
	//	return nil, err
	//}

	rawTx, err := tm.CreateTransaction(testApp, walletID, accountID, amount, to, feeRate, "", contract)

	if err != nil {
		log.Error("CreateTransaction failed, unexpected error:", err)
		return nil, err
	}

	return rawTx, nil
}

func testCreateSummaryTransactionStep(
	tm *openw.WalletManager,
	walletID, accountID, summaryAddress, minTransfer, retainedBalance, feeRate string,
	start, limit int,
	contract *openwallet.SmartContract,
	feeSupportAccount *openwallet.FeesSupportAccount) ([]*openwallet.RawTransactionWithError, error) {

	rawTxArray, err := tm.CreateSummaryRawTransactionWithError(testApp, walletID, accountID, summaryAddress, minTransfer,
		retainedBalance, feeRate, start, limit, contract, feeSupportAccount)

	if err != nil {
		log.Error("CreateSummaryTransaction failed, unexpected error:", err)
		return nil, err
	}

	return rawTxArray, nil
}

func testSignTransactionStep(tm *openw.WalletManager, rawTx *openwallet.RawTransaction) (*openwallet.RawTransaction, error) {

	_, err := tm.SignTransaction(testApp, rawTx.Account.WalletID, rawTx.Account.AccountID, "12345678", rawTx)
	if err != nil {
		log.Error("SignTransaction failed, unexpected error:", err)
		return nil, err
	}

	log.Infof("rawTx: %+v", rawTx)
	return rawTx, nil
}

func testVerifyTransactionStep(tm *openw.WalletManager, rawTx *openwallet.RawTransaction) (*openwallet.RawTransaction, error) {

	//log.Info("rawTx.Signatures:", rawTx.Signatures)

	_, err := tm.VerifyTransaction(testApp, rawTx.Account.WalletID, rawTx.Account.AccountID, rawTx)
	if err != nil {
		log.Error("VerifyTransaction failed, unexpected error:", err)
		return nil, err
	}

	log.Infof("rawTx: %+v", rawTx)
	return rawTx, nil
}

func testSubmitTransactionStep(tm *openw.WalletManager, rawTx *openwallet.RawTransaction) (*openwallet.RawTransaction, error) {

	tx, err := tm.SubmitTransaction(testApp, rawTx.Account.WalletID, rawTx.Account.AccountID, rawTx)
	if err != nil {
		log.Error("SubmitTransaction failed, unexpected error:", err)
		return nil, err
	}

	log.Std.Info("tx: %+v", tx)
	log.Info("wxID:", tx.WxID)
	log.Info("txID:", rawTx.TxID)

	return rawTx, nil
}

func TestTransfer_ONT(t *testing.T) {

	//AGQHruK4xneV7cx7nY1X3GvZ7rTAZNfm5F
	//AK9xbPMDLfyJWCZjL1daQcGnNoFkAh4N6v
	//ASixSRsz1pa1SoPbENUrT5XSC1Mz6fqBtA
	//AVp5xHQDweK2AYVdquue1ay9UjhnLiKRCQ
	//AZjb76TiRWuZf8rwev1B4xaUEb1GgeKtBT
	//Ac3Sf1wQaUqrNAnt2nD2jUz122a8yskmkc

	tm := testInitWalletManager()
	//walletID := "W5TnXcpuvK2zoJCCdN7YaHayh2EdCpdHWQ"
	//accountID := "8Eu54RRVyq6SApMh5EMgYEYg5Q4F53UFuP5J45jBn2t5"
	//to := "AW2dJUGzT14geN3as5hgRxrBCHwYXCkcCm"

	walletID := "WDsEFTdwHqRxSfwTGQm1j2an8M6Q6zp7qX"
	accountID := "3rdEvmbQk8YDmur5yubWuZozj6qwmPSNVGcZRWqfszLy"
	to := "AH1hCEidoVLTTzYgsbhoK4WpwW6u6euCGa"

	//accountID := "8E9ytsk2fyyKbpYE6hKTG6UzAW3YZwRm5QaVHAc92kNn"
	//to := "AH6XoEEE5cm21UoGBixYtV9DoNTLM6UzYa"

	contract := openwallet.SmartContract{
		//ContractID: "0100000000000000000000000000000000000000",
		Address:    "0100000000000000000000000000000000000000",
		Symbol:     "ONT",
		Name:       "ontology",
		Token:      "ONT",
		Decimals:   0,
	}

	//testGetAssetsAccountBalance(tm, walletID, accountID)

	testGetAssetsAccountTokenBalance(tm, walletID, accountID, contract)

	rawTx, err := testCreateTransactionStep(tm, walletID, accountID, to, "1", "", &contract)
	if err != nil {
		return
	}

	_, err = testSignTransactionStep(tm, rawTx)
	if err != nil {
		return
	}

	_, err = testVerifyTransactionStep(tm, rawTx)
	if err != nil {
		return
	}

	_, err = testSubmitTransactionStep(tm, rawTx)
	if err != nil {
		return
	}

}

func TestTransfer_ONG(t *testing.T) {

	//AGQHruK4xneV7cx7nY1X3GvZ7rTAZNfm5F
	//AK9xbPMDLfyJWCZjL1daQcGnNoFkAh4N6v
	//ASixSRsz1pa1SoPbENUrT5XSC1Mz6fqBtA
	//AVp5xHQDweK2AYVdquue1ay9UjhnLiKRCQ
	//AZjb76TiRWuZf8rwev1B4xaUEb1GgeKtBT
	//Ac3Sf1wQaUqrNAnt2nD2jUz122a8yskmkc

	tm := testInitWalletManager()
	//walletID := "W5TnXcpuvK2zoJCCdN7YaHayh2EdCpdHWQ"
	//accountID := "8Eu54RRVyq6SApMh5EMgYEYg5Q4F53UFuP5J45jBn2t5"
	//to := "AW2dJUGzT14geN3as5hgRxrBCHwYXCkcCm"

	walletID := "WDsEFTdwHqRxSfwTGQm1j2an8M6Q6zp7qX"
	accountID := "3rdEvmbQk8YDmur5yubWuZozj6qwmPSNVGcZRWqfszLy"
	to := "AZTod9Ma3bDsorn6NQZgcssvyQvusnbdmp"

	contract := openwallet.SmartContract{
		ContractID: "0200000000000000000000000000000000000000",
		Address:    "0200000000000000000000000000000000000000",
		Symbol:     "ONT",
		Name:       "ontology",
		Token:      "ONG",
		Decimals:   9,
	}

	testGetAssetsAccountBalance(tm, walletID, accountID)

	testGetAssetsAccountTokenBalance(tm, walletID, accountID, contract)

	rawTx, err := testCreateTransactionStep(tm, walletID, accountID, to, "1", "", &contract)
	if err != nil {
		return
	}

	_, err = testSignTransactionStep(tm, rawTx)
	if err != nil {
		return
	}

	_, err = testVerifyTransactionStep(tm, rawTx)
	if err != nil {
		return
	}

	_, err = testSubmitTransactionStep(tm, rawTx)
	if err != nil {
		return
	}

}

func TestSummary_ONG(t *testing.T) {
	tm := testInitWalletManager()
	walletID := "WDsEFTdwHqRxSfwTGQm1j2an8M6Q6zp7qX"
	accountID := "3rdEvmbQk8YDmur5yubWuZozj6qwmPSNVGcZRWqfszLy"
	summaryAddress := "AZTod9Ma3bDsorn6NQZgcssvyQvusnbdmp"
	contract := openwallet.SmartContract{
		//ContractID: "0200000000000000000000000000000000000000",
		Address:    "0200000000000000000000000000000000000000",
		Symbol:     "ONT",
		Name:       "ontology",
		Token:      "ONG",
		Decimals:   9,
	}
	testGetAssetsAccountBalance(tm, walletID, accountID)
	testGetAssetsAccountTokenBalance(tm, walletID, accountID, contract)

	rawTxArray, err := testCreateSummaryTransactionStep(tm, walletID, accountID,
		summaryAddress, "", "", "",
		0, 100, &contract, nil)
	if err != nil {
		log.Errorf("CreateSummaryTransaction failed, unexpected error: %v", err)
		return
	}

	//执行汇总交易
	for _, rawTxWithErr := range rawTxArray {

		if rawTxWithErr.Error != nil {
			log.Error(rawTxWithErr.Error.Error())
			continue
		}

		_, err = testSignTransactionStep(tm, rawTxWithErr.RawTx)
		if err != nil {
			return
		}

		_, err = testVerifyTransactionStep(tm, rawTxWithErr.RawTx)
		if err != nil {
			return
		}

		_, err = testSubmitTransactionStep(tm, rawTxWithErr.RawTx)
		if err != nil {
			return
		}
	}

}

func TestSummary_ONT(t *testing.T) {
	tm := testInitWalletManager()
	walletID := "WDsEFTdwHqRxSfwTGQm1j2an8M6Q6zp7qX"
	accountID := "8E9ytsk2fyyKbpYE6hKTG6UzAW3YZwRm5QaVHAc92kNn"
	summaryAddress := "AH6XoEEE5cm21UoGBixYtV9DoNTLM6UzYa"
	contract := openwallet.SmartContract{
		//ContractID: "0100000000000000000000000000000000000000",
		Address:    "0100000000000000000000000000000000000000",
		Symbol:     "ONT",
		Name:       "ontology",
		Token:      "ONT",
		Decimals:   0,
	}

	feesSupport := openwallet.FeesSupportAccount{
		AccountID: "3rdEvmbQk8YDmur5yubWuZozj6qwmPSNVGcZRWqfszLy",
	}

	testGetAssetsAccountBalance(tm, walletID, accountID)
	testGetAssetsAccountTokenBalance(tm, walletID, accountID, contract)

	rawTxArray, err := testCreateSummaryTransactionStep(tm, walletID, accountID,
		summaryAddress, "", "", "",
		0, 100, &contract, &feesSupport)
	if err != nil {
		log.Errorf("CreateSummaryTransaction failed, unexpected error: %v", err)
		return
	}

	//执行汇总交易
	for _, rawTxWithErr := range rawTxArray {

		if rawTxWithErr.Error != nil {
			log.Error(rawTxWithErr.Error.Error())
			continue
		}

		_, err = testSignTransactionStep(tm, rawTxWithErr.RawTx)
		if err != nil {
			return
		}

		_, err = testVerifyTransactionStep(tm, rawTxWithErr.RawTx)
		if err != nil {
			return
		}

		_, err = testSubmitTransactionStep(tm, rawTxWithErr.RawTx)
		if err != nil {
			return
		}
	}

}
