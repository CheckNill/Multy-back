/*
Copyright 2017 Idealnaya rabota LLC
Licensed under Multy.io license.
See LICENSE for details
*/
package btc

import (
	"encoding/json"
	"fmt"
	"time"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/Appscrunch/Multy-back/store"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

const ( // currency id  nsq
	TxStatusAppearedInMempoolIncoming = 1
	TxStatusAppearedInBlockIncoming   = 2

	TxStatusAppearedInMempoolOutcoming = 3
	TxStatusAppearedInBlockOutcoming   = 4

	TxStatusInBlockConfirmedIncoming  = 5
	TxStatusInBlockConfirmedOutcoming = 6

	// TxStatusInBlockConfirmed = 5

	// TxStatusRejectedFromBlock = -1
)

const (
	SixBlockConfirmation     = 6
	SixPlusBlockConfirmation = 7
)

func notifyNewBlockTx(hash *chainhash.Hash) {
	log.Debugf("New block connected %s", hash.String())

	// block Height
	// blockVerbose, err := rpcClient.GetBlockVerbose(hash)
	// blockHeight := blockVerbose.Height

	//parse all block transactions
	rawBlock, err := rpcClient.GetBlock(hash)
	allBlockTransactions, err := rawBlock.TxHashes()
	if err != nil {
		log.Errorf("parseNewBlock:rawBlock.TxHashes: %s", err.Error())
	}

	var user store.User

	// range over all block txID's and notify clients about including their transaction in block as input or output
	// delete by transaction hash record from mempool db to estimete tx speed
	for _, txHash := range allBlockTransactions {

		blockTxVerbose, err := rpcClient.GetRawTransactionVerbose(&txHash)
		if err != nil {
			log.Errorf("parseNewBlock:rpcClient.GetRawTransactionVerbose: %s", err.Error())
			continue
		}

		// delete all block transations from memPoolDB
		query := bson.M{"hashtx": blockTxVerbose.Txid}
		err = mempoolRates.Remove(query)
		if err != nil {
			log.Errorf("parseNewBlock:mempoolRates.Remove: %s", err.Error())
		} else {
			log.Debugf("Tx removed: %s", blockTxVerbose.Txid)
		}

		// parse block tx outputs and notify
		for _, out := range blockTxVerbose.Vout {
			for _, address := range out.ScriptPubKey.Addresses {

				query := bson.M{"wallets.addresses.address": address}
				err := usersData.Find(query).One(&user)
				if err != nil {
					continue
				}
				log.Debugf("[IS OUR USER] parseNewBlock: usersData.Find = %s", address)

				txMsq := BtcTransactionWithUserID{
					UserID: user.UserID,
					NotificationMsg: &BtcTransaction{
						TransactionType: TxStatusAppearedInBlockIncoming,
						Amount:          out.Value,
						TxID:            blockTxVerbose.Txid,
						Address:         address,
					},
				}
				sendNotifyToClients(&txMsq)

			}
		}

		// parse block tx inputs and notify
		for _, input := range blockTxVerbose.Vin {
			txHash, err := chainhash.NewHashFromStr(input.Txid)
			if err != nil {
				log.Errorf("parseNewBlock: chainhash.NewHashFromStr = %s", err)
			}
			previousTx, err := rpcClient.GetRawTransactionVerbose(txHash)
			if err != nil {
				log.Errorf("parseNewBlock:rpcClient.GetRawTransactionVerbose: %s ", err.Error())
				continue
			}

			for _, out := range previousTx.Vout {
				for _, address := range out.ScriptPubKey.Addresses {
					query := bson.M{"wallets.addresses.address": address}
					err := usersData.Find(query).One(&user)
					if err != nil {
						continue
					}
					log.Debugf("[IS OUR USER]-AS-OUT parseMempoolTransaction: usersData.Find = %s", address)

					txMsq := BtcTransactionWithUserID{
						UserID: user.UserID,
						NotificationMsg: &BtcTransaction{
							// currency id
							TransactionType: TxStatusAppearedInBlockOutcoming,
							Amount:          out.Value,
							TxID:            blockTxVerbose.Txid,
							Address:         address,
						},
					}
					sendNotifyToClients(&txMsq)
				}
			}
		}
	}
}

func sendNotifyToClients(txMsq *BtcTransactionWithUserID) {
	newTxJSON, err := json.Marshal(txMsq)
	if err != nil {
		log.Errorf("sendNotifyToClients: [%+v] %s\n", txMsq, err.Error())
		return
	}

	err = nsqProducer.Publish(TopicTransaction, newTxJSON)
	if err != nil {
		log.Errorf("nsq publish new transaction: [%+v] %s\n", txMsq, err.Error())
		return
	}
	return
}

func blockTransactions(hash *chainhash.Hash) {
	log.Debugf("New block connected %s", hash.String())

	// block Height
	blockVerbose, err := rpcClient.GetBlockVerbose(hash)
	blockHeight := blockVerbose.Height

	//parse all block transactions
	rawBlock, err := rpcClient.GetBlock(hash)
	allBlockTransactions, err := rawBlock.TxHashes()
	if err != nil {
		log.Errorf("parseNewBlock:rawBlock.TxHashes: %s", err.Error())
	}

	for _, txHash := range allBlockTransactions {

		blockTxVerbose, err := rpcClient.GetRawTransactionVerbose(&txHash)
		if err != nil {
			log.Errorf("parseNewBlock:rpcClient.GetRawTransactionVerbose: %s", err.Error())
			continue
		}

		// parseTransaction(blockHeight, blockTxVerbose)

		// // apear as output
		// err = parseOutput(blockTxVerbose, blockHeight, TxStatusAppearedInBlockIncoming)
		// if err != nil {
		// 	log.Errorf("parseNewBlock:parseOutput: %s", err.Error())
		// }

		// // apear as input
		// err = parseInput(blockTxVerbose, blockHeight, TxStatusAppearedInBlockOutcoming)
		// if err != nil {
		// 	log.Errorf("parseNewBlock:parseInput: %s", err.Error())
		// }
	}
}

func parseOutput(txVerbose *btcjson.TxRawResult, blockHeight int64, txStatus int) error {
	user := store.User{}
	blockTimeUnix := time.Now().Unix()

	for _, output := range txVerbose.Vout {
		for _, address := range output.ScriptPubKey.Addresses {
			query := bson.M{"wallets.addresses.address": address}
			err := usersData.Find(query).One(&user)
			if err != nil {
				continue
				// is not our user
			}
			fmt.Println("[ITS OUR USER] ", user.UserID)

			walletIndex := fetchWalletIndex(user.Wallets, address)

			// Update wallets last action time on every new transaction.
			// Set status to OK if some money transfered to this address
			sel := bson.M{"userID": user.UserID, "wallets.walletIndex": walletIndex}
			update := bson.M{
				"$set": bson.M{
					"wallets.$.status":         store.WalletStatusOK,
					"wallets.$.lastActionTime": time.Now().Unix(),
				},
			}
			err = usersData.Update(sel, update)
			if err != nil {
				log.Errorf("parseOutput:restClient.userStore.Update: %s", err.Error())
			}

			// Update address last action time on every new transaction.
			sel = bson.M{"userID": user.UserID, "wallets.addresses.address": address}
			update = bson.M{
				"$set": bson.M{
					"wallets.$.addresses.$[].lastActionTime": time.Now().Unix(),
				},
			}
			err = usersData.Update(sel, update)
			if err != nil {
				log.Errorf("parseOutput:restClient.userStore.Update: %s", err.Error())
			}

			inputs, outputs, fee, err := txInfo(txVerbose)
			if err != nil {
				log.Errorf("parseInput:txInfo:output: %s", err.Error())
				continue
			}

			// Get latest exchange rates from db.
			exRates, err := GetLatestExchangeRate()
			if err != nil {
				log.Errorf("parseOutput:GetLatestExchangeRate: %s", err.Error())
			}

			sel = bson.M{"userid": user.UserID, "transactions.txid": txVerbose.Txid, "transactions.txaddress": address}
			err = txsData.Find(sel).One(nil)
			if err == mgo.ErrNotFound {
				txOutAmount := int64(100000000 * output.Value)

				// Set bloct time -1 if tx from mempool.
				blockTime := blockTimeUnix
				if blockHeight == -1 {
					blockTime = int64(-1)
				}

				newTx := newMultyTX(txVerbose.Txid, txVerbose.Hash, output.ScriptPubKey.Hex, address, txStatus, int(output.N), walletIndex, txOutAmount, blockTime, blockHeight, fee, blockTimeUnix, exRates, inputs, outputs)
				sel = bson.M{"userid": user.UserID}
				update := bson.M{"$push": bson.M{"transactions": newTx}}
				err = txsData.Update(sel, update)
				if err != nil {
					log.Errorf("parseInput.Update add new tx to user: %s", err.Error())
				}
				continue
			} else if err != nil && err != mgo.ErrNotFound {
				log.Errorf("parseInput:txsData.Find: %s", err.Error())
				continue
			}

			// User have this transaction but with another status.
			// Update statsus, block height and block time.
			sel = bson.M{"userid": user.UserID, "transactions.txid": txVerbose.Txid, "transactions.txaddress": address}
			update = bson.M{
				"$set": bson.M{
					"transactions.$.txstatus":    txStatus,
					"transactions.$.blockheight": blockHeight,
					"transactions.$.blocktime":   blockTimeUnix,
				},
			}

			err = txsData.Update(sel, update)
			if err != nil {
				log.Errorf("parseInput:outputsData.Insert case nil: %s", err.Error())
			}
		}
	}
	return nil
}

func parseInput(txVerbose *btcjson.TxRawResult, blockHeight int64, txStatus int) error {
	user := store.User{}
	blockTimeUnix := time.Now().Unix()

	for _, input := range txVerbose.Vin {

		previousTxVerbose, err := rawTxByTxid(input.Txid)
		if err != nil {
			log.Errorf("parseInput:rawTxByTxid: %s", err.Error())
			continue
		}

		for _, address := range previousTxVerbose.Vout[input.Vout].ScriptPubKey.Addresses {
			query := bson.M{"wallets.addresses.address": address}
			// Is it's our user transaction.
			err := usersData.Find(query).One(&user)
			if err != nil {
				continue
				// Is not our user.
			}

			log.Debugf("[ITS OUR USER] %s", user.UserID)

			inputs, outputs, fee, err := txInfo(txVerbose)
			if err != nil {
				log.Errorf("parseInput:txInfo:input: %s", err.Error())
				continue
			}
			exRates, err := GetLatestExchangeRate()
			if err != nil {
				log.Errorf("parseOutput:GetLatestExchangeRate: %s", err.Error())
			}

			walletIndex := fetchWalletIndex(user.Wallets, address)

			// Update wallets last action time on every new transaction.
			sel := bson.M{"userID": user.UserID, "wallets.walletIndex": walletIndex}
			update := bson.M{
				"$set": bson.M{
					"wallets.$.lastActionTime": time.Now().Unix(),
				},
			}
			err = usersData.Update(sel, update)
			if err != nil {
				log.Errorf("parseOutput:restClient.userStore.Update: %s", err.Error())
			}

			// Update address last action time on every new transaction.
			sel = bson.M{"userID": user.UserID, "wallets.addresses.address": address}
			update = bson.M{
				"$set": bson.M{
					"wallets.$.addresses.$[].lastActionTime": time.Now().Unix(),
				},
			}
			err = usersData.Update(sel, update)
			if err != nil {
				log.Errorf("parseOutput:restClient.userStore.Update: %s", err.Error())
			}

			// Is our user already have this transactions.
			sel = bson.M{"userid": user.UserID, "transactions.txid": txVerbose.Txid, "transactions.txaddress": address}
			err = txsData.Find(sel).One(nil)
			if err == mgo.ErrNotFound {
				// User have no transaction like this. Add to DB.
				txOutAmount := int64(100000000 * previousTxVerbose.Vout[input.Vout].Value)

				// Set bloct time -1 if tx from mempool.
				blockTime := blockTimeUnix
				if blockHeight == -1 {
					blockTime = int64(-1)
				}

				newTx := newMultyTX(txVerbose.Txid, txVerbose.Hash, previousTxVerbose.Vout[input.Vout].ScriptPubKey.Hex, address, txStatus, int(previousTxVerbose.Vout[input.Vout].N), walletIndex, txOutAmount, blockTime, blockHeight, fee, blockTimeUnix, exRates, inputs, outputs)
				sel = bson.M{"userid": user.UserID}
				update := bson.M{"$push": bson.M{"transactions": newTx}}
				err = txsData.Update(sel, update)
				if err != nil {
					log.Errorf("parseInput:txsData.Update add new tx to user: %s", err.Error())
				}
				continue
			} else if err != nil && err != mgo.ErrNotFound {
				log.Errorf("parseInput:txsData.Find: %s", err.Error())
				continue
			}

			// User have this transaction but with another status.
			// Update statsus, block height and block time.
			sel = bson.M{"userid": user.UserID, "transactions.txid": txVerbose.Txid, "transactions.txaddress": address}
			update = bson.M{
				"$set": bson.M{
					"transactions.$.txstatus":    txStatus,
					"transactions.$.blockheight": blockHeight,
					"transactions.$.blocktime":   blockTimeUnix,
				},
			}
			err = txsData.Update(sel, update)
			if err != nil {
				log.Errorf("parseInput:txsData.Update: %s", err.Error())
			}
		}
	}
	return nil
}

func GetLatestExchangeRate() ([]store.ExchangeRatesRecord, error) {
	selGdax := bson.M{
		"stockexchange": "Gdax",
	}
	selPoloniex := bson.M{
		"stockexchange": "Poloniex",
	}
	stocksGdax := store.ExchangeRatesRecord{}
	err := exRate.Find(selGdax).Sort("-timestamp").One(&stocksGdax)
	if err != nil {
		return nil, err
	}

	stocksPoloniex := store.ExchangeRatesRecord{}
	err = exRate.Find(selPoloniex).Sort("-timestamp").One(&stocksPoloniex)
	if err != nil {
		return nil, err
	}
	return []store.ExchangeRatesRecord{stocksPoloniex, stocksGdax}, nil

}

func blockConfirmations(hash *chainhash.Hash) {
	blockVerbose, err := rpcClient.GetBlockVerbose(hash)
	blockHeight := blockVerbose.Height

	sel := bson.M{"transactions.txstatus": TxStatusAppearedInBlockIncoming, "transactions.blockheight": bson.M{"$lte": blockHeight - SixBlockConfirmation, "$gte": blockHeight - SixPlusBlockConfirmation}}
	update := bson.M{
		"$set": bson.M{
			"transactions.$.txstatus": TxStatusInBlockConfirmedIncoming,
		},
	}
	err = txsData.Update(sel, update)
	if err != nil {
		log.Errorf("blockConfirmations:txsData.Update: %s", err.Error())
	}

	sel = bson.M{"transactions.txstatus": TxStatusAppearedInBlockOutcoming, "transactions.blockheight": bson.M{"$lte": blockHeight - SixBlockConfirmation, "$gte": blockHeight - SixPlusBlockConfirmation}}
	update = bson.M{
		"$set": bson.M{
			"transactions.$.txstatus": TxStatusInBlockConfirmedOutcoming,
		},
	}
	err = txsData.Update(sel, update)
	if err != nil {
		log.Errorf("blockConfirmations:txsData.Update: %s", err.Error())
	}

	query := bson.M{"transactions.blockheight": blockHeight + SixBlockConfirmation}

	var records []store.TxRecord
	txsData.Find(query).All(&records)
	for _, usertxs := range records {

		txMsq := BtcTransactionWithUserID{
			UserID: usertxs.UserID,
			NotificationMsg: &BtcTransaction{
				TransactionType: TxStatusInBlockConfirmedIncoming,
			},
		}
		sendNotifyToClients(&txMsq)
	}

}

func parseTransaction(rawTransaction *btcjson.TxRawResult, blockHeight int64) {
	if isOurUser(rawTransaction) {
		btcTx := toFullTx(rawTransaction, blockHeight)
		allInputs := []string{}
		allOutputs := []string{}
		for _, input := range btcTx.TxInputs {
			allInputs = append(allInputs, input.Address)
		}
		for _, output := range btcTx.TxOutputs {
			allOutputs = append(allOutputs, output.Address)
		}

		allAddresses := append(allInputs, allOutputs...)

		sel := bson.M{
			"wallets.addresses.address": bson.M{
				"$in": allAddresses,
			},
		}

		users := []store.User{}
		err := usersData.Find(sel).All(&users)
		if err != nil {
			log.Errorf("parseTransaction:usersData.Find %s", err.Error)
		}

		for _, user := range users {
			for _, wallet := range user.Wallets {
				for _, address := range wallet.Adresses {

					for _, input := range allInputs {
						if input == address.Address {

							for _, userTX := range user.BtcTransactions {
								if userTX.TxID == btcTx.TxID {
									// update
								}
								//create new

							}

						}
					}

					for _, output := range allOutputs {
						if output == address.Address {

						}
					}

				}
			}
		}

	}
}
func isOurUser(rawTransaction *btcjson.TxRawResult) bool {
	for _, out := range rawTransaction.Vout {
		for _, address := range out.ScriptPubKey.Addresses {
			query := bson.M{"wallets.addresses.address": address}
			err := usersData.Find(query).One(nil)
			if err != nil {
				continue
			}
			return true
		}
	}
	for _, input := range rawTransaction.Vin {
		txHash, err := chainhash.NewHashFromStr(input.Txid)
		if err != nil {
			log.Errorf("parseNewBlock: chainhash.NewHashFromStr = %s", err)
		}
		previousTx, err := rpcClient.GetRawTransactionVerbose(txHash)
		if err != nil {
			log.Errorf("parseNewBlock:rpcClient.GetRawTransactionVerbose: %s ", err.Error())
			continue
		}
		for _, out := range previousTx.Vout {
			for _, address := range out.ScriptPubKey.Addresses {
				query := bson.M{"wallets.addresses.address": address}
				err := usersData.Find(query).One(nil)
				if err != nil {
					continue
				}
				return true
			}
		}
	}
	return false
}

func toFullTx(rawTransaction *btcjson.TxRawResult, blockHeight int64) BTCMultyTx {
	inputs, outputs, fee, err := txInfo(rawTransaction)
	if err != nil {
		log.Errorf("toFullTx:txInfo: %s", err.Error())
	}

	exRates, err := GetLatestExchangeRate()
	if err != nil {
		log.Errorf("toFullTx:GetLatestExchangeRate: %s", err.Error())
	}

	blockTime := time.Now().Unix()
	if blockHeight == -1 {
		blockTime = -1
	}

	mempoolTime := time.Now().Unix() // check block

	return newBTCMultyTx(rawTransaction.Txid, rawTransaction.Hash, blockTime, blockHeight, fee, mempoolTime, exRates, inputs, outputs)
}

type BTCMultyTx struct {
	TxID              string                      `json:"txid"`
	TxHash            string                      `json:"txhash"`
	BlockTime         int64                       `json:"blocktime"`
	BlockHeight       int64                       `json:"blockheight"`
	TxFee             int64                       `json:"txfee"`
	MempoolTime       int64                       `json:"mempooltime"`
	StockExchangeRate []store.ExchangeRatesRecord `json:"stockexchangerate"`
	TxInputs          []store.AddresAmount        `json:"txinputs"`
	TxOutputs         []store.AddresAmount        `json:"txoutputs"`
}

type BTCMultyUserTx struct {
	BtcTx       BTCMultyTx `json:"btctx"`
	TxAddress   string     `json:"address"`
	TxOutScript string     `json:"txoutscript"`
	TxStatus    int        `json:"txstatus"`
	WalletIndex int        `json:"walletindex"`
	TxOutID     int        `json:"txoutid"`
	TxOutAmount int64      `json:"txoutamount"`
}

func newBTCMultyTx(txid, txhash string, blocktime, blockheight, txfee, mempooltime int64, stockexchangerate []store.ExchangeRatesRecord, inputs, outputs []store.AddresAmount) BTCMultyTx {
	return BTCMultyTx{
		TxID:              txid,
		TxHash:            txhash,
		BlockTime:         blocktime,
		BlockHeight:       blockheight,
		TxFee:             txfee,
		MempoolTime:       mempooltime,
		StockExchangeRate: stockexchangerate,
		TxInputs:          inputs,
		TxOutputs:         outputs,
	}
}

func newBTCMultyUserTx(btctx BTCMultyTx) BTCMultyUserTx {
	return BTCMultyUserTx{
		BtcTx: btctx,
	}
}
