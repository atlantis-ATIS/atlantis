// Copyright 2014 The go-athereum Authors
// This file is part of the go-athereum library.
//
// The go-athereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-athereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-athereum library. If not, see <http://www.gnu.org/licenses/>.

// Package ath implements the Atlantis protocol.
package ath

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/athereum/go-athereum/accounts"
	"github.com/athereum/go-athereum/common"
	"github.com/athereum/go-athereum/common/hexutil"
	"github.com/athereum/go-athereum/consensus"
	"github.com/athereum/go-athereum/consensus/clique"
	"github.com/athereum/go-athereum/consensus/athash"
	"github.com/athereum/go-athereum/core"
	"github.com/athereum/go-athereum/core/bloombits"
	"github.com/athereum/go-athereum/core/rawdb"
	"github.com/athereum/go-athereum/core/types"
	"github.com/athereum/go-athereum/core/vm"
	"github.com/athereum/go-athereum/ath/downloader"
	"github.com/athereum/go-athereum/ath/filters"
	"github.com/athereum/go-athereum/ath/gasprice"
	"github.com/athereum/go-athereum/athdb"
	"github.com/athereum/go-athereum/event"
	"github.com/athereum/go-athereum/internal/athapi"
	"github.com/athereum/go-athereum/log"
	"github.com/athereum/go-athereum/miner"
	"github.com/athereum/go-athereum/node"
	"github.com/athereum/go-athereum/p2p"
	"github.com/athereum/go-athereum/params"
	"github.com/athereum/go-athereum/rlp"
	"github.com/athereum/go-athereum/rpc"
)

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
	SetBloomBitsIndexer(bbIndexer *core.ChainIndexer)
}

// Atlantis implements the Atlantis full node service.
type Atlantis struct {
	config      *Config
	chainConfig *params.ChainConfig

	// Channel for shutting down the service
	shutdownChan chan bool // Channel for shutting down the Atlantis

	// Handlers
	txPool          *core.TxPool
	blockchain      *core.BlockChain
	protocolManager *ProtocolManager
	lesServer       LesServer

	// DB interfaces
	chainDb athdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *core.ChainIndexer             // Bloom indexer operating during block imports

	APIBackend *EthAPIBackend

	miner     *miner.Miner
	gasPrice  *big.Int
	atherbase common.Address

	networkId     uint64
	netRPCService *athapi.PublicNetAPI

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and atherbase)
}

func (s *Atlantis) AddLesServer(ls LesServer) {
	s.lesServer = ls
	ls.SetBloomBitsIndexer(s.bloomIndexer)
}

// New creates a new Atlantis object (including the
// initialisation of the common Atlantis object)
func New(ctx *node.ServiceContext, config *Config) (*Atlantis, error) {
	if config.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run ath.Atlantis in light sync mode, use les.LightAtlantis")
	}
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	chainDb, err := CreateDB(ctx, config, "chaindata")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	ath := &Atlantis{
		config:         config,
		chainDb:        chainDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         CreateConsensusEngine(ctx, &config.Ethash, chainConfig, chainDb),
		shutdownChan:   make(chan bool),
		networkId:      config.NetworkId,
		gasPrice:       config.GasPrice,
		atherbase:      config.Atlantisbase,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, params.BloomBitsBlocks),
	}

	log.Info("Initialising Atlantis protocol", "versions", ProtocolVersions, "network", config.NetworkId)

	if !config.SkipBcVersionCheck {
		bcVersion := rawdb.ReadDatabaseVersion(chainDb)
		if bcVersion != core.BlockChainVersion && bcVersion != 0 {
			return nil, fmt.Errorf("Blockchain DB version mismatch (%d / %d). Run gath upgradedb.\n", bcVersion, core.BlockChainVersion)
		}
		rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
	}
	var (
		vmConfig    = vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
		cacheConfig = &core.CacheConfig{Disabled: config.NoPruning, TrieNodeLimit: config.TrieCache, TrieTimeLimit: config.TrieTimeout}
	)
	ath.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, ath.chainConfig, ath.engine, vmConfig)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		ath.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	ath.bloomIndexer.Start(ath.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = ctx.ResolvePath(config.TxPool.Journal)
	}
	ath.txPool = core.NewTxPool(config.TxPool, ath.chainConfig, ath.blockchain)

	if ath.protocolManager, err = NewProtocolManager(ath.chainConfig, config.SyncMode, config.NetworkId, ath.eventMux, ath.txPool, ath.engine, ath.blockchain, chainDb); err != nil {
		return nil, err
	}
	ath.miner = miner.New(ath, ath.chainConfig, ath.EventMux(), ath.engine)
	ath.miner.SetExtra(makeExtraData(config.ExtraData))

	ath.APIBackend = &EthAPIBackend{ath, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.GasPrice
	}
	ath.APIBackend.gpo = gasprice.NewOracle(ath.APIBackend, gpoParams)

	return ath, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"gath",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// CreateDB creates the chain database.
