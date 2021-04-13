package context

import (
	//"encoding/json"
	//"fmt"
	"errors"
	"strconv"
)

type WithdrawalEventAttribute struct {
	Key string `json:"key"`
	Value string `json:"value"`
}

type WithdrawalEvents struct {
	Type string `json:"type"`
	Attributes []WithdrawalEventAttribute `json:"attributes"`
}

type WithdrawalCosmosLog struct {
	MsgIndex uint64 `json:"msg_index"`
	Events []WithdrawalEvents `json:"events"`

}

type ParsedWithdrawalTxn struct {
	ToChainID string
	LockProxyHash string
	AssetHash string
	FeeAddress string
	FeeAmount uint64
}

func (ptx ParsedWithdrawalTxn) hasEnoughFees(referenceAssetHash string, feeAddress string, minAmount uint64) bool {
	return ptx.FeeAddress == feeAddress && ptx.FeeAmount >= minAmount // TODO: just check > 0 for now but should check asset and set proper min amount in the future
	//return ptx.AssetHash == referenceAssetHash && ptx.FeeAmount >= minAmount
}

func (ptx ParsedWithdrawalTxn) isMatch(lockProxyHash string, toChainID string) bool {
	return ptx.LockProxyHash == lockProxyHash && ptx.ToChainID == toChainID
}

//func main() {
//	data := `[{"msg_index":0,"log":"Withdrawal success","events":[{"type":"lock","attributes":[{"key":"from_contract_hash","value":"737774682d62"},{"key":"to_chain_id","value":"6"},{"key":"to_chain_proxy_hash","value":"b5d4f343412dc8efb6ff599d790074d0f1e8d430"},{"key":"to_chain_asset_hash","value":"250b211ee44459dad5cd3bca803dd6a7ecb5d46c"},{"key":"from_address","value":"swth1p07l6hcdjlme8vn23a0p99ca68awv0m05v9rp0"},{"key":"to_address","value":"3e4399b25b31e4b2b02268da9b51aed41ca305ca"},{"key":"amount","value":"1100000000"},{"key":"lock_proxy_hash","value":"1a785cfc5dbec2e1518e1b1d369154d0ce579640"},{"key":"fee_amount","value":"1"},{"key":"fee_address","value":"swth1prv0t8j8tqcdngdmjlt59pwy6dxxmtqgycy2h7"},{"key":"nonce","value":"11095"}]},{"type":"make_from_cosmos_proof","attributes":[{"key":"status","value":"1"},{"key":"cross_chainId","value":"11117"},{"key":"make_tx_param_hash","value":"95d97b9001f0c39c745f65657291fb923596876b42172daa9dfd078feb59cf4c"},{"key":"from_address","value":"swth1p07l6hcdjlme8vn23a0p99ca68awv0m05v9rp0"},{"key":"from_contract","value":"1a785cfc5dbec2e1518e1b1d369154d0ce579640"},{"key":"to_chain_id","value":"6"},{"key":"make_tx_param","value":"20c1efef580f91f5ea7c1fd9ca70ae2c1bc3b9ff1af14dd76557b1035593cf79a6022b6d141a785cfc5dbec2e1518e1b1d369154d0ce579640060000000000000014b5d4f343412dc8efb6ff599d790074d0f1e8d43006756e6c6f636bbb06737774682d6214250b211ee44459dad5cd3bca803dd6a7ecb5d46c143e4399b25b31e4b2b02268da9b51aed41ca305ca00ab90410000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001408d8f59e475830d9a1bb97d74285c4d34c6dac08140bfdfd5f0d97f793b26a8f5e12971dd1fae63f6f572b000000000000000000000000000000000000000000000000000000000000"}]},{"type":"message","attributes":[{"key":"action","value":"withdraw"},{"key":"sender","value":"swth1p07l6hcdjlme8vn23a0p99ca68awv0m05v9rp0"},{"key":"sender","value":"swth196w5cyqfezzkv6pzl5wsrmfwgesenzykag5yd2"},{"key":"sender","value":"swth1p07l6hcdjlme8vn23a0p99ca68awv0m05v9rp0"}]},{"type":"transfer","attributes":[{"key":"recipient","value":"swth196w5cyqfezzkv6pzl5wsrmfwgesenzykag5yd2"},{"key":"sender","value":"swth1p07l6hcdjlme8vn23a0p99ca68awv0m05v9rp0"},{"key":"amount","value":"1100000000swth"},{"key":"recipient","value":"swth1p07l6hcdjlme8vn23a0p99ca68awv0m05v9rp0"},{"key":"sender","value":"swth196w5cyqfezzkv6pzl5wsrmfwgesenzykag5yd2"},{"key":"amount","value":"1100000000swth-b"},{"key":"recipient","value":"swth17nqz88jcgu6rwm3fk92l8kcnwgxgs59vqavtvq"},{"key":"sender","value":"swth1p07l6hcdjlme8vn23a0p99ca68awv0m05v9rp0"},{"key":"amount","value":"1100000000swth-b"}]}]}]`
//	var logs []WithdrawalCosmosLog
//	json.Unmarshal([]byte(data), &logs)
//	fmt.Printf("After unmarshal: %#v\n", logs)
//
//	parsedTxn, err := parseWithdrawalTxn(logs)
//	if err != nil {
//		fmt.Println("Unable to parse")
//	}
//
//	fmt.Println("parsedTxn", parsedTxn)
//	fmt.Println("hasEnoughFees", parsedTxn.hasEnoughFees("250b211ee44459dad5cd3bca803dd6a7ecb5d46c", "swth1prv0t8j8tqcdngdmjlt59pwy6dxxmtqgycy2h7", 1))
//	fmt.Println("isMatch", parsedTxn.isMatch("b5d4f343412dc8efb6ff599d790074d0f1e8d430", "swth1prv0t8j8tqcdngdmjlt59pwy6dxxmtqgycy2h7", "6"))
//}

// parse relevant data from the txn so we can use it to filter txns later on
func parseWithdrawalTxn(logs []WithdrawalCosmosLog) (parsedTxn *ParsedWithdrawalTxn, err error) {
	// check we can access this first: logs[0].Events[0].Attributes[0].Key
	if len(logs) == 0 || len(logs[0].Events) == 0 || len(logs[0].Events[0].Attributes) == 0  {
		return nil, errors.New("Unable to parse logs as some keys are missing")
	}
	parsedTxn = &ParsedWithdrawalTxn{}

	attributes := logs[0].Events[0].Attributes

	parsedTxn.FeeAddress, err = valueForKey(attributes, "fee_address")
	if err != nil {
		return nil, err
	}
	var feeAmount string
	feeAmount, err = valueForKey(attributes, "fee_amount")
	if err != nil {
		return nil, err
	}
	parsedTxn.FeeAmount, err = strconv.ParseUint(feeAmount, 10, 64)

	parsedTxn.AssetHash, err = valueForKey(attributes, "to_chain_asset_hash")
	if err != nil {
		return nil, err
	}
	parsedTxn.ToChainID, err = valueForKey(attributes, "to_chain_id")
	if err != nil {
		return nil, err
	}
	parsedTxn.LockProxyHash, err = valueForKey(attributes, "to_chain_proxy_hash")
	if err != nil {
		return nil, err
	}

	return
}

func valueForKey(array []WithdrawalEventAttribute, key string) (string, error) {
	for _, v := range array {
		if v.Key == key {
			// Found!
			return v.Value, nil
		}
	}
	return "", errors.New("Attribute key is missing")
}