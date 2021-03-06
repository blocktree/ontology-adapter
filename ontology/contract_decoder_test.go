package ontology

import (
	"fmt"
	"testing"

	"github.com/blocktree/go-owcdrivers/ontologyTransaction"
	"github.com/blocktree/openwallet/v2/openwallet"
)

func Test_GetTokenBalanceByAddress(t *testing.T) {
	tm := NewWalletManager()
	rpc := NewRpcClient("http://192.168.27.124:20336")
	tm.RPCClient = rpc

	contract := openwallet.SmartContract{
		Symbol:     "ONT",
		ContractID: "",
	}

	address1 := "AYmuoVvtCojm1F3ATMf2fNww3wBNvAxbi5"
	address2 := "AaCe8nVkMRABnp5YgEjYZ9E5KYCxks2uce"

	ret, err := tm.ContractDecoder.GetTokenBalanceByAddress(contract, address1, address2)

	if err != nil {
		t.Error(err)
	} else {
		fmt.Println(ret)
	}

	contract.Address = ontologyTransaction.ONGContractAddress
	ret, err = tm.ContractDecoder.GetTokenBalanceByAddress(contract, address1, address2)

	if err != nil {
		t.Error(err)
	} else {
		fmt.Println(ret)
	}
}
