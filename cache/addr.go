package cache

import (
	"math/big"
	"squirrel/addr"
	"sync"
)

// AddrCacheItem caches address related data.
type AddrCacheItem struct {
	CreatedAt           uint64
	LastTransactionTime uint64
	AddrAssetCache      map[uint]*AddrAssetCacheItem
}

// AddrAssetCacheItem records balance of address assets.
type AddrAssetCacheItem struct {
	Balance *big.Float
	// This balance is 'up to date' till 'BlockIndex'.
	BlockIndex uint
}

var (
	addrCache     map[string]*AddrCacheItem
	addrCacheLock sync.Mutex

	// assetAlias maps all assetID with an integer number,
	// so we can reduce memory usage of cache.
	assetAlias      = make(map[string]uint)
	assetLock       sync.Mutex
	assetAliasMaxID = uint(0)
)

// LoadAddrAssetInfo caches all addr asset info.
func LoadAddrAssetInfo(addrAssetInfo []*addr.AssetInfo) {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	addrCache = make(map[string]*AddrCacheItem)

	for _, info := range addrAssetInfo {
		_, ok := addrCache[info.Address]
		if !ok {
			addrCache[info.Address] = &AddrCacheItem{
				CreatedAt:           info.CreatedAt,
				LastTransactionTime: info.LastTransactionTime,
				AddrAssetCache:      make(map[uint]*AddrAssetCacheItem),
			}
		}

		addrCache[info.Address].AddrAssetCache[getAssetAlias(info.AssetID)] = &AddrAssetCacheItem{
			Balance:    info.Balance,
			BlockIndex: 0,
		}
	}
}

// MigrateNEP5 handles nep5 contract migration.
func MigrateNEP5(newAssetAdmin, oldAssetID, newAssetID string) (uint, uint) {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	addrs := uint(0)
	holdingAddrs := uint(0)

	for addr, item := range addrCache {
		if addr == newAssetAdmin {
			if _, ok := addrCache[newAssetAdmin].AddrAssetCache[getAssetAlias(newAssetID)]; ok {
				continue
			}
		}

		if old, ok := item.AddrAssetCache[getAssetAlias(oldAssetID)]; ok {
			item.AddrAssetCache[getAssetAlias(newAssetID)] = &AddrAssetCacheItem{
				Balance:    new(big.Float).SetPrec(256).Copy(old.Balance),
				BlockIndex: old.BlockIndex,
			}

			addrs++
			if old.Balance.Sign() > 0 {
				holdingAddrs++
			}

			delete(item.AddrAssetCache, getAssetAlias(oldAssetID))
		}
	}

	return addrs, holdingAddrs
}

func getAssetAlias(assetID string) uint {
	assetLock.Lock()
	defer assetLock.Unlock()

	if alias, ok := assetAlias[assetID]; ok {
		return alias
	}

	assetAliasMaxID++
	assetAlias[assetID] = assetAliasMaxID
	return assetAliasMaxID
}

// GetAddr returns AddrCacheItem by address.
func GetAddr(address string) (*AddrCacheItem, bool) {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	cache, ok := addrCache[address]
	return cache, ok
}

// GetAddrAsset returns AddrAssetCacheItem by address and assetID.
func GetAddrAsset(address string, assetID string) (*AddrAssetCacheItem, bool) {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	cache, ok := addrCache[address]
	if !ok {
		return nil, false
	}

	addrAssetCache, ok := cache.AddrAssetCache[getAssetAlias(assetID)]
	return addrAssetCache, ok
}

// GetAddrOrCreate gets or creates address cache.
func GetAddrOrCreate(address string, txTime uint64) (*AddrCacheItem, bool) {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	if cache, ok := addrCache[address]; ok {
		return cache, false
	}

	cache := &AddrCacheItem{
		CreatedAt:           txTime,
		LastTransactionTime: txTime,
		AddrAssetCache:      make(map[uint]*AddrAssetCacheItem),
	}
	addrCache[address] = cache

	return cache, true
}

// UpdateCreatedTime updates address created time.
func (cache *AddrCacheItem) UpdateCreatedTime(blockTime uint64) bool {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	if cache.CreatedAt > blockTime {
		cache.CreatedAt = blockTime
		return true
	}

	return false
}

// UpdateLastTxTime updates address last transaction.
func (cache *AddrCacheItem) UpdateLastTxTime(lastTxTime uint64) bool {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	if cache.LastTransactionTime < lastTxTime {
		cache.LastTransactionTime = lastTxTime
		return true
	}

	return false
}

func getAddrCache(address string) *AddrCacheItem {
	cache, ok := addrCache[address]
	if !ok {
		panic("Falied to find target addrCache. Make sure address data is cached first")
	}

	return cache
}

// GetAddrAsset returns AddrAssetCacheItem by assetID.
func (cache *AddrCacheItem) GetAddrAsset(assetID string) (*AddrAssetCacheItem, bool) {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	addrAssetCache, ok := cache.AddrAssetCache[getAssetAlias(assetID)]
	return addrAssetCache, ok
}

// GetAddrAssetOrCreate gets or creates address asset cache.
func (cache *AddrCacheItem) GetAddrAssetOrCreate(assetID string, balance *big.Float) (*AddrAssetCacheItem, bool) {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	assetAlias := getAssetAlias(assetID)

	if addrAssetCache, ok := cache.AddrAssetCache[assetAlias]; ok {
		return addrAssetCache, false
	}

	cache.AddrAssetCache[assetAlias] = &AddrAssetCacheItem{
		Balance:    balance,
		BlockIndex: 0,
	}

	return cache.AddrAssetCache[assetAlias], true
}

// CreateAddrAsset creates address asset cache.
func CreateAddrAsset(address string, assetID string, balance *big.Float, blockIndex uint) {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	cache := getAddrCache(address)
	cache.AddrAssetCache[getAssetAlias(assetID)] = &AddrAssetCacheItem{
		Balance:    balance,
		BlockIndex: blockIndex,
	}
}

// UpdateBalance updates balance of address asset.
func (addrAssetCache *AddrAssetCacheItem) UpdateBalance(balance *big.Float, blockIndex uint) bool {
	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	if blockIndex < addrAssetCache.BlockIndex {
		return false
	}

	addrAssetCache.BlockIndex = blockIndex

	if addrAssetCache.Balance.Cmp(balance) != 0 {
		addrAssetCache.Balance = balance
		return true
	}

	return false
}

// AddBalance increases balance at the given blockIndex.
func (addrAssetCache *AddrAssetCacheItem) AddBalance(delta *big.Float, blockIndex uint) bool {
	if delta.Cmp(big.NewFloat(0)) == 0 {
		return false
	}

	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	if blockIndex < addrAssetCache.BlockIndex {
		return false
	}

	addrAssetCache.BlockIndex = blockIndex

	addrAssetCache.Balance = new(big.Float).SetPrec(256).Add(addrAssetCache.Balance, delta)
	return true
}

// SubtractBalance decreases balance at the given blockIndex.
func (addrAssetCache *AddrAssetCacheItem) SubtractBalance(delta *big.Float, blockIndex uint) bool {
	if delta.Cmp(big.NewFloat(0)) == 0 {
		return false
	}

	addrCacheLock.Lock()
	defer addrCacheLock.Unlock()

	if blockIndex < addrAssetCache.BlockIndex {
		return false
	}

	addrAssetCache.BlockIndex = blockIndex

	addrAssetCache.Balance = new(big.Float).SetPrec(256).Sub(addrAssetCache.Balance, delta)
	return true
}
