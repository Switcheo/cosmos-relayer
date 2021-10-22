/*
 * Copyright (C) 2020 The poly network Authors
 * This file is part of The poly network library.
 *
 * The  poly network  is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The  poly network  is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 * You should have received a copy of the GNU Lesser General Public License
 * along with The poly network .  If not, see <http://www.gnu.org/licenses/>.
 */

package service

import (
	c "context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	tmcoretypes "github.com/tendermint/tendermint/rpc/core/types"

	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	client "github.com/polynetwork/poly-go-sdk/client"
	coretypes "github.com/polynetwork/poly/core/types"
	hsc "github.com/polynetwork/poly/native/service/header_sync/cosmos"

	ccmtypes "github.com/Switcheo/polynetwork-cosmos/x/ccm/types"
	headersynctypes "github.com/Switcheo/polynetwork-cosmos/x/headersync/types"

	"github.com/polynetwork/cosmos-relayer/context"
	"github.com/polynetwork/cosmos-relayer/log"
)

func StartRelay() {
	go ToPolyRoutine()
	go ToCosmosRoutine()
}

// Process with message from channel `ToPoly`.
// When type is `TyHeader`, we must finish the procession for this message
// before processing the transaction-messages.
// When type is `TyTx`, we relay this transaction info and its proof to Poly.
// When type is `TyUpdateHeight`, we update the cosmos height that already
// checked in our db.
// This run as a go-routine
func ToPolyRoutine() {
	for val := range ctx.ToPoly {
		switch val.Type {
		case context.TyHeader:
			if err := handleCosmosHdrs(val.Hdrs); err != nil {
				panic(err)
			}
		case context.TyTx:
			go handleCosmosTx(val.Tx, val.Hdrs[0])
		case context.TyUpdateHeight:
			go func() {
				if err := ctx.Db.SetCosmosHeight(val.Height); err != nil {
					log.Errorf("failed to update cosmos height: %v", err)
				}
			}()
		}
	}
}

// Process cosmos-headers msg. This function would not return before our
// Ploygon tx committing headers confirmed. This guarantee that the next
// cross-chain txs next to relay can be proved on Poly.
func handleCosmosHdrs(headers []*hsc.CosmosHeader) error {
	if ctx.PolyStatus.Len() > 0 {
		ctx.PolyStatus.IsBlocked = true
		ctx.PolyStatus.CosmosEpochHeight = headers[0].Header.Height
		ctx.PolyStatus.Wg.Wait()
		ctx.PolyStatus.Wg.Add(1)
	}
	for i := 0; i < len(headers); i += context.HdrLimitPerBatch {
		var hdrs []*hsc.CosmosHeader
		if i+context.HdrLimitPerBatch > len(headers) {
			hdrs = headers[i:]
		} else {
			hdrs = headers[i : i+context.HdrLimitPerBatch]
		}
		info := make([]string, len(hdrs))
		raw := make([][]byte, len(hdrs))
		for i, h := range hdrs {
			r, err := ctx.Cosmos.Cdc.MarshalBinaryBare(*h)
			if err != nil {
				log.Fatalf("[handleCosmosHdr] failed to marshal CosmosHeader: %v", err)
				return err
			}
			raw[i] = r
			info[i] = fmt.Sprintf("(hash: %s, height: %d)", h.Header.Hash().String(), h.Header.Height)
		}

	SYNC_RETRY:
		txhash, err := ctx.Poly.Native.Hs.SyncBlockHeader(ctx.Conf.SideChainId, ctx.PolyAcc.Address,
			raw, ctx.PolyAcc)
		if err != nil {
			if _, ok := err.(client.PostErr); ok {
				log.Errorf("[handleCosmosHdr] post error, retry after 10 sec wait: %v", err)
				context.SleepSecs(10)
				goto SYNC_RETRY
			}
			if strings.Contains(err.Error(), context.NoUsefulHeaders) {
				log.Warnf("[handleCosmosHdr] your headers could be wrong or already committed: headers: [ %s ]",
					strings.Join(info, ", "))
				return nil
			}
			log.Errorf("[handleCosmosHdr] failed to relay cosmos header to Poly: %v", err)
			return err
		}
		tick := time.NewTicker(100 * time.Millisecond)
		var h uint32
		startTime := time.Now()
		for range tick.C {
			h, _ = ctx.Poly.GetBlockHeightByTxHash(txhash.ToHexString())
			curr, _ := ctx.Poly.GetCurrentBlockHeight()
			if h > 0 && curr > h {
				break
			}

			if startTime.Add(100 * time.Millisecond); startTime.Second() > ctx.Conf.ConfirmTimeout {
				str := ""
				for i, hdr := range hdrs {
					str += fmt.Sprintf("( no.%d: hdr-height: %d, hdr-hash: %s )\n", i+1,
						hdr.Header.Height, hdr.Header.Hash().String())
				}
				panic(fmt.Errorf("tx( %s ) is not confirm for a long time ( over %d sec ): {\n%s}",
					txhash.ToHexString(), ctx.Conf.ConfirmTimeout, str))
			}
		}
		log.Infof("[handleCosmosHdr] successful to relay header and confirmed on Poly: { headers: [ %s ], poly: "+
			"(poly_tx: %s, poly_tx_height: %d) }", strings.Join(info, ", "), txhash.ToHexString(), h)
	}
	return nil
}

