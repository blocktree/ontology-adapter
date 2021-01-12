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

	"github.com/blocktree/openwallet/v2/log"
	"github.com/blocktree/openwallet/v2/openw"
	"github.com/blocktree/openwallet/v2/openwallet"
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

	rawTx, err := tm.CreateTransaction(testApp, walletID, accountID, amount, to, feeRate, "", contract, nil)

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
	contract *openwallet.SmartContract, feeSupportAccount *openwallet.FeesSupportAccount) ([]*openwallet.RawTransactionWithError, error) {

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
	tm := testInitWalletManager()
	walletID := "WMNYyUffg3P2HxbksXnmhkDA73uDX4MFKz"
	accountID := "BXhbNhoaDPSVMnRoUTEfvv2yFnCs11H81AnrGEHZjHHy"
	to := "AW2dJUGzT14geN3as5hgRxrBCHwYXCkcCm"

	contract := openwallet.SmartContract{
		ContractID: "",
		Address:    "0100000000000000000000000000000000000000",
		Symbol:     "ONT",
		Name:       "ontology",
		Token:      "ONT",
		Decimals:   0,
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

func TestTransfer_ONG(t *testing.T) {
	tm := testInitWalletManager()
	walletID := "WMNYyUffg3P2HxbksXnmhkDA73uDX4MFKz"
	accountID := "BXhbNhoaDPSVMnRoUTEfvv2yFnCs11H81AnrGEHZjHHy"
	to := "AU4rGqqGMuQ6eicgAXxpFCiqQUTxz5YuDz"

	contract := openwallet.SmartContract{
		ContractID: "",
		Address:    "0200000000000000000000000000000000000000",
		Symbol:     "ONT",
		Name:       "ontology",
		Token:      "ONG",
		Decimals:   9,
	}

	testGetAssetsAccountBalance(tm, walletID, accountID)

	testGetAssetsAccountTokenBalance(tm, walletID, accountID, contract)

	rawTx, err := testCreateTransactionStep(tm, walletID, accountID, to, "0.01", "", &contract)
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
	walletID := "WMNYyUffg3P2HxbksXnmhkDA73uDX4MFKz"
	accountID := "BXhbNhoaDPSVMnRoUTEfvv2yFnCs11H81AnrGEHZjHHy"
	//feeSupportAccountID := "7cNvUsvhnUTdduCD2iheyXgwb1zt1f7tWc8XF9q3DLXJ"
	summaryAddress := "AMEMKbmLwgEY8tCzJo7rHXdMbULhBsnpTk"
	contract := openwallet.SmartContract{
		ContractID: "",
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
	for _, rawTx := range rawTxArray {
		_, err = testSignTransactionStep(tm, rawTx.RawTx)
		if err != nil {
			return
		}

		_, err = testVerifyTransactionStep(tm, rawTx.RawTx)
		if err != nil {
			return
		}

		_, err = testSubmitTransactionStep(tm, rawTx.RawTx)
		if err != nil {
			return
		}
	}

}

func TestSummary_ONT(t *testing.T) {
	tm := testInitWalletManager()
	walletID := "WMNYyUffg3P2HxbksXnmhkDA73uDX4MFKz"
	accountID := "2HTnrBSFvZZnFeSXFinbfH1zvxojyXBjhnSN5CyznyzB"
	summaryAddress := "AJZT7WkUZtK8dfBkxAiVzfgBHDihGyAWzh"
	contract := openwallet.SmartContract{
		ContractID: "",
		Address:    "0100000000000000000000000000000000000000",
		Symbol:     "ONT",
		Name:       "ontology",
		Token:      "ONT",
		Decimals:   0,
	}

	feesSupport := openwallet.FeesSupportAccount{
		AccountID: "BXhbNhoaDPSVMnRoUTEfvv2yFnCs11H81AnrGEHZjHHy",
	}
	testGetAssetsAccountBalance(tm, walletID, accountID)
	testGetAssetsAccountTokenBalance(tm, walletID, accountID, contract)

	rawTxArray, err := testCreateSummaryTransactionStep(tm, walletID, accountID,
		summaryAddress, "0", "0", "",
		0, 100, &contract, &feesSupport)
	if err != nil {
		log.Errorf("CreateSummaryTransaction failed, unexpected error: %v", err)
		return
	}

	//执行汇总交易
	for _, rawTx := range rawTxArray {
		_, err = testSignTransactionStep(tm, rawTx.RawTx)
		if err != nil {
			return
		}

		_, err = testVerifyTransactionStep(tm, rawTx.RawTx)
		if err != nil {
			return
		}

		_, err = testSubmitTransactionStep(tm, rawTx.RawTx)
		if err != nil {
			return
		}
	}

}
