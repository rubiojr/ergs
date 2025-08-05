package shared

import (
	"sort"

	"github.com/rubiojr/ergs/cmd/web/components/types"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
)

func GetDatasourceList(registry *core.Registry, storageManager *storage.Manager) []types.DatasourceInfo {
	datasources := registry.GetAllDatasources()
	datasourceInfos := make([]types.DatasourceInfo, 0, len(datasources))

	stats, _ := storageManager.GetStats()

	for name, ds := range datasources {
		info := types.DatasourceInfo{
			Name: name,
			Type: ds.Type(),
		}

		if stats != nil {
			if dsStats, exists := stats[name]; exists {
				info.Stats = dsStats.(map[string]interface{})
			}
		}

		datasourceInfos = append(datasourceInfos, info)
	}

	sort.Slice(datasourceInfos, func(i, j int) bool {
		return datasourceInfos[i].Name < datasourceInfos[j].Name
	})

	return datasourceInfos
}
