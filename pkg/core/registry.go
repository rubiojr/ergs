package core

import (
	"fmt"
	"sync"
)

// Global registry for datasource self-registration
var globalRegistry = &Registry{
	prototypes:  make(map[string]Datasource),
	datasources: make(map[string]Datasource),
}

type Registry struct {
	prototypes  map[string]Datasource
	datasources map[string]Datasource
	mu          sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		prototypes:  make(map[string]Datasource),
		datasources: make(map[string]Datasource),
	}
}

// RegisterDatasourcePrototype allows datasources to register themselves during init()
func RegisterDatasourcePrototype(name string, prototype Datasource) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.prototypes[name] = prototype
}

// GetGlobalRegistry returns the global registry with all registered datasources
func GetGlobalRegistry() *Registry {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	// Create a new registry and copy all registered prototypes
	registry := NewRegistry()
	for name, prototype := range globalRegistry.prototypes {
		registry.prototypes[name] = prototype
	}
	return registry
}

func (r *Registry) RegisterPrototype(name string, prototype Datasource) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.prototypes[name]; exists {
		return fmt.Errorf("datasource prototype %s already registered", name)
	}

	r.prototypes[name] = prototype
	return nil
}

func (r *Registry) CreateDatasource(instanceName string, factoryType string, config interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prototype, exists := r.prototypes[factoryType]
	if !exists {
		return fmt.Errorf("datasource prototype %s not found", factoryType)
	}

	if validator, ok := config.(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			return fmt.Errorf("invalid config for datasource %s: %w", instanceName, err)
		}
	}

	datasource, err := prototype.Factory(instanceName, config)
	if err != nil {
		return fmt.Errorf("creating datasource %s: %w", instanceName, err)
	}

	if existingDS, exists := r.datasources[instanceName]; exists {
		if err := existingDS.Close(); err != nil {
			return fmt.Errorf("closing existing datasource %s: %w", instanceName, err)
		}
	}

	r.datasources[instanceName] = datasource
	return nil
}

func (r *Registry) GetDatasource(name string) (Datasource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	datasource, exists := r.datasources[name]
	if !exists {
		return nil, fmt.Errorf("datasource %s not found", name)
	}

	return datasource, nil
}

func (r *Registry) GetAllDatasources() map[string]Datasource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]Datasource)
	for name, ds := range r.datasources {
		result[name] = ds
	}
	return result
}

func (r *Registry) ListDatasources() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.datasources))
	for name := range r.datasources {
		names = append(names, name)
	}
	return names
}

func (r *Registry) RemoveDatasource(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	datasource, exists := r.datasources[name]
	if !exists {
		return fmt.Errorf("datasource %s not found", name)
	}

	if err := datasource.Close(); err != nil {
		return fmt.Errorf("closing datasource %s: %w", name, err)
	}

	delete(r.datasources, name)
	return nil
}

func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, datasource := range r.datasources {
		if err := datasource.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing datasource %s: %w", name, err))
		}
	}

	r.datasources = make(map[string]Datasource)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing datasources: %v", errs)
	}

	return nil
}
