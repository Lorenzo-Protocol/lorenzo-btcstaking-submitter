package btc

import (
	"fmt"
	"github.com/btcsuite/btcd/txscript"
	"testing"
)

func TestQueryBlockByHeight(t *testing.T) {
	btcQuery := NewBTCQuery("https://btc-rpc-testnet.lorenzo-protocol.xyz/testnet/api")
	block, err := btcQuery.GetBlockByHeight(2815059)
	if err != nil {
		t.Error(err)
		return
	}

	receivingAddress := "tb1p97g0dpmsm2fxkmkw9w7mpasmxprsye3k0v49qknwmclwxj78rfjqu6nacq"
	expectTxid := "45a691b6de4526147880737792a8677b7fe9c385851cd946e99e86f209169acc"
	for _, tx := range block.Transactions {
		for _, out := range tx.TxOut {
			pkScript, err := txscript.ParsePkScript(out.PkScript)
			if err != nil {
				continue
			}

			txid := tx.TxHash().String()
			addr, err := pkScript.Address(GetBTCParams("testnet"))
			if err != nil {
				continue
			}
			if addr.String() != receivingAddress {
				continue
			}
			if txid != expectTxid {
				t.Errorf("bad txid: %s, expect:%s", txid, expectTxid)
			}
			fmt.Println(out.Value)
		}
	}
}
