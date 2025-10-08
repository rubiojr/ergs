package api

import (
	"time"

	"github.com/rubiojr/ergs/cmd/web/components/types"
)

type BlockResponse struct {
	ID        string                 `json:"id"`
	Text      string                 `json:"text"`
	Source    string                 `json:"source"`
	CreatedAt time.Time              `json:"created_at"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type ListDatasourcesResponse struct {
	Datasources []types.DatasourceInfo `json:"datasources"`
	Count       int                    `json:"count"`
}

type ListBlocksResponse struct {
	Datasource string          `json:"datasource"`
	Blocks     []BlockResponse `json:"blocks"`
	Count      int             `json:"count"`
	Query      string          `json:"query,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type SearchResponse struct {
	Query      string                        `json:"query"`
	Results    map[string]ListBlocksResponse `json:"results"`
	TotalCount int                           `json:"total_count"`
	Page       int                           `json:"page"`
	Limit      int                           `json:"limit"`
	TotalPages int                           `json:"total_pages"`
	HasMore    bool                          `json:"has_more"`
}

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

type FirehoseResponse struct {
	Blocks     []BlockResponse `json:"blocks"`
	Count      int             `json:"count"`
	Page       int             `json:"page"`
	Limit      int             `json:"limit"`
	TotalPages int             `json:"total_pages"`
	HasMore    bool            `json:"has_more"`
}
