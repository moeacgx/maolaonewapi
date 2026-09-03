package controller

import (
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/model"
)

const maxBatchDeleteIDs = 500

type batchDeleteRequest struct {
	Ids []int `json:"ids"`
}

func buildBatchDeleteResult(requested, deleted []int) model.BatchDeleteResult {
	result := model.BatchDeleteResult{
		DeletedIds: append([]int{}, deleted...),
		Skipped:    make([]model.BatchDeleteSkipped, 0),
	}
	deletedSet := make(map[int]struct{}, len(deleted))
	for _, id := range deleted {
		deletedSet[id] = struct{}{}
	}
	for _, id := range requested {
		if _, ok := deletedSet[id]; !ok {
			result.Skipped = append(result.Skipped, model.BatchDeleteSkipped{Id: id, Reason: "not_found"})
		}
	}
	return result
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
