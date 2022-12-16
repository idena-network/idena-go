package blockchain

// TODO uncomment this test in upgrade 11 (contracts)
//func Test_blockGasLimit(t *testing.T) {
//	key1, _ := crypto.GenerateKey()
//	addr1 := crypto.PubkeyToAddress(key1.PublicKey)
//
//	key2, _ := crypto.GenerateKey()
//	addr2 := crypto.PubkeyToAddress(key2.PublicKey)
//
//	key3, _ := crypto.GenerateKey()
//	addr3 := crypto.PubkeyToAddress(key3.PublicKey)
//
//	balance := new(big.Int).Mul(big.NewInt(1e+18), big.NewInt(999999999))
//	consensusCfg := GetDefaultConsensusConfig()
//	consensusCfg.Automine = true
//	consensusCfg.StatusSwitchRange = 10
//	consensusCfg.EnableUpgrade10 = true
//	cfg := &config.Config{
//		Network:   0x99,
//		Consensus: consensusCfg,
//		GenesisConf: &config.GenesisConf{
//			Alloc: map[common.Address]config.GenesisAllocation{
//				addr1: {
//					State:   uint8(state.Verified),
//					Balance: balance,
//				},
//			},
//			GodAddress:        addr1,
//			FirstCeremonyTime: 4070908800, //01.01.2099
//		},
//		Validation: &config.ValidationConfig{},
//		Blockchain: &config.BlockchainConfig{},
//	}
//	cfg.Mempool = config.GetDefaultMempoolConfig()
//	cfg.Mempool.TxPoolAddrQueueLimit = 256
//	cfg.Mempool.TxPoolAddrExecutableLimit = 256
//	chain, appState := NewCustomTestBlockchainWithConfig(5, 0, key1, cfg)
//	defer chain.SecStore().Destroy()
//
//	validation.SetAppConfig(chain.config)
//	defer validation.SetAppConfig(nil)
//
//	chain.GenerateBlocks(10, 0)
//
//	{
//		tx, err := types.SignTx(BuildTx(appState, addr1, &addr2, types.SendTx, decimal.RequireFromString("100000"), decimal.RequireFromString("500"), decimal.Zero, 0, 0, nil), key1)
//		require.NoError(t, err)
//		require.NoError(t, chain.txpool.AddInternalTx(tx))
//		chain.GenerateBlocks(1, 0)
//		require.Equal(t, "100000", ConvertToFloat(appState.State.GetBalance(addr2)).String())
//	}
//
//	chain.GenerateBlocks(10, 0)
//
//	{
//		var deployContractTxHash common.Hash
//		{
//			var data [][]byte
//			data = append(data, common.ToBytes(byte(2))) // maxVotes
//			data = append(data, common.ToBytes(byte(2))) // minVotes
//			attachment := attachments.CreateDeployContractAttachment(embedded.MultisigContract, data...)
//			payload, err := attachment.ToBytes()
//			require.NoError(t, err)
//			tx, err := types.SignTx(BuildTx(appState, addr1, nil, types.DeployContractTx, decimal.RequireFromString("99999999"), decimal.New(51200, 0), decimal.Zero, 0, 0, payload), key1)
//			require.NoError(t, err)
//			require.NoError(t, chain.txpool.AddInternalTx(tx))
//
//			deployContractTxHash = tx.Hash()
//		}
//
//		for i := 0; i < 15; i++ {
//			payload := random.GetRandomBytes(32 * 1024)
//			tx, err := types.SignTx(BuildTx(appState, addr2, &addr3, types.SendTx, decimal.RequireFromString("1"), decimal.RequireFromString("4000"), decimal.Zero, 0, 0, payload), key2)
//			require.NoError(t, err)
//			require.NoError(t, chain.txpool.AddInternalTx(tx))
//		}
//
//		{
//			payload := random.GetRandomBytes(17*1024 + 883)
//			tx, err := types.SignTx(BuildTx(appState, addr2, &addr3, types.SendTx, decimal.RequireFromString("1"), decimal.RequireFromString("4000"), decimal.Zero, 0, 0, payload), key2)
//			require.NoError(t, err)
//			require.NoError(t, chain.txpool.AddInternalTx(tx))
//		}
//
//		for i := 0; i < 1; i++ {
//			var payload []byte
//			tx, err := types.SignTx(BuildTx(appState, addr2, &addr3, types.SendTx, decimal.RequireFromString("1"), decimal.RequireFromString("4000"), decimal.Zero, 0, 0, payload), key2)
//			require.NoError(t, err)
//			require.NoError(t, chain.txpool.AddInternalTx(tx))
//		}
//
//		require.Len(t, chain.txpool.BuildBlockTransactions(), 18)
//
//		chain.GenerateBlocks(1, 0)
//
//		receipt := chain.GetReceipt(deployContractTxHash)
//		require.NotNil(t, receipt)
//		require.True(t, receipt.Success)
//
//		require.Equal(t, "16", ConvertToFloat(appState.State.GetBalance(addr3)).String())
//
//		chain.GenerateBlocks(1, 0)
//
//		require.Equal(t, "17", ConvertToFloat(appState.State.GetBalance(addr3)).String())
//	}
//}
