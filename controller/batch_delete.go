package controller

import (
	"fmt"
	"sort"
)

const maxBatchDeleteIDs = 500

type batchDeleteRequest struct {
	Ids []int `json:"ids"`
}

func normalizeBatchDeleteIDs(resource string, ids []int) ([]int, error) {
	if len(ids) == 0 || len(ids) > maxBatchDeleteIDs {
		return nil, fmt.Errorf("%s ID 数量必须为 1-%d", resource, maxBatchDeleteIDs)
	}
	seen := make(map[int]struct{}, len(ids))
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("%s ID 必须为正整数", resource)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Ints(result)
	return result, nil
}
