# ontology-adapter

本项目适配了openwallet.AssetsAdapter接口，给应用提供了底层的区块链协议支持。

## 如何测试

openwtester包下的测试用例已经集成了openwallet钱包体系，创建conf文件，新建ONT.ini文件，编辑如下内容：

```ini


# restful api url
restfulServerAPI = "http://ip:port"

# gasLimit 
gasLimit = 20000

# gas price type 0: fixed   1: get from node
gasPriceType = 0

# fixexd  gas price value
gasPriceFixed = 500

```

## Tips
合约为"0200000000000000000000000000000000000000",转账金额指定为0时，可以提取对应地址上未解绑的ong