// Relay COSMOS cross-chain tx to polygon.
func handleCosmosTx(tx *context.CosmosTx, hdr *hsc.CosmosHeader) {
	raw, err := ctx.Cosmos.Cdc.MarshalBinaryBare(*hdr)
	if err != nil {
		panic(fmt.Errorf("failed to marshal cosmos header %s: %v", hdr.Commit.BlockID.Hash.String(), err))
	}
RELAY_RETRY:
	txhash, err := ctx.Poly.Native.Ccm.ImportOuterTransfer(ctx.Conf.SideChainId, tx.PVal, uint32(tx.ProofHeight+1),
		tx.Proof, ctx.PolyAcc.Address[:], raw, ctx.PolyAcc)
	if err != nil {
		if strings.Contains(err.Error(), context.TxAlreadyExist) {
			log.Debugf("[handleCosmosTx] tx already on Poly: (txhash: %s)", tx.Tx.Hash.String())
			if err = ctx.Db.DelCosmosTxReproving(tx.Tx.Hash); err != nil {
				panic(err)
			}
			return
		} else if strings.Contains(err.Error(), context.NewEpoch) {
			log.Debugf("[handleCosmosTx] new epoch already, tx %s need to reprove: %v", tx.Tx.Hash.String(), err)
			if err = ctx.Db.SetCosmosTxReproving(tx.Tx); err != nil {
				panic(fmt.Errorf("[handleCosmosTx] failed to save cosmos tx into DB: %v", err))
			}
			return
		} else if strings.Contains(err.Error(), context.UtxoNotEnough) {
			log.Debugf("[handleCosmosTx] this tx transfers btc back to bitcoin but utxo on poly is not "+
				"enough which is pretty weird, so reprove tx %s: %v", tx.Tx.Hash.String(), err)
			if err = ctx.Db.SetCosmosTxReproving(tx.Tx); err != nil {
				panic(fmt.Errorf("[handleCosmosTx] failed to save cosmos tx into DB: %v", err))
			}
		} else if _, ok := err.(client.PostErr); ok {
			log.Errorf("[handleCosmosTx] post error, retry after 10 sec wait: %v", err)
			context.SleepSecs(10)
			goto RELAY_RETRY
		}
		log.Errorf("[handleCosmosTx] failed to relay cosmos tx: (txhash: %s, tx_height %d, proof_height: %d, "+
			"error: %v)", tx.Tx.Hash.String(), tx.Tx.Height, tx.ProofHeight, err)
		return
	}
	if err := ctx.PolyStatus.AddTx(txhash, tx.Tx); err != nil {
		panic(err)
	}
	log.Infof("[handleCosmosTx] relay tx success: (txhash: %s, tx_height: %d, proof_height: %d, poly_txhash: %s)",
		tx.Tx.Hash.String(), tx.Tx.Height, tx.ProofHeight, txhash.ToHexString())
}

// Process with message from channel `ToCosmos`.
// When type is `TyHeader`, we must finish the procession for this message
// before processing next message to guarantee the expected result.
// When type is `TyTx`, we relay this transaction info and its proof to COSMOS.
// When type is `TyUpdateHeight`, we update the Poly height that already
// checked in our db.
// This run as a go-routine
func ToCosmosRoutine() {
	for val := range ctx.ToCosmos {
		switch val.Type {
		case context.TyHeader:
			handlePolyHdr(val.Hdr)
		case context.TyTx:
			handlePolyTx(val)
		case context.TyUpdateHeight:
			go func() {
				if err := ctx.Db.SetPolyHeight(val.Height); err != nil {
					log.Errorf("failed to update cosmos height: %v", err)
				}
				log.Tracef("update poly height %d to db", val.Height)
			}()
		}
	}
}

