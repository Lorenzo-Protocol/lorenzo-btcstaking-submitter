package btc

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func ExtractPaymentToWithOpReturnId(tx *wire.MsgTx, addr btcutil.Address) (uint64, []byte, error) {
	const maxOpReturnPkScriptSize = 83
	payToAddrScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid address")
	}
	var amt uint64 = 0
	foundOpReturnId := false
	var opReturnId []byte
	for _, out := range tx.TxOut {
		if bytes.Equal(out.PkScript, payToAddrScript) {
			amt += uint64(out.Value)
		} else {
			pkScript := out.PkScript
			pkScriptLen := len(pkScript)
			// valid op return script will have at least 2 bytes
			// - fisrt byte should be OP_RETURN marker
			// - second byte should indicate how many bytes there are in opreturn script
			if pkScriptLen > 1 &&
				pkScriptLen <= maxOpReturnPkScriptSize &&
				pkScript[0] == txscript.OP_RETURN {

				// if this is OP_PUSHDATA1, we need to drop first 3 bytes as those are related
				// to script iteslf i.e OP_RETURN + OP_PUSHDATA1 + len of bytes
				if pkScript[1] == txscript.OP_PUSHDATA1 {
					opReturnId = pkScript[3:]
				} else {
					// this should be one of OP_DATAXX opcodes we drop first 2 bytes
					opReturnId = pkScript[2:]
				}
				foundOpReturnId = true
			}
		}
	}
	if !foundOpReturnId {
		return 0, nil, fmt.Errorf("expected op_return_id not found")
	}
	return amt, opReturnId, nil
}

func NewBTCTxFromBytes(txBytes []byte) (*wire.MsgTx, error) {
	var msgTx wire.MsgTx
	rbuf := bytes.NewReader(txBytes)
	if err := msgTx.Deserialize(rbuf); err != nil {
		return nil, err
	}

	return &msgTx, nil
}

func GetBTCParams(net string) *chaincfg.Params {
	switch net {
	case "mainnet":
		return &chaincfg.MainNetParams
	case "testnet":
		return &chaincfg.TestNet3Params
	}

	err := fmt.Errorf(
		"BTC network with name %s does not exist. should be one of {testnet,mainnet}",
		net,
	)
	panic(err)
}
