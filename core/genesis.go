// Copyright 2014 The The go-taichain Authors
// This file is part of The go-taichain library.
//
// The go-taichain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-taichain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with The go-taichain library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/taichain/go-taichain/core/types/devotedb"
	"github.com/taichain/go-taichain/crypto"
	"github.com/taichain/go-taichain/p2p/enode"
	"github.com/taichain/go-taichain/trie"
	"math/big"
	"strings"

	"github.com/taichain/go-taichain/common"
	"github.com/taichain/go-taichain/common/hexutil"
	"github.com/taichain/go-taichain/common/math"
	"github.com/taichain/go-taichain/core/rawdb"
	"github.com/taichain/go-taichain/core/state"
	"github.com/taichain/go-taichain/core/types"
	"github.com/taichain/go-taichain/ethdb"
	"github.com/taichain/go-taichain/log"
	"github.com/taichain/go-taichain/params"
	"github.com/taichain/go-taichain/rlp"
)

//go:generate gencodec -type Genesis -field-override genesisSpecMarshaling -out gen_genesis.go
//go:generate gencodec -type GenesisAccount -field-override genesisAccountMarshaling -out gen_genesis_account.go

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config     *params.ChainConfig `json:"config"`
	Nonce      uint64              `json:"nonce"`
	Timestamp  uint64              `json:"timestamp"`
	ExtraData  []byte              `json:"extraData"`
	GasLimit   uint64              `json:"gasLimit"   gencodec:"required"`
	Difficulty *big.Int            `json:"difficulty" gencodec:"required"`
	Mixhash    common.Hash         `json:"mixHash"`
	Coinbase   common.Address      `json:"coinbase"`
	StateRoot  common.Hash         `json:"stateRoot"`
	Alloc      GenesisAlloc        `json:"alloc"      gencodec:"required"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis blocks.
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[common.Address]GenesisAccount

func (ga *GenesisAlloc) UnmarshalJSON(data []byte) error {
	m := make(map[common.UnprefixedAddress]GenesisAccount)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*ga = make(GenesisAlloc)
	for addr, a := range m {
		(*ga)[common.Address(addr)] = a
	}
	return nil
}

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte                      `json:"code,omitempty"`
	Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance    *big.Int                    `json:"balance" gencodec:"required"`
	Nonce      uint64                      `json:"nonce,omitempty"`
	PrivateKey []byte                      `json:"secretKey,omitempty"` // for tests
}

// field type overrides for gencodec
type genesisSpecMarshaling struct {
	Nonce      math.HexOrDecimal64
	Timestamp  math.HexOrDecimal64
	ExtraData  hexutil.Bytes
	GasLimit   math.HexOrDecimal64
	GasUsed    math.HexOrDecimal64
	Number     math.HexOrDecimal64
	Difficulty *math.HexOrDecimal256
	Alloc      map[common.UnprefixedAddress]GenesisAccount
}

type genesisAccountMarshaling struct {
	Code       hexutil.Bytes
	Balance    *math.HexOrDecimal256
	Nonce      math.HexOrDecimal64
	Storage    map[storageJSON]storageJSON
	PrivateKey hexutil.Bytes
}

// storageJSON represents a 256 bit byte array, but allows less than 256 bits when
// unmarshaling from hex.
type storageJSON common.Hash

func (h *storageJSON) UnmarshalText(text []byte) error {
	text = bytes.TrimPrefix(text, []byte("0x"))
	if len(text) > 64 {
		return fmt.Errorf("too many hex characters in storage key/value %q", text)
	}
	offset := len(h) - len(text)/2 // pad on the left
	if _, err := hex.Decode(h[offset:], text); err != nil {
		fmt.Println(err)
		return fmt.Errorf("invalid hex storage key/value %q", text)
	}
	return nil
}

func (h storageJSON) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

// GenesisMismatchError is raised when trying to overwrite an existing
// genesis block with an incompatible one.
type GenesisMismatchError struct {
	Stored, New common.Hash
}

func (e *GenesisMismatchError) Error() string {
	return fmt.Sprintf("database already contains an incompatible genesis block (have %x, new %x)", e.Stored[:8], e.New[:8])
}

// SetupGenesisBlock writes or updates the genesis block in db.
// The block that will be used is:
//
//                          genesis == nil       genesis != nil
//                       +------------------------------------------
//     db has no genesis |  main-net default  |  genesis
//     db has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork block below the local head block). In case of a conflict, the
// error is a *params.ConfigCompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisBlock(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {
		return params.DevoteChainConfig, common.Hash{}, errGenesisNoConfig
	}
	// Just commit the new block if there is no stored genesis block.
	stored := rawdb.ReadCanonicalHash(db, params.GenesisBlockNumber)
	if (stored == common.Hash{}) {
		if genesis == nil {
			log.Info("Writing default main-net genesis block")
			genesis = DefaultGenesisBlock()
			root, err := genesisAccounts(common.Hash{}, db)
			if err != nil {
				return params.DevoteChainConfig, common.Hash{}, err
			}
			genesis.StateRoot = root
		} else {
			log.Info("Writing custom genesis block")
		}
		block, err := genesis.Commit(db)
		return genesis.Config, block.Hash(), err
	}
	// Check whether the genesis block is already written.
	if genesis != nil {
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}
	// Get the existing chain configuration.
	newcfg := genesis.configOrDefault(stored)
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)

		return newcfg, stored, nil
	}
	// Special case: don't change the existing config of a non-mainnet chain if no new
	// config is supplied. These chains would get AllProtocolChanges (and a compat error)
	// if we just continued here.
	if genesis == nil && stored != params.MainnetGenesisHash {
		return storedcfg, stored, nil
	}
	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {

		return newcfg, stored, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {

		return newcfg, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newcfg)
	return newcfg, stored, nil
}

func (g *Genesis) configOrDefault(ghash common.Hash) *params.ChainConfig {
	switch {
	case g != nil:
		return g.Config
	default:
		return params.DevoteChainConfig
	}
}

// ToBlock creates the genesis block and writes state of a genesis specification
// to the given database (or discards it if nil).
func (g *Genesis) ToBlock(db ethdb.Database) *types.Block {
	if db == nil {
		db = ethdb.NewMemDatabase()
	}

	statedb, _ := state.New(g.StateRoot, state.NewDatabase(db))
	for addr, account := range g.Alloc {
		statedb.AddBalance(addr, account.Balance, big.NewInt(1))
		statedb.SetCode(addr, account.Code)
		statedb.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			statedb.SetState(addr, key, value)
		}
	}
	root := statedb.IntermediateRoot(false)

	// add devote protocol
	devoteDB := initGenesisDevoteProtocol(g, db)
	// add devote protocol
	protcol, _ := devoteDB.Commit()

	head := &types.Header{
		Number:     new(big.Int).SetUint64(g.Number),
		Nonce:      types.EncodeNonce(g.Nonce),
		Time:       g.Timestamp,
		ParentHash: g.ParentHash,
		Extra:      g.ExtraData,
		GasLimit:   g.GasLimit,
		GasUsed:    g.GasUsed,
		Difficulty: g.Difficulty,
		MixDigest:  g.Mixhash,
		Coinbase:   g.Coinbase,
		Root:       root,
		Protocol:   protcol,
	}
	if g.GasLimit == 0 {
		head.GasLimit = params.GenesisGasLimit
	}
	if g.Difficulty == nil {
		head.Difficulty = params.GenesisDifficulty
	}
	statedb.Commit(false)
	statedb.Database().TrieDB().Commit(root, false)
	block := types.NewBlock(head, nil, nil, nil)

	return block
}

// Commit writes the block and state of a genesis specification to the database.
// The block is committed as the canonical head block.
func (g *Genesis) Commit(db ethdb.Database) (*types.Block, error) {
	block := g.ToBlock(db)
	if block.NumberU64() != params.GenesisBlockNumber {
		return nil, fmt.Errorf("can't commit genesis block with number != %d", params.GenesisBlockNumber)
	}
	rawdb.WriteTd(db, block.Hash(), block.NumberU64(), g.Difficulty)
	rawdb.WriteBlock(db, block)
	rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), nil)
	rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())

	config := g.Config
	if config == nil {
		config = params.AllEthashProtocolChanges
	}
	rawdb.WriteChainConfig(db, block.Hash(), config)
	return block, nil
}

// MustCommit writes the genesis block and state to db, panicking on error.
// The block is committed as the canonical head block.
func (g *Genesis) MustCommit(db ethdb.Database) *types.Block {
	block, err := g.Commit(db)
	if err != nil {
		panic(err)
	}
	return block
}

// GenesisBlockForTesting creates and writes a block in which addr has the given wei balance.
func GenesisBlockForTesting(db ethdb.Database, addr common.Address, balance *big.Int) *types.Block {
	g := Genesis{Alloc: GenesisAlloc{addr: {Balance: balance}}}
	return g.MustCommit(db)
}

// DefaultGenesisBlock returns the Ethereum main net genesis block.
func DefaultGenesisBlock() *Genesis {
	alloc := decodePrealloc(mainnetAllocData)
	configMainnet := params.DevoteChainConfig
	configMainnet.Devote.Witnesses = params.MainnetInitIds
	alloc[common.BytesToAddress(params.MasterndeContractAddress.Bytes())] = masternodeContractAccount(params.TestnetMasternodes)
	// record in the contracts
	// test net accounts
	alloc[common.HexToAddress("0x2ccd4e66af1c8041dea40bd95a9be81dacc94654")] = GenesisAccount{
		Balance: new(big.Int).Mul(big.NewInt(5.5e+15), big.NewInt(1e+9)),
	}
	accs := []string{
		"0x53fb21daf47c00d6ad517582554f95c4d5899823",
		"0x297e0601a7a10320fb5be1b595bb4d1a31fbf663",
		"0x5cae1c239d1e9cd89be5f758f270c81491e77d1d",
		"0x27af23f7ed60d8897fc951bec7b0bf3555ae3020",
		"0x7722ed77666fcb2ceb0d5c2335d09cc165929182",
		"0x53a74daa9f3fb4c981bf86603d8ca0029315a513",
		"0xf5b8a7d52c8386f414d12bff694e940bc622e99d",
		"0x463f9f5e123b8db03459edbb4e8542c80307fac1",
		"0x6db13c33fbdd1daf94202a6c4f8530849fb0b422",
		"0x2667b045633d986cf37e29504d7bd6531a64fc68",
		"0x5de267f959c75a1e46d6e0ba4ddfe4306542006a",
		"0xe7de925e9e8a98cfa2c1521968012085c2ea7788",
		"0xb785076a94542007958df2bbfba25f582c7c70e0",
		"0xd9298d4a0ace899cbbec399c7df860397fcd8b6d",
		"0x08e3a9f92d51ee3bfa04591e1a03d107f5fe3acd",
		"0xd41eed8a85480dc0b1ed5fc96d9082b358d8ae95",
		"0x88a453b8fc2aada8b26dd3aa5e9af19abca0fe97",
		"0x055613727695d2fd6b1264a46854af2475d6d914",
		"0x3c31e4918cc1aa92347d9b8912df4852701602d2",
		"0x23bffe0d45bb73e8d25ef4e148345a32f2f1d2fc",
		"0x10b32cc0410dec2fe0373c621ed75220ee03d845",
	}
	for _, v := range accs {
		alloc[common.HexToAddress(v)] = GenesisAccount{
			Balance: new(big.Int).Mul(big.NewInt(1e+3), big.NewInt(1e+15)),
		}
	}

	return &Genesis{
		Config:     configMainnet,
		Nonce:      1,
		Timestamp:  1566225000,
		GasLimit:   10000000,
		Difficulty: big.NewInt(1),
		Alloc:      alloc,
		Number:     params.GenesisBlockNumber,
	}
}

// DefaultTestnetGenesisBlock returns the Ropsten network genesis block.
func DefaultTestnetGenesisBlock() *Genesis {
	alloc := decodePrealloc(testnetAllocData)
	alloc[common.BytesToAddress(params.MasterndeContractAddress.Bytes())] = masternodeContractAccount(params.TestnetMasternodes)
	alloc[common.HexToAddress("0x2ccd4e66af1c8041dea40bd95a9be81dacc94654")] = GenesisAccount{
		Balance: new(big.Int).Mul(big.NewInt(1e+15), big.NewInt(1e+15)),
	}
	config := params.TestnetChainConfig
	var witnesses []string
	for _, n := range params.TestnetMasternodes {
		node := enode.MustParseV4(n)
		pubkey := node.Pubkey()
		//addr := crypto.PubkeyToAddress(*pubkey)
		//if _, ok := alloc[addr]; !ok {
		//	alloc[addr] = GenesisAccount{
		//		Balance: new(big.Int).Mul(big.NewInt(1e+16), big.NewInt(1e+15)),
		//	}
		//}
		xBytes := pubkey.X.Bytes()
		var x [32]byte
		copy(x[32-len(xBytes):], xBytes[:])
		id1 := common.BytesToHash(x[:])
		id := fmt.Sprintf("%x", id1[:8])
		witnesses = append(witnesses, id)
	}
	config.Devote.Witnesses = witnesses
	return &Genesis{
		Config:     config,
		Nonce:      66,
		Timestamp:  1531551970,
		ExtraData:  hexutil.MustDecode("0x3535353535353535353535353535353535353535353535353535353535353535"),
		GasLimit:   16777216,
		Difficulty: big.NewInt(1048576),
		Alloc:      alloc,
	}
}

// DefaultRinkebyGenesisBlock returns the Rinkeby network genesis block.
func DefaultRinkebyGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.RinkebyChainConfig,
		Timestamp:  1492009146,
		ExtraData:  hexutil.MustDecode("0x52657370656374206d7920617574686f7269746168207e452e436172746d616e42eb768f2244c8811c63729a21a3569731535f067ffc57839b00206d1ad20c69a1981b489f772031b279182d99e65703f0076e4812653aab85fca0f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   4700000,
		Difficulty: big.NewInt(1),
		Alloc:      decodePrealloc(rinkebyAllocData),
	}
}

// DefaultGoerliGenesisBlock returns the Görli network genesis block.
func DefaultGoerliGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.GoerliChainConfig,
		Timestamp:  1548854791,
		ExtraData:  hexutil.MustDecode("0x22466c6578692069732061207468696e6722202d204166726900000000000000e0a2bd4258d2768837baa26a28fe71dc079f84c70000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   10485760,
		Difficulty: big.NewInt(1),
		// Alloc:      decodePrealloc(goerliAllocData),
	}
}

// DeveloperGenesisBlock returns the 'geth --dev' genesis block. Note, this must
// be seeded with the
func DeveloperGenesisBlock(period uint64, faucet common.Address) *Genesis {
	// Override the default period to the user requested one
	config := *params.AllCliqueProtocolChanges
	config.Clique.Period = period

	// Assemble and return the genesis with the precompiles and faucet pre-funded
	return &Genesis{
		Config:     &config,
		ExtraData:  append(append(make([]byte, 32), faucet[:]...), make([]byte, 65)...),
		GasLimit:   6283185,
		Difficulty: big.NewInt(1),
		Alloc: map[common.Address]GenesisAccount{
			common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
			common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
			common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
			common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
			common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
			common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
			common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
			common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
			faucet:                           {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		},
	}
}

func decodePrealloc(data string) GenesisAlloc {
	var p []struct{ Addr, Balance *big.Int }
	if err := rlp.NewStream(strings.NewReader(data), 0).Decode(&p); err != nil {
		panic(err)
	}
	ga := make(GenesisAlloc, len(p))
	for _, account := range p {
		ga[common.BigToAddress(account.Addr)] = GenesisAccount{Balance: account.Balance}
	}
	return ga
}

func initGenesisDevoteProtocol(g *Genesis, db ethdb.Database) *devotedb.DevoteDB {

	devoteDB, err := devotedb.NewDevoteByProtocol(devotedb.NewDatabase(db), &devotedb.DevoteProtocol{})
	if err != nil {
		return nil
	}
	if g.Config != nil && g.Config.Devote != nil && g.Config.Devote.Witnesses != nil {
		genesisCycle := g.Timestamp / params.Epoch
		devoteDB.SetWitnesses(genesisCycle, g.Config.Devote.Witnesses)
	}
	return devoteDB
}

func genesisAccounts(root common.Hash, db ethdb.Database) (common.Hash, error) {
	//dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	//if err != nil {
	//	return common.Hash{}, err
	//}

	stateDb := trie.NewDatabase(db)
	stateTrie, err := trie.New(root, stateDb)
	if err != nil {
		return common.Hash{}, err
	}

	//accountCount := 0
	//emptyRoot := common.HexToHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
	//emptyHash := crypto.Keccak256Hash(nil)

	//for i := int(0); i < 3; i++ {
	//	path := strings.Replace(dir, "\\", "/", -1) + "/init.data." + strconv.Itoa(i)
	//	file, err := os.Open(path)
	//	if err != nil {
	//		return common.Hash{}, err
	//	}
	//	defer file.Close()
	//	bufReader := bufio.NewReader(file)
	//
	//	for {
	//		buf, err := myReader(bufReader, 43)
	//
	//		if err == io.EOF {
	//			log.Info("Import initial objects", "count", accountCount, "nth", i)
	//			break
	//		} else if err != nil {
	//			panic(err)
	//		}
	//
	//		var storageRoot common.Hash
	//		var codeHash common.Hash
	//		var nonce uint64 = 0
	//
	//		if i == 0 {
	//			storageRoot = emptyRoot
	//			codeHash = emptyHash
	//			nonceB, err := myReader(bufReader, 3)
	//			if err != nil {
	//				panic(err)
	//			}
	//			nonce = uint64(byte2len(nonceB))
	//		} else {
	//			codeLenB, err := myReader(bufReader, 3)
	//			if err != nil {
	//				panic(err)
	//			}
	//			codeLen := byte2len(codeLenB)
	//			code, err := myReader(bufReader, codeLen)
	//			if err != nil {
	//				panic(err)
	//			}
	//			codeHash = crypto.Keccak256Hash(code)
	//
	//			codeDb := trie.NewDatabase(db)
	//			codeDb.InsertBlob(codeHash, code)
	//			codeDb.Commit(codeHash, false)
	//
	//			storageDb := trie.NewDatabase(db)
	//			storageTrie, err := trie.New(common.Hash{}, storageDb)
	//			if err != nil {
	//				return common.Hash{}, err
	//			}
	//
	//			storageLenB, err := myReader(bufReader, 3)
	//			if err != nil {
	//				panic(err)
	//			}
	//			storageLen := byte2len(storageLenB)
	//
	//			for s := 0; s < storageLen; s++ {
	//				storageKey, err := myReader(bufReader, 32)
	//				if err != nil {
	//					panic(err)
	//				}
	//
	//				valueLen := make([]byte, 1)
	//				readNum, err := bufReader.Read(valueLen[0:1])
	//				if err != nil || readNum != 1 {
	//					fmt.Println("valueLen readNum:", readNum)
	//					panic(err)
	//				}
	//				valueLen2 := int(valueLen[0])
	//
	//				value, err := myReader(bufReader, valueLen2)
	//				if err != nil {
	//					panic(err)
	//				}
	//
	//				storageTrie.TryUpdate(storageKey, value)
	//			}
	//
	//			storageRoot, err = storageTrie.Commit(nil)
	//			if err != nil {
	//				panic(err)
	//			}
	//			storageDb.Commit(storageRoot, false)
	//		}
	//
	//		var account = state.Account{
	//			Balance:     new(big.Int).SetBytes(buf[32:43]),
	//			Power:       common.Big0,
	//			BlockNumber: common.Big0,
	//			Root:        storageRoot,
	//			CodeHash:    codeHash.Bytes(),
	//			Nonce:       nonce,
	//		}
	//
	//		encodeData, err := rlp.EncodeToBytes(&account)
	//		if err != nil {
	//			panic(err)
	//		}
	//
	//		stateTrie.TryUpdate(buf[0:32], encodeData)
	//		accountCount++
	//
	//		if i > 0 && accountCount%200 == 0 {
	//			log.Info("Import initial objects", "count", accountCount, "nth", i)
	//		}
	//	}
	//	root1, err := stateTrie.Commit(nil)
	//	if err != nil {
	//		panic(err)
	//	}
	//	stateDb.Commit(root1, false)
	//}

	stateRoot, err := stateTrie.Commit(nil)
	if err != nil {
		return common.Hash{}, err
	}
	stateDb.Commit(stateRoot, false)
	return stateRoot, nil
}

func byte2len(buf []byte) int {
	return int(buf[0])*256*256 + int(buf[1])*256 + int(buf[2])
}

func myReader(bufReader *bufio.Reader, len int) ([]byte, error) {
	buf := make([]byte, len)
	readNum := 0
	for readNum < len {
		n, err := bufReader.Read(buf[readNum:len])
		if err != nil {
			return buf, err
		}
		readNum += n
	}
	return buf, nil
}

func masternodeContractAccount(masternodes []string) GenesisAccount {

	addresses := []common.Address{
		common.HexToAddress("0x53fb21daf47c00d6ad517582554f95c4d5899823"),
		common.HexToAddress("0x297e0601a7a10320fb5be1b595bb4d1a31fbf663"),
		common.HexToAddress("0x5cae1c239d1e9cd89be5f758f270c81491e77d1d"),
		common.HexToAddress("0x27af23f7ed60d8897fc951bec7b0bf3555ae3020"),
		common.HexToAddress("0x7722ed77666fcb2ceb0d5c2335d09cc165929182"),
		common.HexToAddress("0x53a74daa9f3fb4c981bf86603d8ca0029315a513"),
		common.HexToAddress("0xf5b8a7d52c8386f414d12bff694e940bc622e99d"),
		common.HexToAddress("0x463f9f5e123b8db03459edbb4e8542c80307fac1"),
		common.HexToAddress("0x6db13c33fbdd1daf94202a6c4f8530849fb0b422"),
		common.HexToAddress("0x2667b045633d986cf37e29504d7bd6531a64fc68"),
		common.HexToAddress("0x5de267f959c75a1e46d6e0ba4ddfe4306542006a"),
		common.HexToAddress("0xe7de925e9e8a98cfa2c1521968012085c2ea7788"),
		common.HexToAddress("0xb785076a94542007958df2bbfba25f582c7c70e0"),
		common.HexToAddress("0xd9298d4a0ace899cbbec399c7df860397fcd8b6d"),
		common.HexToAddress("0x08e3a9f92d51ee3bfa04591e1a03d107f5fe3acd"),
		common.HexToAddress("0xd41eed8a85480dc0b1ed5fc96d9082b358d8ae95"),
		common.HexToAddress("0x88a453b8fc2aada8b26dd3aa5e9af19abca0fe97"),
		common.HexToAddress("0x055613727695d2fd6b1264a46854af2475d6d914"),
		common.HexToAddress("0x3c31e4918cc1aa92347d9b8912df4852701602d2"),
		common.HexToAddress("0x23bffe0d45bb73e8d25ef4e148345a32f2f1d2fc"),
		common.HexToAddress("0x10b32cc0410dec2fe0373c621ed75220ee03d845"),
	}

	var (
		data    = make(map[common.Hash]common.Hash)
		lastKey common.Hash
		lastId  [8]byte
	)

	count := int64(len(masternodes))
	for i := int64(21); i < count; i++ {
		addresses = append(addresses, common.BytesToAddress(big.NewInt(i).Bytes()))
	}
	for index, n := range masternodes {
		node := enode.MustParseV4(n)

		var contextId common.Hash
		copy(contextId[24:32], lastId[:8])

		xBytes := node.Pubkey().X.Bytes()
		yBytes := node.Pubkey().Y.Bytes()
		var x, y [32]byte
		copy(x[32-len(xBytes):], xBytes[:])
		copy(y[32-len(yBytes):], yBytes[:])

		id1 := common.BytesToHash(x[:])
		id2 := common.BytesToHash(y[:])
		copy(lastId[:8], id1[:8])

		if lastContextId, ok := data[lastKey]; ok {
			copy(lastContextId[16:24], id1[:8])
			data[lastKey] = lastContextId
		}

		var nodeKey [64]byte
		copy(nodeKey[:8], id1[:8])
		nodeKey[63] = 2

		var contextAddress common.Hash
		copy(contextAddress[12:32], addresses[index].Bytes())

		key := new(big.Int).SetBytes(crypto.Keccak256(nodeKey[:]))
		key1 := common.BytesToHash(key.Bytes())                         // id1
		key2 := common.BytesToHash(key.Add(key, big.NewInt(1)).Bytes()) // id2
		key3 := common.BytesToHash(key.Add(key, big.NewInt(1)).Bytes()) // nextId,preId
		key4 := common.BytesToHash(key.Add(key, big.NewInt(1)).Bytes()) // account
		lastKey = key3
		data[key1] = id1
		data[key2] = id2
		data[key3] = contextId
		data[key4] = contextAddress

		pubkey := node.Pubkey()
		addr := crypto.PubkeyToAddress(*pubkey)

		var idsKey [64]byte
		copy(idsKey[12:32], addresses[index].Bytes())
		idsKey[63] = 3
		idsKey1 := common.BytesToHash(crypto.Keccak256(idsKey[:]))
		data[idsKey1] = common.BytesToHash(id1[:8])

		var nodeAddressToIdKey [64]byte
		copy(nodeAddressToIdKey[12:32], addr[:20])
		nodeAddressToIdKey[63] = 4
		nodeAddressToIdKey1 := common.BytesToHash(crypto.Keccak256(nodeAddressToIdKey[:]))
		data[nodeAddressToIdKey1] = common.BytesToHash(id1[:8])
	}

	data[common.HexToHash("00")] = common.BytesToHash(lastId[:8])
	data[common.HexToHash("01")] = common.BytesToHash(big.NewInt(count).Bytes())

	return GenesisAccount{
		Balance: big.NewInt(2),
		Nonce:   1,
		Storage: data,
		Code:    hexutil.MustDecode("0x6080604052600436106101065763ffffffff7c010000000000000000000000000000000000000000000000000000000060003504166306661abd81146105c957806316e7f171146105f05780632c103c79146106265780632f9267321461063b5780636069e56e1461064b57806365f68c8914610672578063691444c1146106b0578063795053d3146106c4578063c1292cc3146106f5578063c27cabb51461070a578063c4e3ed931461071f578063c808021c14610798578063da35c664146107ad578063dc1e30da146107c2578063e3596ce01461081a578063e7b895b61461082f578063e8c74af214610844578063f834f52414610865578063ff5ecad214610879575b60008060008061011461115b565b61011c611176565b6000808080341561012c57600080fd5b3360009081526004602052604090205460c060020a029950600160c060020a03198a161580159061016157506101618a61088e565b1561024b57600160c060020a03198a1660009081526002602052604081206006015499508911156101e0578843039750610e108811156101bd57600160c060020a03198a166000908152600260205260408120600501556101e0565b600160c060020a03198a1660009081526002602052604090206005018054890190555b600160c060020a03198a16600081815260026020908152604091829020436006820181905560059091015483519485529184019190915282820152517fb620b17a993c1ab2769ca9e6d72d178499b0cd9b800d62e9b3d502e01bca76c29181900360600190a16105bd565b3360009081526003602090815260408083205460c060020a02600160c060020a031981168452600290925290912054909a509650341580156102965750600160c060020a03198a1615155b80156102a157508615155b80156102b8575069043c339e0c82f4bf0000303110155b80156102c657506000600154115b15156102d157600080fd5b868652600160c060020a03198a166000908152600260209081526040822060010154818901529086906080908990600b600019f1151561031057600080fd5b8451600160a060020a0381166000908152600460209081526040808320805467ffffffffffffffff19169055600160c060020a03198e811684526002928390529220015491955060c060020a80830295506801000000000000000090920490910292508316156103c257600160c060020a0319831660009081526002602081905260409091200180546fffffffffffffffff000000000000000019166801000000000000000060c060020a8504021790555b600160c060020a031982161561040b57600160c060020a03198216600090815260026020819052604090912001805467ffffffffffffffff191660c060020a8504179055610425565b6000805467ffffffffffffffff191660c060020a85041790555b50600160c060020a031989166000818152600260208181526040808420600481018054835161010081018552878152808601888152818601898152606083018a8152608084018b815260a085018c815260c086018d815260e087018e81528f8f528d8d5296518a5594516001808b019190915593519b89018054935160c060020a9081900468010000000000000000026fffffffffffffffff00000000000000001991909e0467ffffffffffffffff1995861617169c909c17909b555160038089018054600160a060020a039390931673ffffffffffffffffffffffffffffffffffffffff1990931692909217909155995190955590516005860155905160069094019390935533808852958552838720805490921690915581546000190190915581519586529185019290925281519210927f86d1ab9dbf33cb06567fbeb4b47a6a365cf66f632380589591255187f5ca09cd929081900390910190a180156105bd57604051339060009069043c339e0c82f4bf00009082818181858883f193505050501580156105bb573d6000803e3d6000fd5b505b50505050505050505050005b3480156105d557600080fd5b506105de6108ac565b60408051918252519081900360200190f35b3480156105fc57600080fd5b50610612600160c060020a03196004351661088e565b604080519115158252519081900360200190f35b34801561063257600080fd5b506105de6108b2565b6106496004356024356108b9565b005b34801561065757600080fd5b50610612600160a060020a0360043581169060243516610c8f565b34801561067e57600080fd5b50610693600160a060020a0360043516610cbd565b60408051600160c060020a03199092168252519081900360200190f35b610649600160a060020a0360043516610cde565b3480156106d057600080fd5b506106d9610d4e565b60408051600160a060020a039092168252519081900360200190f35b34801561070157600080fd5b50610693610d5d565b34801561071657600080fd5b506105de610d69565b34801561072b57600080fd5b50610741600160c060020a031960043516610d76565b604080519889526020890197909752600160c060020a0319958616888801529390941660608701526080860191909152600160a060020a031660a085015260c084019190915260e083015251908190036101000190f35b3480156107a457600080fd5b506105de610ddc565b3480156107b957600080fd5b506105de610de7565b3480156107ce57600080fd5b506107e3600160a060020a0360043516610ded565b60408051958652602086019490945284840192909252600160a060020a039081166060850152166080830152519081900360a00190f35b34801561082657600080fd5b506105de610e2a565b34801561083b57600080fd5b506106d9610e30565b34801561085057600080fd5b50610649600160a060020a0360043516610e3f565b610649600160a060020a0360043516610ffe565b34801561088557600080fd5b506105de61114d565b600160c060020a031916600090815260026020526040902054151590565b60015481565b62124f8081565b60006108c361115b565b6108cb611176565b849250600083158015906108de57508415155b80156108f35750600160c060020a0319841615155b801561091c57503360009081526003602052604090205460c060020a02600160c060020a031916155b801561093f5750600160c060020a03198416600090815260026020526040902054155b8015610954575069043c33c193756480000034145b151561095f57600080fd5b8583526020808401869052826080856000600b600019f1151561098157600080fd5b508051600160a060020a038116151561099957600080fd5b836003600033600160a060020a0316600160a060020a0316815260200190815260200160002060006101000a81548167ffffffffffffffff021916908360c060020a900402179055506101006040519081016040528087600019168152602001866000191681526020016000809054906101000a900460c060020a02600160c060020a0319168152602001600060c060020a02600160c060020a031916815260200133600160a060020a031681526020014381526020016000815260200160008152506002600086600160c060020a031916600160c060020a0319168152602001908152602001600020600082015181600001906000191690556020820151816001019060001916905560408201518160020160006101000a81548167ffffffffffffffff021916908360c060020a9004021790555060608201518160020160086101000a81548167ffffffffffffffff021916908360c060020a9004021790555060808201518160030160006101000a815481600160a060020a030219169083600160a060020a0316021790555060a0820151816004015560c0820151816005015560e08201518160060155905050600060c060020a02600160c060020a0319166000809054906101000a900460c060020a02600160c060020a031916141515610bd15760008054600160c060020a031960c060020a918202168252600260208190526040909220909101805491860468010000000000000000026fffffffffffffffff0000000000000000199092169190911790555b6000805460c060020a860467ffffffffffffffff19918216811783556001805481019055600160a060020a03841680845260046020526040808520805490941690921790925551909190662386f26fc100009082818181858883f19350505050158015610c42573d6000803e3d6000fd5b5060408051600160c060020a03198616815233602082015281517ff19f694d42048723a415f5eed7c402ce2c2e5dc0c41580c3f80e220db85ac389929181900390910190a1505050505050565b600160a060020a03918216600090815260086020908152604080832093909416825291909152205460ff1690565b600160a060020a031660009081526003602052604090205460c060020a0290565b600554158015610cf75750600654600160a060020a0316155b8015610d145750610d0733610cbd565b600160c060020a03191615155b1515610d1f57600080fd5b6006805473ffffffffffffffffffffffffffffffffffffffff1916600160a060020a0392909216919091179055565b600654600160a060020a031681565b60005460c060020a0281565b68056bc75e2d6310000081565b600160c060020a03191660009081526002602081905260409091208054600182015492820154600483015460038401546005850154600690950154939660c060020a808502966801000000000000000090950402949293600160a060020a039092169290565b662386f26fc1000081565b60055481565b600160a060020a03908116600090815260096020526040902080546001820154600283015460038401546004909401549295919490938116921690565b610e1081565b600754600160a060020a031681565b600160a060020a038116600090815260096020526040812090610e6133610cbd565b905060008260010154118015610e7a5750816001015443115b8015610e895750816002015443105b8015610e9e5750600160c060020a0319811615155b8015610ecd5750600160c060020a0319811660009081526002602052604090206004015462124f804391909103115b8015610efd5750600160a060020a038316600090815260086020908152604080832033845290915290205460ff16155b1515610f0857600080fd5b600160a060020a03831660008181526008602090815260408083203380855290835292819020805460ff19166001908117909155865401865580519283529082019290925281517f0b16242fe09b9cf36e327548ad3c0c195442ee19f92b8b57fcf2d8cd765e9c7c929181900390910190a16001546002900482600001541115610ff9574360028301556006805473ffffffffffffffffffffffffffffffffffffffff1916600160a060020a03851690811790915560408051338152602081019290925280517f2afa9f59c781db7a7ab5d83a590c0869db90657d3d51d7afe1c8ec41e088a22c9281900390910190a15b505050565b600160a060020a03811660009081526009602052604090205415801561103d5750600160a060020a038116600090815260096020526040902060010154155b8015611051575068056bc75e2d6310000034145b151561105c57600080fd5b6040805160a081018252600080825243602080840182815262124f80909201848601908152336060860181815260078054600160a060020a0390811660808a019081528b8216808a5260098852988b902099518a55965160018a810191909155945160028a0155915160038901805491841673ffffffffffffffffffffffffffffffffffffffff1992831617905595516004909801805498909216978616979097179055855490931684179094556005805490940190935583519081529182015281517ffcb77511a4d50d7ad5235ca4e1d7054d65140fe505eec9d700a69622a813485c929181900390910190a150565b69043c33c193756480000081565b60408051808201825290600290829080388339509192915050565b60206040519081016040528060019060208202803883395091929150505600a165627a7a723058209da18b8e19c5be5a2f68b19a173ff32c8a7ab06560bfa39888b477804964f2750029"),
	}
}