// Commit and confirm the Poly header to COSMOS
func handlePolyHdr(hdr *coretypes.Header) {
	if ctx.CMStatus.Len() > 0 {
		ctx.CMStatus.IsBlocked = true
		ctx.CMStatus.PolyEpochHeight = hdr.Height
		ctx.CMStatus.Wg.Wait()
	}
	res, seq, err := sendCosmosTx([]sdk.Msg{
		headersynctypes.NewMsgSyncHeaders(ctx.Cosmos.Address.String(), []string{hex.EncodeToString(hdr.ToArray())}),
	})
	if err != nil {
		log.Fatalf("[handlePolyHdr] handlePolyHdr error: %v", err)
		panic(err)
	}

	tick := time.NewTicker(100 * time.Millisecond)
	startTime := time.Now()
	hash := hdr.Hash()
	var resTx *tmcoretypes.ResultTx
	for range tick.C {
		client := txtypes.NewServiceClient(ctx.Cosmos.GrpcConn)
		resTx, err := client.GetTx(c.Background(), &txtypes.GetTxRequest{Hash: res.Hash.String()})
		if err != nil {
			panic(err)
		}
		if resTx == nil {
			continue
		}
		status, err := ctx.Cosmos.RpcClient.Status(c.Background())
		if err != nil {
			panic(err)
		}
		if resTx.TxResponse.Height > 0 && status.SyncInfo.LatestBlockHeight > resTx.TxResponse.Height {
			break
		}
		if startTime.Add(100 * time.Millisecond); startTime.Second() > ctx.Conf.ConfirmTimeout {
			panic(fmt.Errorf("( txhash: %s, hdr-height: %d, hdr-hash: %s ) is not confirm for a long "+
				"time ( over %d sec )", res.Hash.String(), hdr.Height, hash.ToHexString(), ctx.Conf.ConfirmTimeout))
		}
	}
	log.Infof("[handlePolyHdr] successful to relay header and confirmed on COSMOS: (cosmos_txhash: %s, "+
		"cosmos_height: %d, header: %s, height: %d, sequence: %d)", res.Hash.String(), resTx.Height, hash.ToHexString(),
		hdr.Height, seq)
}

// Relay the cross-chain tx to COSMOS.
func handlePolyTx(val *context.PolyInfo) {
	if val.Tx.IsEpoch && val.Hdr.Height > ctx.CMStatus.PolyEpochHeight && ctx.CMStatus.Len() > 0 {
		ctx.CMStatus.IsBlocked = true
		ctx.CMStatus.PolyEpochHeight = val.Hdr.Height
		ctx.CMStatus.Wg.Wait()
		ctx.CMStatus.Wg.Add(1)
	}
	res, seq, err := sendCosmosTx([]sdk.Msg{
		ccmtypes.NewMsgProcessCrossChainTx(ctx.Cosmos.Address.String(), ctx.Poly.ChainId, val.Tx.Proof,
			hex.EncodeToString(val.Hdr.ToArray()), val.HeaderProof, val.EpochAnchor),
	})
	if err != nil {
		log.Fatalf("[handlePolyTx] handlePolyTx error: %v", err)
		panic(err)
	}
	if err = ctx.CMStatus.AddTx(res.Hash, val); err != nil {
		panic(err)
	}
	log.Infof("[handlePolyTx] relay tx success: (cosmos_txhash: %s, code: %d, log: %s, poly_height: %d, "+
		"poly_hash: %s, sequence: %d)", res.Hash.String(), res.Code, res.Log, val.Tx.Height, val.Tx.TxHash, seq)
}

func sendCosmosTx(msgs []sdk.Msg) (res *tmcoretypes.ResultBroadcastTx, seq uint64, err error) {
	seq = ctx.Cosmos.Sequence.GetAndAdd()
	txBuilder := ctx.NewTxBuilder()

	err = txBuilder.SetMsgs(msgs...)
	if err != nil {
		return
	}

	// Adapted from: https://docs.cosmos.network/master/run-node/txs.html#broadcasting-a-transaction-3
	// cosmos-sdk/x/auth/ante/testutil_test.go

	// First round: we gather all the signer infos. We use the "set empty
	// signature" hack to do that.
	signMode := ctx.Cosmos.TxConfig.SignModeHandler().DefaultMode()
	sigData := signingtypes.SingleSignatureData{
		SignMode:  signMode,
		Signature: nil,
	}
	sig := signingtypes.SignatureV2{
		PubKey: ctx.Cosmos.PrivKey.PubKey(),
		Data: &sigData,
		Sequence: seq,
	}
	err = txBuilder.SetSignatures(sig)
	if err != nil {
		return
	}

	// Second round: all signer infos are set, so each signer can sign.
	signerData := authsigning.SignerData{
		ChainID:       ctx.Conf.CosmosChainId,
		AccountNumber: ctx.Cosmos.AccountNumber,
		Sequence:      seq,
	}

	sig, err = clienttx.SignWithPrivKey(
		signMode, signerData,
		txBuilder, ctx.Cosmos.PrivKey, ctx.Cosmos.TxConfig, seq)

	if err != nil {
		return
	}

	err = txBuilder.SetSignatures(sig)
	txn := txBuilder.GetTx()
	txBytes, err := ctx.Cosmos.TxConfig.TxEncoder()(txn)
	if err != nil {
		return
	}

	for {
		res, err := ctx.Cosmos.RpcClient.BroadcastTxSync(c.Background(), txBytes)
		if err != nil {
			panic(fmt.Sprintf("failed to broadcast tx with error: %v", err))
		}
		if res.Code != 0 {
			panic(fmt.Sprintf("failed to broadcast tx with non-zero code: %v", res))
		}
		break
	}

	return
}
