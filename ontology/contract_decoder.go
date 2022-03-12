package ontology

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/blocktree/go-owcdrivers/ontologyTransaction"
	"github.com/blocktree/openwallet/v2/openwallet"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
)

type AddrBalance struct {
	Address    string
	ONTBalance *big.Int
	ONGBalance *big.Int
	ONGUnbound *big.Int
	index      int
}

func newONTBalance(data string) *AddrBalance {
	ontBalance, err := strconv.ParseInt(gjson.Get(data, "ont").String(), 10, 64)
	if err != nil {
		return nil
	}
	return &AddrBalance{
		ONTBalance: big.NewInt(ontBalance),
	}
}

func newAddrBalance(data []string) *AddrBalance {
	ontBalance, ret := new(big.Int).SetString(gjson.Get(data[0], "ont").String(), 10) //strconv.ParseInt(gjson.Get(data[0], "ont").String(), 10, 64)
	if ret == false {
		return nil
	}

	ongBalance, ret := new(big.Int).SetString(gjson.Get(data[0], "ong").String(), 10) //strconv.ParseInt(gjson.Get(data[0], "ong").String(), 10, 64)
	if ret == false {
		return nil
	}

	ongUnbound, err := strconv.ParseInt(data[1][1:len(data[1])-1], 10, 64)
	if err != nil {
		return nil
	}
	return &AddrBalance{
		ONTBalance: ontBalance,
		ONGBalance: ongBalance,
		ONGUnbound: big.NewInt(ongUnbound),
	}
}

func convertFloatStringToBigInt(amount string, offset int) (*big.Int, error) {
	vDecimal, err := decimal.NewFromString(amount)
	if err != nil {
		return nil, err
	}

	decimalInt := big.NewInt(1)
	for i := 0; i < offset; i++ {
		decimalInt.Mul(decimalInt, big.NewInt(10))
	}
	d, _ := decimal.NewFromString(decimalInt.String())
	vDecimal = vDecimal.Mul(d)
	rst := new(big.Int)
	if _, valid := rst.SetString(vDecimal.String(), 10); !valid {
		return nil, errors.New("conver to big.int failed")
	}
	return rst, nil
}

func convertBigIntToFloatDecimal(amount string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(amount)
	if err != nil {
		return d, err
	}

	decimalInt := big.NewInt(1)
	for i := 0; i < 9; i++ {
		decimalInt.Mul(decimalInt, big.NewInt(10))
	}

	w, _ := decimal.NewFromString(decimalInt.String())
	d = d.Div(w)
	return d, nil
}

func convertIntStringToBigInt(amount string) (*big.Int, error) {
	vInt64, err := strconv.ParseInt(amount, 10, 64)
	if err != nil {
		return nil, err
	}

	return big.NewInt(vInt64), nil
}

func convertBigIntToFloatViaDecimal(amount string, decimals int) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(amount)
	if err != nil {
		return d, err
	}

	decimalInt := big.NewInt(1)
	for i := 0; i < decimals; i++ {
		decimalInt.Mul(decimalInt, big.NewInt(10))
	}

	w, _ := decimal.NewFromString(decimalInt.String())
	d = d.Div(w)
	return d, nil
}

type ContractDecoder struct {
	*openwallet.SmartContractDecoderBase
	wm *WalletManager
}

//NewContractDecoder 智能合约解析器
func NewContractDecoder(wm *WalletManager) *ContractDecoder {
	decoder := ContractDecoder{}
	decoder.wm = wm
	return &decoder
}

func (decoder *ContractDecoder) GetTokenBalanceByAddress(contract openwallet.SmartContract, address ...string) ([]*openwallet.TokenBalance, error) {

	var tokenBalanceList []*openwallet.TokenBalance

	for i := 0; i < len(address); i++ {
		tokenBalance := openwallet.TokenBalance{
			Contract: &contract,
		}
		if contract.Address == ontologyTransaction.ONTContractAddress {
			balance, err := decoder.wm.RPCClient.getONTBalance(address[i])
			if err != nil {
				return nil, fmt.Errorf("Get ONT balance of address [%v] failed with error : " + address[i])
			}

			balanceWithDecimal, err := convertBigIntToFloatViaDecimal(balance.ONTBalance.String(), int(contract.Decimals))
			tokenBalance.Balance = &openwallet.Balance{
				Address:          address[i],
				Symbol:           contract.Symbol,
				Balance:          balanceWithDecimal.String(),
				ConfirmBalance:   balanceWithDecimal.String(),
				UnconfirmBalance: "0",
			}
		} else if contract.Address == ontologyTransaction.ONGContractAddress {
			balance, err := decoder.wm.RPCClient.getONGBalance(address[i])
			if err != nil {
				return nil, fmt.Errorf("Get ONG balance of address [%v] failed with error : " + address[i])
			}

			balanceWithDecimal, err := convertBigIntToFloatViaDecimal(balance.ONGBalance.String(), int(contract.Decimals))
			unboundBalanceWithDecimal, err := convertBigIntToFloatViaDecimal(balance.ONGUnbound.String(), int(contract.Decimals))
			tokenBalance.Balance = &openwallet.Balance{
				Address:          address[i],
				Symbol:           contract.Symbol,
				Balance:          balanceWithDecimal.String(),
				ConfirmBalance:   balanceWithDecimal.String(),
				UnconfirmBalance: unboundBalanceWithDecimal.String(),
			}
		} else {
			// other contract
		}
		tokenBalanceList = append(tokenBalanceList, &tokenBalance)
	}

	return tokenBalanceList, nil
}
