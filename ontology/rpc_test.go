package ontology

import (
	"fmt"
	"testing"
)

func Test_getBalanceByRest(t *testing.T) {
	//address := "ATfZt5HAHrx3Xmio3Ak9rr23SyvmgNVJqU"
	address := "ASvZLkStvC87CXrEdxjFApi2uXufSyHqsg"

	balance, err := tw.RPCClient.getBalance(address)

	if err != nil {
		t.Error("get balance by local failed!")
	} else {
		fmt.Println("ONT: ", balance.ONTBalance)
		fmt.Println("ONG: ", balance.ONGBalance)
		fmt.Println("ONGUnbound: ", balance.ONGUnbound)
	}
}

func Test_getBlock(t *testing.T) {
	hash := "2acad6ed9fe771a2631f311cba5738c3e64e39606877074f6d26551806dc600e"
	ret, err := tw.RPCClient.getBlock(hash)

	if err != nil {
		t.Error("get current block height failed!")
	} else {
		fmt.Println("current block height: ", ret)
	}
}

func Test_getBlockByHeight(t *testing.T) {
	height := uint64(1685)
	ret, err := tw.RPCClient.getBlockByHeight(height)

	if err != nil {
		t.Error("get current block height failed!")
	} else {
		fmt.Println("current block height: ", ret)
	}
}

func Test_getBlockHeightFromTxIDByRest(t *testing.T) {
	txid := "652edb90c0a46d2f6a220c293f0dbf002c72a3716c47ca32a95e60e568361f58"

	height, err := tw.RPCClient.getBlockHeightFromTxID(txid)

	if err != nil {
		t.Error("get current block height failed!")
	} else {
		fmt.Println("current block height: ", height)
	}
}

func Test_getBlockHeightByRest(t *testing.T) {
	height, err := tw.RPCClient.getBlockHeight()

	if err != nil {
		t.Error("get current block height failed!")
	} else {
		fmt.Println("current block height: ", height)
	}
}

func Test_getBlockHashByRest(t *testing.T) {
	hash, err := tw.RPCClient.getBlockHash(4066042)
	if err != nil {
		t.Error("get block hash failed!")
	} else {
		fmt.Println("block hash is  :", hash)
	}
}

func Test_getTransaction(t *testing.T) {
	txid := "0bc0c7a34f5e83211a03a9f6f3fc8c03ef7a9da2b45b87886d90488eff3bfc60"
	trx, err := tw.RPCClient.getTransaction(txid)
	fmt.Println(err)
	fmt.Println(trx)
}
