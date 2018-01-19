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

		parseTransaction(blockHeight, blockTxVerbose)

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

func parseTransaction(blockHeight int64, txVerbose *btcjson.TxRawResult) {
	fullWallets := []rawWallet{}
	txTimeUnix := time.Now().Unix()
	user := store.User{}
	if isClientMentioned(txVerbose) {
		rawWallets := getUsersByMentionedAddresses(txVerbose)
		for _, rawWallet := range rawWallets {
			fullWallets = append(fullWallets, setTransactionTypeForWallet(rawWallet, blockHeight, txVerbose))
		}

		for _, fullWallet := range fullWallets {

			if fullWallet.TxStatus == store.TxStatusAppearedInMempoolIncoming || fullWallet.TxStatus == store.TxStatusAppearedInBlockIncoming {
				for _, output := range txVerbose.Vout {
					for _, address := range output.ScriptPubKey.Addresses {

						for _, fullWalletAddress := range fullWallet.wallet.Adresses {
							if fullWalletAddress.Address == address {
								//

								query := bson.M{"wallets.addresses.address": address}
								err := usersData.Find(query).One(&user)
								if err != nil {
									log.Errorf("parseTransaction:usersData.Find: %s", err.Error())
								}

								walletIndex := fullWallet.wallet.WalletIndex

								// Update wallets last action time on every new transaction.
								// Set status to OK if some money transfered to this address
								sel := bson.M{"userID": fullWallet.userid, "wallets.walletIndex": fullWallet.wallet.WalletIndex}
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

								sel = bson.M{"userid": user.UserID, "transactions.txid": txVerbose.Txid, "transactions.walletindex": fullWallet.wallet.WalletIndex}
								err = txsData.Find(sel).One(nil)
								if err == mgo.ErrNotFound {
									txOutAmount := int64(100000000 * output.Value)
									// Set bloct time -1 if tx from mempool.
									blockTime := txTimeUnix
									if blockHeight == -1 {
										blockTime = int64(-1)
									}

									newTx := newMultyTX(txVerbose.Txid, txVerbose.Hash, output.ScriptPubKey.Hex, address, fullWallet.TxStatus, int(output.N), walletIndex, txOutAmount, blockTime, blockHeight, fee, txTimeUnix, exRates, inputs, outputs)
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
								sel = bson.M{"userid": user.UserID, "transactions.txid": txVerbose.Txid, "transactions.walletindex": fullWallet.wallet.WalletIndex}
								update = bson.M{
									"$set": bson.M{
										"transactions.$.txstatus":    fullWallet.TxStatus,
										"transactions.$.blockheight": blockHeight,
										"transactions.$.blocktime":   txTimeUnix,
									},
								}

								err = txsData.Update(sel, update)
								if err != nil {
									log.Errorf("parseInput:outputsData.Insert case nil: %s", err.Error())
								}

								//
							}
						}

					}
				}
			}
			if fullWallet.TxStatus == store.TxStatusAppearedInMempoolOutcoming || fullWallet.TxStatus == store.TxStatusAppearedInBlockOutcoming {
				for _, output := range txVerbose.Vout {
					for _, address := range output.ScriptPubKey.Addresses {

						for _, fullWalletAddress := range fullWallet.wallet.Adresses {
							if fullWalletAddress.Address == address {
								//
								for _, input := range txVerbose.Vin {

									previousTxVerbose, err := rawTxByTxid(input.Txid)
									if err != nil {
										log.Errorf("parseInput:rawTxByTxid: %s", err.Error())
										continue
									}

									for _, address := range previousTxVerbose.Vout[input.Vout].ScriptPubKey.Addresses {
										query := bson.M{"wallets.addresses.address": address}
										err := usersData.Find(query).One(&user)
										if err != nil {
											log.Errorf("parseTransaction:usersData.Find: %s", err.Error())
										}

										inputs, outputs, fee, err := txInfo(txVerbose)
										if err != nil {
											log.Errorf("parseInput:txInfo:input: %s", err.Error())
											continue
										}
										exRates, err := GetLatestExchangeRate()
										if err != nil {
											log.Errorf("parseOutput:GetLatestExchangeRate: %s", err.Error())
										}

										walletIndex := fullWallet.wallet.WalletIndex

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
										sel = bson.M{"userid": user.UserID, "transactions.txid": txVerbose.Txid, "transactions.walletindex": fullWallet.wallet.WalletIndex} ///////
										err = txsData.Find(sel).One(nil)
										if err == mgo.ErrNotFound {
											// User have no transaction like this. Add to DB.
											txOutAmount := int64(100000000 * previousTxVerbose.Vout[input.Vout].Value)

											// Set bloct time -1 if tx from mempool.
											blockTime := txTimeUnix
											if blockHeight == -1 {
												blockTime = int64(-1)
											}

											newTx := newMultyTX(txVerbose.Txid, txVerbose.Hash, output.ScriptPubKey.Hex, address, fullWallet.TxStatus, int(output.N), walletIndex, txOutAmount, blockTime, blockHeight, fee, txTimeUnix, exRates, inputs, outputs)
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
										sel = bson.M{"userid": user.UserID, "transactions.txid": txVerbose.Txid, "transactions.walletindex": fullWallet.wallet.WalletIndex}
										update = bson.M{
											"$set": bson.M{
												"transactions.$.txstatus":    fullWallet.TxStatus,
												"transactions.$.blockheight": blockHeight,
												"transactions.$.blocktime":   txTimeUnix,
											},
										}
										err = txsData.Update(sel, update)
										if err != nil {
											log.Errorf("parseInput:txsData.Update: %s", err.Error())
										}
									}
								}
								//
							}
						}

					}
				}
			}
		}
	}

}

func isClientMentioned(txVerbose *btcjson.TxRawResult) bool {
	var flag bool

	for _, output := range txVerbose.Vout {
		for _, address := range output.ScriptPubKey.Addresses {
			query := bson.M{"wallets.addresses.address": address}
			err := usersData.Find(query).One(nil)
			if err != mgo.ErrNotFound {
				continue
			}
			if err != nil && err != mgo.ErrNotFound {
				log.Errorf("parseInput:txsData.Find: %s", err.Error())
				continue
			}
			flag = true
			return flag
		}
	}

	for _, input := range txVerbose.Vin {
		previousTxVerbose, err := rawTxByTxid(input.Txid)
		if err != nil {
			log.Errorf("parseInput:rawTxByTxid: %s", err.Error())
			continue
		}
		for _, address := range previousTxVerbose.Vout[input.Vout].ScriptPubKey.Addresses {
			query := bson.M{"wallets.addresses.address": address}
			err := usersData.Find(query).One(nil)
			if err != mgo.ErrNotFound {
				continue
			}
			if err != nil && err != mgo.ErrNotFound {
				log.Errorf("parseInput:txsData.Find: %s", err.Error())
				continue
			}
			flag = true
			return flag
		}
	}
	return false
}

func getUsersByMentionedAddresses(txVerbose *btcjson.TxRawResult) []rawWallet {
	rawWallets := []rawWallet{}
	user := store.User{}
	for _, output := range txVerbose.Vout {
		for _, address := range output.ScriptPubKey.Addresses {
			query := bson.M{"wallets.addresses.address": address}
			err := usersData.Find(query).One(&user)
			if err != mgo.ErrNotFound {
				continue
			}
			if err != nil && err != mgo.ErrNotFound {
				log.Errorf("parseInput:txsData.Find: %s", err.Error())
				continue
			}

			for _, userWallet := range user.Wallets {
				for _, walletAddress := range userWallet.Adresses {
					if walletAddress.Address == address {
						rawWallets = append(rawWallets, rawWallet{
							userid: user.UserID,
							wallet: userWallet,
						})
					}
				}
			}
		}
	}
	for _, input := range txVerbose.Vin {
		previousTxVerbose, err := rawTxByTxid(input.Txid)
		if err != nil {
			log.Errorf("parseInput:rawTxByTxid: %s", err.Error())
			continue
		}
		for _, address := range previousTxVerbose.Vout[input.Vout].ScriptPubKey.Addresses {
			query := bson.M{"wallets.addresses.address": address}
			err := usersData.Find(query).One(nil)
			if err != mgo.ErrNotFound {
				continue
			}
			if err != nil && err != mgo.ErrNotFound {
				log.Errorf("parseInput:txsData.Find: %s", err.Error())
				continue
			}
			for _, userWallet := range user.Wallets {
				for _, walletAddress := range userWallet.Adresses {
					if walletAddress.Address == address {
						rawWallets = append(rawWallets, rawWallet{
							userid: user.UserID,
							wallet: userWallet,
						})
					}
				}
			}
		}
	}

	return rawWallets
}

type rawWallet struct {
	userid   string
	wallet   store.Wallet
	TxStatus int
}

func setTransactionTypeForWallet(rWallet rawWallet, blockHeight int64, txVerbose *btcjson.TxRawResult) rawWallet {

	for _, input := range txVerbose.Vin {
		previousTxVerbose, err := rawTxByTxid(input.Txid)
		if err != nil {
			log.Errorf("parseInput:rawTxByTxid: %s", err.Error())
			continue
		}
		for _, address := range previousTxVerbose.Vout[input.Vout].ScriptPubKey.Addresses {
			for _, walletAddress := range rWallet.wallet.Adresses {
				if address == walletAddress.Address {
					if blockHeight == -1 {
						rWallet.TxStatus = store.TxStatusAppearedInMempoolOutcoming
					}
					if blockHeight != -1 {
						rWallet.TxStatus = store.TxStatusAppearedInBlockOutcoming
					}
				}
			}
		}
	}

	for _, output := range txVerbose.Vout {
		for _, address := range output.ScriptPubKey.Addresses {
			for _, walletAddress := range rWallet.wallet.Adresses {
				if address == walletAddress.Address {
					if blockHeight == -1 {
						rWallet.TxStatus = store.TxStatusAppearedInMempoolIncoming
					}
					if blockHeight != -1 {
						rWallet.TxStatus = store.TxStatusAppearedInBlockIncoming
					}
				}
			}
		}
	}

	return rWallet
}