func CreateDB(ctx *node.ServiceContext, config *Config, name string) (athdb.Database, error) {
	db, err := ctx.OpenDatabase(name, config.DatabaseCache, config.DatabaseHandles)
	if err != nil {
		return nil, err
	}
	if db, ok := db.(*athdb.LDBDatabase); ok {
		db.Meter("ath/db/chaindata/")
	}
	return db, nil
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Atlantis service
func CreateConsensusEngine(ctx *node.ServiceContext, config *athash.Config, chainConfig *params.ChainConfig, db athdb.Database) consensus.Engine {
	// If proof-of-authority is requested, set it up
	if chainConfig.Clique != nil {
		return clique.New(chainConfig.Clique, db)
	}
	// Otherwise assume proof-of-work
	switch config.PowMode {
	case athash.ModeFake:
		log.Warn("Ethash used in fake mode")
		return athash.NewFaker()
	case athash.ModeTest:
		log.Warn("Ethash used in test mode")
		return athash.NewTester()
	case athash.ModeShared:
		log.Warn("Ethash used in shared mode")
		return athash.NewShared()
	default:
		engine := athash.New(athash.Config{
			CacheDir:       ctx.ResolvePath(config.CacheDir),
			CachesInMem:    config.CachesInMem,
			CachesOnDisk:   config.CachesOnDisk,
			DatasetDir:     config.DatasetDir,
			DatasetsInMem:  config.DatasetsInMem,
			DatasetsOnDisk: config.DatasetsOnDisk,
		})
		engine.SetThreads(-1) // Disable CPU mining
		return engine
	}
}

// APIs return the collection of RPC services theatlantis package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Atlantis) APIs() []rpc.API {
	apis := athapi.GetAPIs(s.APIBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "ath",
			Version:   "1.0",
			Service:   NewPublicAtlantisAPI(s),
			Public:    true,
		}, {
			Namespace: "ath",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "ath",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "ath",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.APIBackend, false),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s.chainConfig, s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *Atlantis) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Atlantis) Atlantisbase() (eb common.Address, err error) {
	s.lock.RLock()
	atherbase := s.atherbase
	s.lock.RUnlock()

	if atherbase != (common.Address{}) {
		return atherbase, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			atherbase := accounts[0].Address

			s.lock.Lock()
			s.atherbase = atherbase
			s.lock.Unlock()

			log.Info("Atlantisbase automatically configured", "address", atherbase)
			return atherbase, nil
		}
	}
	return common.Address{}, fmt.Errorf("atherbase must be explicitly specified")
}

// SetAtlantisbase sets the mining reward address.
func (s *Atlantis) SetAtlantisbase(atherbase common.Address) {
	s.lock.Lock()
	s.atherbase = atherbase
	s.lock.Unlock()

	s.miner.SetAtlantisbase(atherbase)
}

func (s *Atlantis) StartMining(local bool) error {
	eb, err := s.Atlantisbase()
	if err != nil {
		log.Error("Cannot start mining without atherbase", "err", err)
		return fmt.Errorf("atherbase missing: %v", err)
	}
	if clique, ok := s.engine.(*clique.Clique); ok {
		wallet, err := s.accountManager.Find(accounts.Account{Address: eb})
		if wallet == nil || err != nil {
			log.Error("Atlantisbase account unavailable locally", "err", err)
			return fmt.Errorf("signer missing: %v", err)
		}
		clique.Authorize(eb, wallet.SignHash)
	}
	if local {
		// If local (CPU) mining is started, we can disable the transaction rejection
		// mechanism introduced to speed sync times. CPU mining on mainnet is ludicrous
		// so none will ever hit this path, whereas marking sync done on CPU mining
		// will ensure that private networks work in single miner mode too.
		atomic.StoreUint32(&s.protocolManager.acceptTxs, 1)
	}
	go s.miner.Start(eb)
	return nil
}

func (s *Atlantis) StopMining()         { s.miner.Stop() }
func (s *Atlantis) IsMining() bool      { return s.miner.Mining() }
func (s *Atlantis) Miner() *miner.Miner { return s.miner }

func (s *Atlantis) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *Atlantis) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *Atlantis) TxPool() *core.TxPool               { return s.txPool }
func (s *Atlantis) EventMux() *event.TypeMux           { return s.eventMux }
func (s *Atlantis) Engine() consensus.Engine           { return s.engine }
func (s *Atlantis) ChainDb() athdb.Database            { return s.chainDb }
func (s *Atlantis) IsListening() bool                  { return true } // Always listening
func (s *Atlantis) EthVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *Atlantis) NetVersion() uint64                 { return s.networkId }
func (s *Atlantis) Downloader() *downloader.Downloader { return s.protocolManager.downloader }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *Atlantis) Protocols() []p2p.Protocol {
	if s.lesServer == nil {
		return s.protocolManager.SubProtocols
	}
	return append(s.protocolManager.SubProtocols, s.lesServer.Protocols()...)
}

// Start implements node.Service, starting all internal goroutines needed by the
// Atlantis protocol implementation.
func (s *Atlantis) Start(srvr *p2p.Server) error {
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers()

	// Start the RPC service
	s.netRPCService = athapi.NewPublicNetAPI(srvr, s.NetVersion())

	// Figure out a max peers count based on the server limits
	maxPeers := srvr.MaxPeers
	if s.config.LightServ > 0 {
		if s.config.LightPeers >= srvr.MaxPeers {
			return fmt.Errorf("invalid peer config: light peer count (%d) >= total peer count (%d)", s.config.LightPeers, srvr.MaxPeers)
		}
		maxPeers -= s.config.LightPeers
	}
	// Start the networking layer and the light server if requested
	s.protocolManager.Start(maxPeers)
	if s.lesServer != nil {
		s.lesServer.Start(srvr)
	}
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Atlantis protocol.
func (s *Atlantis) Stop() error {
	s.bloomIndexer.Close()
	s.blockchain.Stop()
	s.protocolManager.Stop()
	if s.lesServer != nil {
		s.lesServer.Stop()
	}
	s.txPool.Stop()
	s.miner.Stop()
	s.eventMux.Stop()

	s.chainDb.Close()
	close(s.shutdownChan)

	return nil
}
