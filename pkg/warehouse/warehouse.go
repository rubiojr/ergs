package warehouse

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
)

type Config struct {
	OptimizeInterval time.Duration
}

type Warehouse struct {
	config              Config
	storageManager      *storage.Manager
	datasources         []core.Datasource
	datasourceNames     map[core.Datasource]string
	datasourceIntervals map[string]time.Duration
	datasourceTickers   map[string]*time.Ticker
	optimizeTicker      *time.Ticker
	stopCh              chan struct{}
	mu                  sync.RWMutex
	wg                  sync.WaitGroup
	running             bool
}

func NewWarehouse(config Config, storageManager *storage.Manager) *Warehouse {
	return &Warehouse{
		config:              config,
		storageManager:      storageManager,
		datasources:         make([]core.Datasource, 0),
		datasourceNames:     make(map[core.Datasource]string),
		datasourceIntervals: make(map[string]time.Duration),
		datasourceTickers:   make(map[string]*time.Ticker),
		stopCh:              make(chan struct{}),
	}
}

// AddDatasource adds a datasource to the warehouse with the default 30-minute fetch interval.
// For custom intervals, use AddDatasourceWithInterval instead.
func (w *Warehouse) AddDatasource(name string, ds core.Datasource) error {
	return w.AddDatasourceWithInterval(name, ds, 30*time.Minute)
}

// AddDatasourceWithInterval adds a datasource to the warehouse with a specific fetch interval.
// The interval determines how often this datasource will be polled for new data.
// Use 30*time.Minute for the default interval, or specify a custom duration.
func (w *Warehouse) AddDatasourceWithInterval(name string, ds core.Datasource, interval time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	err := w.storageManager.InitializeDatasourceStorage(name, ds.Schema())
	if err != nil {
		return fmt.Errorf("initializing storage for datasource %s: %w", name, err)
	}

	w.storageManager.RegisterBlockPrototype(name, ds.BlockPrototype())
	w.datasources = append(w.datasources, ds)
	w.datasourceNames[ds] = name
	w.datasourceIntervals[name] = interval
	return nil
}

func (w *Warehouse) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return fmt.Errorf("warehouse is already running")
	}

	if len(w.datasources) == 0 {
		return fmt.Errorf("no datasources configured")
	}

	w.running = true

	// Log all configured datasources and their intervals
	log.Printf("Starting warehouse with %d datasources:", len(w.datasources))
	for name, interval := range w.datasourceIntervals {
		log.Printf("  - %s: %v", name, interval)
	}

	// Start individual tickers for each datasource
	for name, interval := range w.datasourceIntervals {
		ticker := time.NewTicker(interval)
		w.datasourceTickers[name] = ticker
		w.wg.Add(1)
		go w.runDatasource(ctx, name, ticker)
		log.Printf("Started scheduler for datasource %s with interval %v", name, interval)
	}

	// Start optimization ticker if interval is configured
	if w.config.OptimizeInterval > 0 {
		w.optimizeTicker = time.NewTicker(w.config.OptimizeInterval)
		w.wg.Add(1)
		go w.runOptimization(ctx)
	}

	// Start initial fetch for all datasources (non-blocking)
	log.Println("Starting initial fetch")
	go func() {
		if err := w.fetchAll(ctx); err != nil {
			log.Printf("Initial fetch failed: %v", err)
		}
	}()

	log.Printf("Warehouse started with %d datasources, optimize interval: %v",
		len(w.datasources), w.config.OptimizeInterval)
	return nil
}

func (w *Warehouse) runDatasource(ctx context.Context, datasourceName string, ticker *time.Ticker) {
	defer w.wg.Done()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Datasource %s context cancelled", datasourceName)
			return
		case <-w.stopCh:
			log.Printf("Datasource %s stop signal received", datasourceName)
			return
		case <-ticker.C:
			log.Printf("Running scheduled fetch for datasource: %s", datasourceName)
			if err := w.fetchFromDatasourceByName(ctx, datasourceName); err != nil {
				log.Printf("Scheduled fetch failed for datasource %s: %v", datasourceName, err)
			}
		}
	}
}

func (w *Warehouse) runOptimization(ctx context.Context) {
	defer w.wg.Done()
	defer w.optimizeTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Optimization context cancelled")
			return
		case <-w.stopCh:
			log.Println("Optimization stop signal received")
			return
		case <-w.optimizeTicker.C:
			log.Println("Running database optimization")
			if err := w.storageManager.OptimizeAll(); err != nil {
				log.Printf("Database optimization failed: %v", err)
			}
		}
	}
}

