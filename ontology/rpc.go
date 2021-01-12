package ontology

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/blocktree/go-owcdrivers/ontologyTransaction"
	"github.com/tidwall/gjson"
)

type RpcClient struct {
	addr       string
	httpClient *http.Client
}

func NewRpcClient(addr string) *RpcClient {
	return &RpcClient{
		addr: addr,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   5,
				DisableKeepAlives:     false,
				IdleConnTimeout:       300000000000,
				ResponseHeaderTimeout: 300000000000,
			},
			Timeout: 300000000000,
		},
	}
}

type JsonRpcRequest struct {
	Version string        `json:"jsonrpc"`
	Id      string        `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type JsonRpcResponse struct {
	Id     string          `json:"id"`
	Error  int64           `json:"error"`
	Desc   string          `json:"desc"`
	Result json.RawMessage `json:"result"`
}

func (this *RpcClient) sendRpcRequest(qid, method string, params []interface{}) ([]byte, error) {
	rpcReq := &JsonRpcRequest{
		Version: "2.0",
		Id:      qid,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("JsonRpcRequest json.Marsha error:%s", err)
	}
	// resp, err := this.httpClient.Post(this.addr, "application/json", bytes.NewReader(data))
	resp, err := http.Post(this.addr, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("http post request:%s error:%s", data, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rpc response body error:%s", err)
	}

	rpcRsp := &JsonRpcResponse{}
	err = json.Unmarshal(body, rpcRsp)
	if err != nil {
		return nil, fmt.Errorf("json.Unmarshal JsonRpcResponse:%s error:%s", body, err)
	}
	if rpcRsp.Error != 0 {
		return nil, fmt.Errorf("JsonRpcResponse error code:%d desc:%s result:%s", rpcRsp.Error, rpcRsp.Desc, rpcRsp.Result)
	}
	return rpcRsp.Result, nil
}

func (rpc *RpcClient) getBlockHeightFromTxID(txid string) (uint64, error) {
	param := []interface{}{txid}

	resp, err := rpc.sendRpcRequest("0", "getblockheightbytxhash", param)

	if err != nil {
		return 0, err
	}
	height, _ := strconv.Atoi(string(resp))
	return uint64(height), nil
}

func (rpc *RpcClient) getBlockByHeight(height uint64) (*Block, error) {
	params := []interface{}{height, 1}

	resp, err := rpc.sendRpcRequest("0", "getblock", params)
	if err != nil {
		return nil, err
	}

	json := gjson.Parse(string(resp))

	return NewBlock(&json), nil
}

func (rpc *RpcClient) getBlock(hash string) (*Block, error) {
	params := []interface{}{hash, 1}

	resp, err := rpc.sendRpcRequest("0", "getblock", params)
	if err != nil {
		return nil, err
	}
	json := gjson.Parse(string(resp))

	return NewBlock(&json), nil
}

func (rpc *RpcClient) getTransaction(txid string) (*Transaction, error) {
	param := []interface{}{txid, 1}
	resp, err := rpc.sendRpcRequest("0", "getrawtransaction", param)
	if err != nil {
		return nil, err
	}

	trx := Transaction{}
	trx.TxID = gjson.Get(string(resp), "Hash").String()
	trx.Version = gjson.Get(string(resp), "Version").Uint()
	trx.Nonce = gjson.Get(string(resp), "Nonce").Uint()
	trx.GasPrice = gjson.Get(string(resp), "GasPrice").Uint()
	trx.GasLimit = gjson.Get(string(resp), "GasLimit").Uint()
	trx.Payer = gjson.Get(string(resp), "Payer").String()
	trx.TxType = gjson.Get(string(resp), "TxType").Uint()
	trx.BlockHeight = gjson.Get(string(resp), "Height").Uint()
	trx.BlockHash, err = rpc.getBlockHash(trx.BlockHeight)
	if err != nil {
		return nil, err
	}

	trx.Notifys, err = rpc.getTxDetail(trx.TxID)

	if err != nil {
		return nil, err
	}
	return &trx, nil
}

func (rpc *RpcClient) getBlockHeight() (uint64, error) {
	param := []interface{}{}

	resp, err := rpc.sendRpcRequest("0", "getblockcount", param)
	if err != nil {
		return 0, err
	}
	height, _ := strconv.Atoi(string(resp))
	return uint64(height), nil
}

func (rpc *RpcClient) getBlockHash(height uint64) (string, error) {
	params := []interface{}{uint32(height)}

	hash, err := rpc.sendRpcRequest("0", "getblockhash", params)
	if err != nil {
		return "", err
	}
	return string(hash)[1:65], nil
}

func (rpc *RpcClient) getTxCountImMemPool() (uint64, error) {
	param := []interface{}{}

	resp, err := rpc.sendRpcRequest("0", "getmempooltxcount", param)
	if err != nil {
		return 0, err
	}
	height, _ := strconv.Atoi(string(resp))
	return uint64(height), nil
}

func (rpc *RpcClient) getONTBalance(address string) (*AddrBalance, error) {
	params := []interface{}{address}

	balance, err := rpc.sendRpcRequest("0", "getbalance", params)
	if err != nil {
		return nil, errors.New("get ONT balance failed!")
	}
	ret := newONTBalance(string(balance))
	ret.Address = address

	return ret, nil
}

func (rpc *RpcClient) getONGBalance(address string) (*AddrBalance, error) {
	return rpc.getBalance(address)
}

func (rpc *RpcClient) getBalance(address string) (*AddrBalance, error) {

	params := []interface{}{address}

	balance, err := rpc.sendRpcRequest("0", "getbalance", params)
	if err != nil {
		return nil, errors.New("Get address balance failed")
	}

	unboundong, err := rpc.sendRpcRequest("0", "getunboundong", params)

	ret := newAddrBalance([]string{string(balance), string(unboundong)})

	if ret == nil {
		return nil, errors.New("Get address balance failed!")
	}

	ret.Address = address

	return ret, nil
}

// from,to,amount,contract,method,error
func (rpc *RpcClient) getTxDetail(txid string) ([]Notify, error) {
	params := []interface{}{txid}

	resp, err := rpc.sendRpcRequest("0", "getsmartcodeevent", params)
	if err != nil {
		return nil, errors.New("Get transaction result failed")
	}
	if err != nil {
		return nil, errors.New("Get transaction result failed")
	}

	notifys := gjson.Get(string(resp), "Notify").Array()
	var ret []Notify
	if len(notifys) >= 1 {
		for _, notify := range notifys {
			contractAddress := notify.Get("ContractAddress").String()
			if contractAddress != ontologyTransaction.ONGContractAddress && contractAddress != ontologyTransaction.ONTContractAddress {
				continue
			}
			states := notify.Get("States").Array()

			if len(states) != 4 {
				return nil, errors.New("Get transaction result failed")
			}
			if states[2].String() != "AFmseVrdL9f9oyCzZefL9tG6UbviEH9ugK" {
				ret = append(ret, Notify{
					ContractAddress: contractAddress,
					Method:          states[0].String(),
					From:            states[1].String(),
					To:              states[2].String(),
					Amount:          states[3].String(),
				})
			} else {
				ret = append(ret, Notify{
					ContractAddress: contractAddress,
					IsFee:           true,
					Method:          states[0].String(),
					From:            states[1].String(),
					To:              states[2].String(),
					Amount:          states[3].String(),
				})
			}

		}
	}

	return ret, nil
}

func (rpc *RpcClient) getGasPrice() (uint64, error) {
	params := []interface{}{}
	resp, err := rpc.sendRpcRequest("0", "getgasprice", params)
	if err != nil {
		return 0, err
	}

	return gjson.Get(string(resp), "gasprice").Uint(), nil
}