func (w *Warehouse) fetchAll(ctx context.Context) error {
	w.mu.RLock()
	datasources := make([]core.Datasource, len(w.datasources))
	copy(datasources, w.datasources)
	w.mu.RUnlock()

	// Create a new channel for this fetch cycle
	blockCh := make(chan core.Block, 1000)

	// Use a separate WaitGroup for the initial fetch to avoid interfering with main scheduler
	var fetchWg sync.WaitGroup
	var processorWg sync.WaitGroup

	// Start block processor
	processorWg.Add(1)
	go func() {
		defer processorWg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case block, ok := <-blockCh:
				if !ok {
					return
				}
				if err := w.storeBlock(block); err != nil {
					log.Printf("Error storing block %s: %v", block.ID(), err)
				}
			}
		}
	}()

	for _, ds := range datasources {
		fetchWg.Add(1)
		go func(ds core.Datasource) {
			defer fetchWg.Done()
			w.mu.RLock()
			name := w.datasourceNames[ds]
			w.mu.RUnlock()

			log.Printf("Starting to fetch blocks from datasource: %s", name)
			err := ds.FetchBlocks(ctx, blockCh)
			if err != nil && err != context.Canceled {
				log.Printf("Error fetching blocks from datasource %s: %v", name, err)
			}
			log.Printf("Finished fetching blocks from datasource: %s", name)
		}(ds)
	}

	log.Printf("Started fetching from %d datasources", len(datasources))

	// Wait for all datasources to complete, then close the channel
	go func() {
		fetchWg.Wait()
		close(blockCh)
	}()

	// Wait for the block processor to finish
	processorWg.Wait()

	return nil
}

func (w *Warehouse) fetchFromDatasourceByName(ctx context.Context, datasourceName string) error {
	w.mu.RLock()
	var targetDS core.Datasource
	for ds, name := range w.datasourceNames {
		if name == datasourceName {
			targetDS = ds
			break
		}
	}
	w.mu.RUnlock()

	if targetDS == nil {
		return fmt.Errorf("datasource %s not found", datasourceName)
	}

	// Create a temporary channel for this single datasource fetch
	blockCh := make(chan core.Block, 1000)
	var fetchWg sync.WaitGroup
	var processorWg sync.WaitGroup

	// Start block processor
	processorWg.Add(1)
	go func() {
		defer processorWg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case block, ok := <-blockCh:
				if !ok {
					return
				}
				if err := w.storeBlock(block); err != nil {
					log.Printf("Error storing block %s: %v", block.ID(), err)
				}
			}
		}
	}()

	// Start fetching from the specific datasource
	fetchWg.Add(1)
	go func() {
		defer fetchWg.Done()
		log.Printf("Starting to fetch blocks from datasource: %s", datasourceName)
		err := targetDS.FetchBlocks(ctx, blockCh)
		if err != nil && err != context.Canceled {
			log.Printf("Error fetching blocks from datasource %s: %v", datasourceName, err)
		}
		log.Printf("Finished fetching blocks from datasource: %s", datasourceName)
	}()

	// Wait for fetch to complete, then close the channel
	go func() {
		fetchWg.Wait()
		close(blockCh)
	}()

	// Wait for the block processor to finish
	processorWg.Wait()

	return nil
}

func (w *Warehouse) storeBlock(block core.Block) error {
	// Use the block's source directly as the datasource name
	// since blocks now use instance names as their source
	storage, err := w.storageManager.GetStorage(block.Source())
	if err != nil {
		return fmt.Errorf("getting storage for datasource %s: %w", block.Source(), err)
	}

	// Find the datasource to get its type
	var datasourceType string
	w.mu.RLock()
	for ds, name := range w.datasourceNames {
		if name == block.Source() {
			datasourceType = ds.Type()
			break
		}
	}
	w.mu.RUnlock()

	// If we couldn't find the datasource type, use "unknown"
	if datasourceType == "" {
		datasourceType = "unknown"
	}

	err = storage.StoreBlock(block, datasourceType)
	if err != nil {
		return fmt.Errorf("storing block %s: %w", block.ID(), err)
	}

	return nil
}

func (w *Warehouse) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}

	log.Printf("Stopping warehouse...")
	close(w.stopCh)
	for name, ticker := range w.datasourceTickers {
		log.Printf("Stopping ticker for datasource: %s", name)
		ticker.Stop()
	}
	if w.optimizeTicker != nil {
		w.optimizeTicker.Stop()
	}
	w.running = false

	log.Printf("Waiting for warehouse goroutines to finish...")
	w.wg.Wait()
	log.Printf("Warehouse stopped")
}

func (w *Warehouse) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

func (w *Warehouse) FetchOnce(ctx context.Context) error {
	w.mu.RLock()
	datasources := make([]core.Datasource, len(w.datasources))
	copy(datasources, w.datasources)
	w.mu.RUnlock()

	if len(datasources) == 0 {
		return fmt.Errorf("no datasources configured")
	}

	// Create a new channel for this fetch cycle
	blockCh := make(chan core.Block, 1000)
	var fetchWg sync.WaitGroup
	var processorWg sync.WaitGroup

	// Start block processor
	processorWg.Add(1)
	go func() {
		defer processorWg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case block, ok := <-blockCh:
				if !ok {
					return
				}
				if err := w.storeBlock(block); err != nil {
					log.Printf("Error storing block %s: %v", block.ID(), err)
				}
			}
		}
	}()

	// Start fetching from all datasources
	for _, ds := range datasources {
		fetchWg.Add(1)
		go func(ds core.Datasource) {
			defer fetchWg.Done()
			name := w.datasourceNames[ds]
			log.Printf("Starting to fetch blocks from datasource: %s", name)
			err := ds.FetchBlocks(ctx, blockCh)
			if err != nil && err != context.Canceled {
				log.Printf("Error fetching blocks from datasource %s: %v", name, err)
			}
			log.Printf("Finished fetching blocks from datasource: %s", name)
		}(ds)
	}

	// Wait for all datasources to complete, then close the channel
	go func() {
		fetchWg.Wait()
		close(blockCh)
	}()

	// Wait for the block processor to finish
	processorWg.Wait()

	log.Printf("One-time fetch completed from %d datasources", len(datasources))
	return nil
}

func (w *Warehouse) FetchOnceWithStreaming(ctx context.Context, onBlock func(core.Block)) error {
	w.mu.RLock()
	datasources := make([]core.Datasource, len(w.datasources))
	copy(datasources, w.datasources)
	w.mu.RUnlock()

	if len(datasources) == 0 {
		return fmt.Errorf("no datasources configured")
	}

	// Create a new channel for this fetch cycle
	blockCh := make(chan core.Block, 1000)
	var fetchWg sync.WaitGroup
	var processorWg sync.WaitGroup

	// Start block processor with streaming callback
	processorWg.Add(1)
	go func() {
		defer processorWg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case block, ok := <-blockCh:
				if !ok {
					return
				}
				// Call the streaming callback first
				if onBlock != nil {
					onBlock(block)
				}
				// Then store the block
				if err := w.storeBlock(block); err != nil {
					log.Printf("Error storing block %s: %v", block.ID(), err)
				}
			}
		}
	}()

	// Start fetching from all datasources
	for _, ds := range datasources {
		fetchWg.Add(1)
		go func(ds core.Datasource) {
			defer fetchWg.Done()
			name := w.datasourceNames[ds]
			log.Printf("Starting to fetch blocks from datasource: %s", name)
			err := ds.FetchBlocks(ctx, blockCh)
			if err != nil && err != context.Canceled {
				log.Printf("Error fetching blocks from datasource %s: %v", name, err)
			}
			log.Printf("Finished fetching blocks from datasource: %s", name)
		}(ds)
	}

	// Wait for all datasources to complete, then close the channel
	go func() {
		fetchWg.Wait()
		close(blockCh)
	}()

	// Wait for the block processor to finish
	processorWg.Wait()

	log.Printf("Streaming fetch completed from %d datasources", len(datasources))
	return nil
}

func (w *Warehouse) Close() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for _, ds := range w.datasources {
		if err := ds.Close(); err != nil {
			log.Printf("Error closing datasource %s: %v", ds.Name(), err)
		}
	}

	return nil
}

func (w *Warehouse) GetStats() (map[string]interface{}, error) {
	return w.storageManager.GetStats()
}

func (w *Warehouse) SearchBlocks(datasourceName, query string, limit int) ([]core.Block, error) {
	return w.storageManager.SearchBlocks(datasourceName, query, limit)
}

func (w *Warehouse) SearchAllDatasources(query string, limit int) (map[string][]core.Block, error) {
	return w.storageManager.SearchAllDatasources(query, limit)
}
